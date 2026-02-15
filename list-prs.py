#!/usr/bin/env python3
import json
import os
import sys
import urllib.request

def list_prs(state="open"):
    """List GitHub PRs"""
    
    token = os.environ.get('GITHUB_TOKEN')
    if not token:
        print("❌ GITHUB_TOKEN not set")
        sys.exit(1)
    
    url = f"https://api.github.com/repos/bluefishforsale/homelab/pulls?state={state}"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json'
    }
    
    req = urllib.request.Request(url, headers=headers)
    
    with urllib.request.urlopen(req) as response:
        prs = json.loads(response.read().decode('utf-8'))
        
        if not prs:
            print(f"No {state} pull requests found.")
            return
        
        print(f"Found {len(prs)} {state} pull request(s):\n")
        
        for pr in prs:
            print(f"PR #{pr['number']}: {pr['title']}")
            print(f"  🌿 {pr['head']['ref']} → {pr['base']['ref']}")
            print(f"  🔗 {pr['html_url']}")
            print(f"  👤 {pr['user']['login']}")
            print(f"  📅 {pr['created_at']}")
            print()

if __name__ == '__main__':
    state = sys.argv[1] if len(sys.argv) > 1 else "open"
    list_prs(state)
