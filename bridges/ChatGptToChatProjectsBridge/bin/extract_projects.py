RECIPIENT = "chatgpt-backup"
#!/usr/bin/env python3
"""
extract_projects.py — ChatGptToChatProjectsBridge

Fetches all ChatGPT Projects via the internal sidebar API and writes one
outbox envelope per project for ThePostalService to deliver to ChatProjects.

Flow:
  1. Get session token from .env (fast path) or Chrome cookie store (bootstrap)
     — if read from Chrome, saves it to .env so Chrome is never touched again
  2. Refresh a short-lived Bearer JWT via /api/auth/session
  3. Paginate /backend-api/gizmos/snorlax/sidebar?owned_only=true
  4. For each project: write outbox/{slug}/project.json + letter.toml
  5. Publish a postal.signal so ThePostalService moves the folder

Token expiry: if the saved token stops working, delete CHATGPT_SESSION_TOKEN
from .env and run once manually — it will re-bootstrap from Chrome.
"""

import json
import os
import re
import time
import uuid
from pathlib import Path

import browser_cookie3
import pika
from curl_cffi import requests  # impersonates Chrome TLS — bypasses Cloudflare bot checks
from dotenv import load_dotenv, set_key

ENV_FILE      = Path(__file__).resolve().parent.parent / ".env"
load_dotenv(ENV_FILE)

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "localhost")

BASE_URL  = "https://chatgpt.com"
OUTBOX    = Path(__file__).resolve().parent.parent / "outbox"
SENDER    = "chatgpt-projects-bridge"
RECIPIENT = "chat-projects"


# ---------------------------------------------------------------------------
# Device ID — generated once, persisted to .env automatically
# ---------------------------------------------------------------------------

def get_or_create_device_id() -> str:
    """
    Return the stable device UUID from .env.
    If it's missing or still the placeholder, generate one and save it.
    """
    val = os.getenv("CHATGPT_DEVICE_ID", "")
    if not val or val == "replace_me":
        val = str(uuid.uuid4())
        set_key(str(ENV_FILE), "CHATGPT_DEVICE_ID", val)
        print(f"Generated new CHATGPT_DEVICE_ID: {val}")
    return val


# ---------------------------------------------------------------------------
# Auth
# ---------------------------------------------------------------------------

def get_session_token() -> str:
    """
    Return the ChatGPT session token.

    Fast path  — CHATGPT_SESSION_TOKEN is already in .env (all normal runs).
    Bootstrap  — if empty, read it from Chrome's cookie store and save it to
                 .env so Chrome is never touched again on future runs.

    If the saved token has expired, delete CHATGPT_SESSION_TOKEN from .env
    and run once manually to re-bootstrap from Chrome.
    """
    token = os.getenv("CHATGPT_SESSION_TOKEN", "")
    if token:
        return token

    print("CHATGPT_SESSION_TOKEN not in .env — bootstrapping from Chrome cookie store ...")
    try:
        cookie_jar = browser_cookie3.chrome(domain_name="chatgpt.com")
    except Exception as exc:
        raise RuntimeError(
            f"Could not read Chrome cookies: {exc}\n"
            "Make sure you are logged into chatgpt.com in Chrome."
        ) from exc

    # Chrome splits long cookies into numbered chunks:
    #   __Secure-next-auth.session-token.0, .1, ...
    # Collect all chunks in order and concatenate them.
    PREFIX = "__Secure-next-auth.session-token"
    chunks: dict[int, str] = {}
    for cookie in cookie_jar:
        if cookie.name == PREFIX:
            # Unchunked (short token) — use directly
            token = cookie.value
            break
        if cookie.name.startswith(PREFIX + "."):
            suffix = cookie.name[len(PREFIX) + 1:]
            if suffix.isdigit():
                chunks[int(suffix)] = cookie.value

    if not token and chunks:
        token = "".join(chunks[i] for i in sorted(chunks))

    if not token:
        raise RuntimeError(
            "Could not find __Secure-next-auth.session-token in Chrome cookies.\n"
            "Make sure you are logged into chatgpt.com in Chrome."
        )

    set_key(str(ENV_FILE), "CHATGPT_SESSION_TOKEN", token)
    print("Session token saved to .env — Chrome won't be needed on future runs.")
    return token


