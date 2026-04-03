import sys
import os
from pathlib import Path

# Add bin to path to import helpers
sys.path.append(str(Path("bin").resolve()))
from extract_projects import make_session, auth_headers

def main():
    session, jwt = make_session()
    device_id = os.getenv("CHATGPT_DEVICE_ID")
    
    # We will grab the first project ID to test 
    from extract_projects import fetch_projects, extract_project_id
    projects = fetch_projects(session, jwt, device_id)
    if not projects:
        print("No projects found.")
        return
        
    project_id = extract_project_id(projects[-2]) # Grab the 2nd to last just in case the last is empty
    print(f"Testing with project ID: {project_id}")

    urls = [
        f"https://chatgpt.com/backend-api/conversations?project_id={project_id}",
        f"https://chatgpt.com/backend-api/gizmos/{project_id}/conversations",
        f"https://chatgpt.com/backend-api/projects/{project_id}"
    ]
    
    for url in urls:
        print(f"\n--- Testing {url} ---")
        try:
            resp = session.get(url, headers=auth_headers(jwt, device_id), timeout=10)
            print(f"Status: {resp.status_code}")
            if resp.status_code == 200:
                data = resp.json()
                print("KEYS:", list(data.keys()))
                items = data.get("items", [])
                print(f"Items type: {type(items)}")
                if isinstance(items, list):
                    print(f"Found {len(items)} items.")
                    if items:
                        print(f"First item: {items[0].get('id')} / {items[0].get('title')}")
                else:
                    print("Items is not a list. Full data preview:")
                    print(str(data)[:500])
                    
            else:
                print(resp.text[:200])
        except Exception as e:
            print(f"Error: {e}")

if __name__ == "__main__":
    main()
