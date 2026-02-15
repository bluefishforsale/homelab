#!/bin/bash
# Create GitHub labels using API

set -e

REPO="bluefishforsale/homelab"
API_BASE="https://api.github.com"

echo "Creating labels for $REPO..."
echo ""

# Function to create a label
create_label() {
  local name="$1"
  local color="$2"
  local description="$3"
  
  echo -n "Creating label: $name ... "
  
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "$API_BASE/repos/$REPO/labels" \
    -d "{\"name\":\"$name\",\"color\":\"$color\",\"description\":\"$description\"}")
  
  HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
  BODY=$(echo "$RESPONSE" | sed '$d')
  
  if [ "$HTTP_CODE" = "201" ]; then
    echo "✅"
  elif [ "$HTTP_CODE" = "422" ] && echo "$BODY" | grep -q "already_exists"; then
    echo "⚠️  (already exists)"
  else
    echo "❌ (HTTP $HTTP_CODE)"
    echo "$BODY" | head -3
  fi
}

# Create labels
create_label "agent:homelab-gitops" "0E8A16" "Tasks for Homelab GitOps agent"
create_label "priority:critical" "D73A4A" "Critical priority"
create_label "priority:high" "FF6B6B" "High priority"
create_label "priority:medium" "FFA94D" "Medium priority"
create_label "priority:low" "94D82D" "Low priority"
create_label "status:blocked" "FBCA04" "Blocked - waiting on external dependency"
create_label "status:in-progress" "0075CA" "Currently being worked on"
create_label "status:ready" "7057FF" "Ready to be worked on"

echo ""
echo "✅ Labels setup complete!"