def make_session() -> tuple[requests.Session, str]:
    """
    Build a curl_cffi Session impersonating Chrome (bypasses Cloudflare TLS fingerprinting),
    set the session cookie, then fetch a Bearer JWT.
    """
    session = requests.Session(impersonate="chrome")
    session.cookies.set(
        "__Secure-next-auth.session-token",
        get_session_token(),
        domain="chatgpt.com",
    )
    jwt = get_bearer_jwt(session)
    return session, jwt


def get_bearer_jwt(session: requests.Session) -> str:
    """Exchange the session cookie for a short-lived Bearer JWT."""
    resp = session.get(f"{BASE_URL}/api/auth/session", timeout=15)
    resp.raise_for_status()
    data = resp.json()
    token = data.get("accessToken") or data.get("access_token")
    if not token:
        raise RuntimeError(
            "/api/auth/session returned no accessToken — session token may have expired.\n"
            "Delete CHATGPT_SESSION_TOKEN from .env and run again to re-bootstrap from Chrome."
        )
    return token


def auth_headers(jwt: str, device_id: str) -> dict:
    return {
        "Authorization":          f"Bearer {jwt}",
        "oai-device-id":          device_id,
        "oai-language":           "en-US",
        "Content-Type":           "application/json",
        "User-Agent":             (
            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
            "(KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
        ),
    }


# ---------------------------------------------------------------------------
# API calls
# ---------------------------------------------------------------------------

def fetch_projects(session: requests.Session, jwt: str, device_id: str) -> list[dict]:
    """Paginate the sidebar endpoint and return all project entries."""
    projects = []
    cursor   = None

    while True:
        params = {
            "owned_only":            "true",
            "conversations_per_gizmo": "0",
        }
        if cursor:
            params["cursor"] = cursor

        resp = session.get(
            f"{BASE_URL}/backend-api/gizmos/snorlax/sidebar",
            headers=auth_headers(jwt, device_id),
            params=params,
            timeout=20,
        )
        resp.raise_for_status()
        data = resp.json()

        # The response wraps projects in a list; key may be "gizmos" or "items".
        page = data.get("gizmos") or data.get("items") or []
        projects.extend(page)

        cursor = data.get("cursor")
        if not cursor:
            break

        time.sleep(0.5)  # be polite between pages

    return projects


def fetch_conversation_ids(
    session: requests.Session, jwt: str, device_id: str, project_id: str
) -> list[str]:
    """Return all conversation IDs that belong to this project."""
    ids    = []
    cursor = None

    while True:
        params = {}
        if cursor:
            params["cursor"] = cursor

        resp = session.get(
            f"{BASE_URL}/backend-api/gizmos/{project_id}/conversations",
            headers=auth_headers(jwt, device_id),
            params=params,
            timeout=20,
        )
        if resp.status_code == 404:
            # Project has no conversations yet
            break
        resp.raise_for_status()
        data  = resp.json()
        items = data.get("items") or []
        ids.extend(item["id"] for item in items if "id" in item)

        cursor = data.get("cursor")
        if not cursor:
            break
        time.sleep(0.3)

    return ids


def fetch_conversation_detail(session: requests.Session, jwt: str, device_id: str, conv_id: str) -> dict | None:
    """Fetch the full conversation payload including all messages mapping."""
    url = f"{BASE_URL}/backend-api/conversation/{conv_id}"
    try:
        resp = session.get(url, headers=auth_headers(jwt, device_id), timeout=20)
        resp.raise_for_status()
        return resp.json()
    except Exception as exc:
        print(f"  ✗ Failed to fetch details for {conv_id}: {exc}")
        return None


# ---------------------------------------------------------------------------
# Slug / ID helpers
# ---------------------------------------------------------------------------

def slugify(text: str) -> str:
    """'Code: Github.com Learning Projects' → 'code-github-com-learning-projects'"""
    text = text.lower()
    text = re.sub(r"[^a-z0-9]+", "-", text)
    return text.strip("-")


def _inner(project: dict) -> dict:
    """Unwrap the confirmed nested structure: project['gizmo']['gizmo']."""
    return (project.get("gizmo") or {}).get("gizmo") or {}


