#!/usr/bin/env python3
import json
import os
import sys
import urllib.request

def comment_on_issue(issue_number, comment):
    """Add a comment to a GitHub issue"""
    
    token = os.environ.get('GITHUB_TOKEN')
    if not token:
        print("❌ GITHUB_TOKEN not set")
        sys.exit(1)
    
    url = f"https://api.github.com/repos/bluefishforsale/homelab/issues/{issue_number}/comments"
    headers = {
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github.v3+json',
        'Content-Type': 'application/json'
    }
    
    data = {'body': comment}
    
    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode('utf-8'),
        headers=headers,
        method='POST'
    )
    
    with urllib.request.urlopen(req) as response:
        result = json.loads(response.read().decode('utf-8'))
        print(f"✅ Comment added to Issue #{issue_number}")
        print(f"🔗 {result['html_url']}")

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: update-issue.py <issue_number> <comment>")
        sys.exit(1)
    
    issue_number = sys.argv[1]
    comment = sys.argv[2]
    
    comment_on_issue(issue_number, comment)
