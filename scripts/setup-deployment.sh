#!/bin/bash

# Deployment Setup Helper Script for Paybutton
# This script helps you configure the GitHub secrets needed for deployment

set -e

echo "==================================="
echo "Paybutton Deployment Setup Helper"
echo "==================================="
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if gh CLI is installed
if ! command -v gh &> /dev/null; then
    echo -e "${RED}GitHub CLI (gh) is not installed.${NC}"
    echo "Please install it from: https://cli.github.com/"
    echo "Or run: brew install gh (on macOS)"
    echo "Or run: sudo apt install gh (on Ubuntu/Debian)"
    exit 1
fi

# Check if authenticated
if ! gh auth status &> /dev/null; then
    echo -e "${YELLOW}You need to authenticate with GitHub first.${NC}"
    gh auth login
fi

echo -e "${GREEN}✓ GitHub CLI is installed and authenticated${NC}"
echo ""

# Function to set a secret
set_secret() {
    local secret_name=$1
    local secret_value=$2
    local repo=$3

    echo -n "Setting $secret_name... "
    if gh secret set "$secret_name" --body "$secret_value" --repo "$repo" 2>/dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo "Failed to set $secret_name. Please set it manually in GitHub."
    fi
}

# Get repository name
echo "Enter your GitHub repository (format: username/repo):"
read -r REPO

# Verify repository exists
if ! gh repo view "$REPO" &> /dev/null; then
    echo -e "${RED}Repository $REPO not found or you don't have access.${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Repository verified${NC}"
echo ""

echo "==================================="
echo "Dokploy Configuration"
echo "==================================="
echo ""

echo "Enter your Dokploy Webhook URL:"
echo "(Get it from: Dokploy Dashboard → Your App → Deployments → Webhook URL)"
read -r DOKPLOY_WEBHOOK_URL

echo ""
echo "==================================="
echo "Setting GitHub Secrets"
echo "==================================="
echo ""

# Set the webhook secret
set_secret "DOKPLOY_WEBHOOK_URL" "$DOKPLOY_WEBHOOK_URL" "$REPO"

echo ""
echo "==================================="
echo "Setup Complete!"
echo "==================================="
echo ""
echo -e "${GREEN}✓ GitHub secrets have been configured${NC}"
echo ""
echo "Next steps:"
echo "1. Configure your Dokploy application to use image: ghcr.io/$REPO:latest"
echo "2. Commit and push the .github/workflows directory to your repository"
echo "3. The deployment will trigger automatically on push to main/master"
echo "4. Monitor the deployment in the GitHub Actions tab"
echo "5. Check your application in Dokploy dashboard"
echo ""
echo "To trigger a manual deployment:"
echo "  gh workflow run deploy-dokploy.yml --repo $REPO"
echo ""
echo "To view workflow runs:"
echo "  gh run list --repo $REPO"
echo ""
echo "To view container packages:"
echo "  Visit: https://github.com/$REPO/pkgs/container/$(basename $REPO)"
echo ""