def extract_project_id(project: dict) -> str | None:
    inner = _inner(project)
    val = inner.get("id") or inner.get("gizmo_id")
    if val and str(val).startswith("g-p-"):
        return str(val)
    return None


def extract_display_name(project: dict) -> str:
    inner = _inner(project)
    # display.name is the confirmed path from the API response
    val = (inner.get("display") or {}).get("name") or inner.get("name")
    return str(val) if val else "untitled"


def extract_slug(project: dict) -> str | None:
    """
    Use the API-provided short_url as the folder slug — it's already in the
    correct format: g-p-{32hex}-{slugified-name}
    """
    inner = _inner(project)
    return inner.get("short_url") or None


def build_slug(project_id: str, display_name: str) -> str:
    return f"{project_id}-{slugify(display_name)}"


# ---------------------------------------------------------------------------
# Outbox writing
# ---------------------------------------------------------------------------

def write_package(slug: str, project_id: str, display_name: str, conv_details: list[dict]):
    package_dir = OUTBOX / slug
    package_dir.mkdir(parents=True, exist_ok=True)

    # conversation.json
    (package_dir / "conversation.json").write_text(
        json.dumps(conv_details, indent=2),
        encoding="utf-8",
    )

    # context.md (legacy format for PHP parser to pick up the exact project name)
    (package_dir / "context.md").write_text(
        f"## Matched Known Project\n{display_name}\n",
        encoding="utf-8",
    )

    # letter.toml
    (package_dir / "letter.toml").write_text(
        f'recipient = "{RECIPIENT}"\n'
        f'package_id = "{slug}"\n',
        encoding="utf-8",
    )


# ---------------------------------------------------------------------------
# Postal signal
# ---------------------------------------------------------------------------

def signal_postal_service(slug: str):
    try:
        connection = pika.BlockingConnection(
            pika.ConnectionParameters(
                    host=RABBITMQ_HOST,
                    port=int(os.getenv("RABBITMQ_PORT", 5672)),
                    credentials=pika.PlainCredentials(
                        os.getenv("RABBITMQ_USER", "postalWorker"),
                        os.getenv("RABBITMQ_PASS", "D0n74G37Me"),
                    ),
                )
        )
        channel = connection.channel()
        channel.exchange_declare(
            exchange="postal.signals", exchange_type="topic", durable=True
        )
        channel.basic_publish(
            exchange="postal.signals",
            routing_key="signal.ready",
            body=json.dumps(
                {
                    "event":      "package_ready",
                    "sender":     SENDER,
                    "package_id": slug,
                }
            ),
        )
        print(f"  → Signaled postal service for {slug}")
        connection.close()
    except Exception as exc:
        print(f"  ✗ Failed to signal postal service: {exc}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    device_id = get_or_create_device_id()
    OUTBOX.mkdir(parents=True, exist_ok=True)

    print("Reading Chrome cookies and refreshing Bearer JWT ...")
    session, jwt = make_session()

    print("Fetching project list ...")
    projects = fetch_projects(session, jwt, device_id)
    print(f"Found {len(projects)} project(s).")

    for project in projects:
        project_id   = extract_project_id(project)
        display_name = extract_display_name(project)

        if not project_id:
            print(f"  Skipping entry with no project_id: {project}")
            continue

        slug        = extract_slug(project) or build_slug(project_id, display_name)
        package_dir = OUTBOX / slug

        if package_dir.exists():
            print(f"  Skipping {slug} (already in outbox)")
            continue

        print(f"  Processing: {display_name} ({project_id})")

        conv_ids = fetch_conversation_ids(session, jwt, device_id, project_id)
        print(f"    {len(conv_ids)} conversation(s)")

        conv_details = []
        for i, cid in enumerate(conv_ids):
            print(f"    Fetching {i+1}/{len(conv_ids)}: {cid}")
            detail = fetch_conversation_detail(session, jwt, device_id, cid)
            if detail:
                conv_details.append(detail)
            time.sleep(1)

        write_package(slug, project_id, display_name, conv_details)
        signal_postal_service(slug)

        time.sleep(1)  # rate-limit between projects

    print("Done.")


if __name__ == "__main__":
    main()
