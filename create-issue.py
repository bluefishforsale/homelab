#!/usr/bin/env python3
import json
import os
import sys
import urllib.request
import urllib.error

def create_issue(title, body_file, labels):
    """Create a GitHub issue using the API"""
    
    token = os.environ.get('GITHUB_TOKEN')
    if not token:
        print("❌ GITHUB_TOKEN not set")
        sys.exit(1)
    
    # Read body from file
    with open(body_file, 'r') as f:
        body = f.read()
    
    # Prepare API request
    url = "https://api.github.com/repos/bluefishforsale/homelab/issues"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json',
        'Content-Type': 'application/json'
    }
    
    data = {
        'title': title,
        'body': body,
        'labels': labels
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
            print(f"✅ Created: Issue #{result['number']}")
            print(f"🔗 {result['html_url']}")
            return result['number']
    except urllib.error.HTTPError as e:
        error_body = e.read().decode('utf-8')
        print(f"❌ Failed to create issue (HTTP {e.code})")
        print(f"Error: {error_body}")
        return None

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: create-issue.py <title> <body_file> [label1,label2,...]")
        sys.exit(1)
    
    title = sys.argv[1]
    body_file = sys.argv[2]
    labels = sys.argv[3].split(',') if len(sys.argv) > 3 else []
    
    print(f"Creating issue: {title}")
    create_issue(title, body_file, labels)
