# Implementation Plan — ChatGptToChatProjectsBridge

## Overview

This tool fetches ChatGPT Projects from the internal API and materializes each one as a folder in `ChatProjects/`, routed via **ThePostalService**.

---

## 1. ChatGPT Project ID / URL Scheme

### URL Pattern (when a conversation is open inside a project)
```
https://chatgpt.com/g/g-p-696ea607031c81919264ed4b3bbe6c75/c/69706090-b100-832d-9702-855d6766f717
                        ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                              project_id (unique key)                 conversation_id (UUID)
```

### Project Name Slug (used in navigation links / DOM)
```
g-p-696ea607031c81919264ed4b3bbe6c75-code-github-com-learning-projects
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
         project_id (unique key)          slugified display name
```

### How to Parse the Unique Key
- The project ID is always the prefix: `g-p-` + exactly 32 hex characters = 36 chars total.
- Everything after position 36 (and the following `-`) is the human-readable slug derived from the display name.
- Example: `"Code: Github.com Learning Projects"` → `code-github-com-learning-projects`

### Folder Naming Convention (in `ChatProjects/`)
```
{project_id}-{slugified-name}/
```
e.g. `g-p-696ea607031c81919264ed4b3bbe6c75-code-github-com-learning-projects/`

The folder is **uniquely indexed** by `project_id`. The slug suffix is cosmetic and mirrors the display name.

---

## 2. What This Tool Does

```
ChatGPT Internal API
        │
        ▼
bin/extract_projects.py      ← hits /backend-api/gizmos/snorlax/sidebar?owned_only=true
        │  produces one JSON envelope per project
        ▼
outbox/{project_id}-{slug}/
    letter.toml              ← recipient = "chat-projects"
    project.json             ← id, display_name, slug, conversation_ids[]
        │
        ▼
ThePostalService             ← receives postal.signal, moves folder
        │
        ▼
ChatProjects/inbox/{project_id}-{slug}/
```

---

## 3. ChatGPT Internal API Endpoints

### Confirmed: Sidebar / Project List

```
GET /backend-api/gizmos/snorlax/sidebar?owned_only=true&conversations_per_gizmo=0
GET /backend-api/gizmos/snorlax/sidebar?owned_only=true&conversations_per_gizmo=0&cursor={cursor}
```

`snorlax` is the internal OpenAI codename for this sidebar component — not a variable.
`conversations_per_gizmo=0` skips embedding conversation previews (faster); set to `N` to get the last N conversation IDs per project inline.
Response is cursor-paginated; keep fetching while a `cursor` field is present in the response.

### Confirmed: Required Headers

| Header | Source |
|---|---|
| `Authorization: Bearer {JWT}` | Obtained from `GET /api/auth/session` → `.accessToken` |
| `Cookie: __Secure-next-auth.session-token=...` | Stored in `.env`, chunked as `.0` / `.1` if long |
| `oai-device-id` | A stable UUID you generate once and reuse |
| `oai-language` | `en-US` |
| `oai-client-build-number` | Any recent value (e.g. `4927083`); unlikely to be validated strictly |
| `oai-client-version` | Any recent value; same caveat |

### Credentials Storage (`.env`)

```dotenv
# Refresh this by grabbing Set-Cookie from a browser login
CHATGPT_SESSION_TOKEN=eyJ...
# Stable; generate once with: python -c "import uuid; print(uuid.uuid4())"
CHATGPT_DEVICE_ID=f26bf227-b7d2-494a-96a2-00ca9243d92d
```

The Bearer JWT is **short-lived** (~24h). The script must call `GET /api/auth/session` first
(sending the session token cookie) to get a fresh JWT before hitting the sidebar endpoint.

### Still To Confirm

| Purpose | Likely Endpoint |
|---|---|
| Conversations inside a project | `GET /backend-api/conversations?project_id={id}&limit=100` |

---

## 4. Proposed File Structure

```text
Code/ChatGptToChatProjectsBridge/
├── bin/
│   └── extract_projects.py        # Hits ChatGPT internal API, writes outbox envelopes
├── outbox/                        # One subfolder per project, picked up by ThePostalService
│   └── {project_id}-{slug}/
│       ├── letter.toml            # recipient = "chat-projects"
│       └── project.json           # project metadata + conversation_ids[]
├── inbox/                         # Unused for now (this service only sends)
├── logs/
├── registry/
│   └── profile.toml               # Registers this service with ThePostalService
├── systemd/
│   ├── chatgptprojects.timer      # Runs once per day/week
│   └── chatgptprojects.service    # Executes bin/extract_projects.py
├── install.sh                     # Symlinks systemd units, sets up Python venv
├── register.sh                    # Copies profile.toml → ThePostalService/registry/
├── .env.example                   # SESSION_TOKEN=...
└── implementation_plan.md
```

---

## 5. `project.json` Envelope Format

```json
{
  "project_id": "g-p-696ea607031c81919264ed4b3bbe6c75",
  "slug": "g-p-696ea607031c81919264ed4b3bbe6c75-code-github-com-learning-projects",
  "display_name": "Code: Github.com Learning Projects",
  "conversation_ids": [
    "69706090-b100-832d-9702-855d6766f717"
  ],
  "fetched_at": "2026-02-26T00:00:00Z"
}
```

---

## 6. `letter.toml` Format

```toml
recipient = "chat-projects"
package_id = "g-p-696ea607031c81919264ed4b3bbe6c75"
```

ThePostalService reads this to know where to deliver the folder.

---

## 7. Registry Profile (`registry/profile.toml`)

```toml
service_name = "chatgpt-projects-bridge"
outbox_path  = "/home/craigpar/Code/ChatGptToChatProjectsBridge/outbox"
inbox_path   = "/home/craigpar/Code/ChatGptToChatProjectsBridge/inbox"
```

---

## 8. `bin/extract_projects.py` — Logic Outline

```
1. Load SESSION_TOKEN and DEVICE_ID from .env
2. GET /api/auth/session  (sends session cookie → receives fresh Bearer JWT)
3. Paginate: GET /backend-api/gizmos/snorlax/sidebar?owned_only=true&conversations_per_gizmo=0
   - Loop while response contains a `cursor` field, passing &cursor={cursor} each time
4. For each project in response:
   a. slug  = f"{project_id}-{slugify(display_name)}"
   b. GET /backend-api/conversations?project_id={project_id}&limit=100  (paginate as needed)
   c. Write outbox/{slug}/project.json
   d. Write outbox/{slug}/letter.toml
   e. Publish postal.signal to RabbitMQ:
      { "event": "package_ready", "sender": "chatgpt-projects-bridge", "package_id": project_id }
```

---

## 9. Chosen Approach

**Option B (Direct Internal API)** — same as ChatGptBackup. No headless browser needed. We authenticate with the session cookie directly and hit the internal endpoints.

---

## 10. Open Questions / Next Steps

- [x] Confirm exact API endpoint for listing projects → `/backend-api/gizmos/snorlax/sidebar?owned_only=true`
- [ ] Confirm conversation list endpoint per project
- [ ] Scaffold `bin/extract_projects.py`
- [ ] Create `registry/profile.toml` and `register.sh`
- [ ] Scaffold `systemd/` units and `install.sh`
- [ ] Add `ChatProjects` service to ThePostalService registry as recipient
