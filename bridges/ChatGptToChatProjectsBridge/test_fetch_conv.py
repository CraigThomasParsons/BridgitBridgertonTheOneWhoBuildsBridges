import sys
import os
from pathlib import Path

sys.path.append(str(Path("bin").resolve()))
from extract_projects import make_session, auth_headers

def main():
    session, jwt = make_session()
    device_id = os.getenv("CHATGPT_DEVICE_ID")
    
    # Just grab an ID from the recent output, for instance: 699a189b-5908-8325-bad8-88a8d13cd098
    conv_id = "699a189b-5908-8325-bad8-88a8d13cd098"
    url = f"https://chatgpt.com/backend-api/conversation/{conv_id}"
    
    print(f"Testing {url} ...")
    resp = session.get(url, headers=auth_headers(jwt, device_id), timeout=10)
    print(f"Status: {resp.status_code}")
    if resp.status_code == 200:
        data = resp.json()
        print(f"Title: {data.get('title')}")
        mapping = data.get("mapping", {})
        print(f"Mapping keys: {len(mapping.keys())}")
    else:
        print(resp.text[:500])

if __name__ == "__main__":
    main()
