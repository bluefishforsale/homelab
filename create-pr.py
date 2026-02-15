#!/usr/bin/env python3
import json
import os
import sys
import urllib.request
import urllib.error

def create_pr(title, body_file, head, base="master"):
    """Create a GitHub PR using the API"""
    
    token = os.environ.get('GITHUB_TOKEN')
    if not token:
        print("❌ GITHUB_TOKEN not set")
        sys.exit(1)
    
    # Read body from file
    with open(body_file, 'r') as f:
        body = f.read()
    
    # Prepare API request
    url = "https://api.github.com/repos/bluefishforsale/homelab/pulls"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json',
        'Content-Type': 'application/json'
    }
    
    data = {
        'title': title,
        'body': body,
        'head': head,
        'base': base
    }
    
    # Make request
    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode('utf-8'),
        headers=headers,
        method='POST'
    )
    
    try:
        with urllib.request.urlopen(req) as response:
            result = json.loads(response.read().decode('utf-8'))
            print(f"✅ Created: PR #{result['number']}")
            print(f"🔗 {result['html_url']}")
            print(f"📝 Title: {result['title']}")
            print(f"🌿 {result['head']['ref']} → {result['base']['ref']}")
            return result['number']
    except urllib.error.HTTPError as e:
        error_body = e.read().decode('utf-8')
        print(f"❌ Failed to create PR (HTTP {e.code})")
        print(f"Error: {error_body}")
        return None

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: create-pr.py <title> <body_file> <head_branch> [base_branch]")
        sys.exit(1)
    
    title = sys.argv[1]
    body_file = sys.argv[2]
    head = sys.argv[3]
    base = sys.argv[4] if len(sys.argv) > 4 else "master"
    
    print(f"Creating PR: {title}")
    print(f"Branch: {head} → {base}")
    print("")
    create_pr(title, body_file, head, base)
