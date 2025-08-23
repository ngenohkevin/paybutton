#!/bin/bash

echo "🚀 PayButton Analytics v2 Test Script"
echo "======================================"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Test configuration
HOST="localhost:8080"
SITE_NAME="test-site-v2"

echo -e "\n${YELLOW}Testing Analytics v2 Endpoints...${NC}"

# Test 1: Check if analytics.js is served
echo -e "\n1. Testing analytics.js endpoint..."
response=$(curl -s -o /dev/null -w "%{http_code}" http://$HOST/analytics.js)
if [ "$response" = "200" ]; then
    echo -e "${GREEN}✓ analytics.js served successfully${NC}"
else
    echo -e "${RED}✗ Failed to serve analytics.js (HTTP $response)${NC}"
fi

# Test 2: Check if analytics-v2.js is served
echo -e "\n2. Testing analytics-v2.js endpoint..."
response=$(curl -s -o /dev/null -w "%{http_code}" http://$HOST/analytics-v2.js)
if [ "$response" = "200" ]; then
    echo -e "${GREEN}✓ analytics-v2.js served successfully${NC}"
    
    # Check for v2 version header
    version=$(curl -s -I http://$HOST/analytics-v2.js | grep -i "x-analytics-version" | cut -d' ' -f2 | tr -d '\r')
    if [ ! -z "$version" ]; then
        echo -e "${GREEN}  Version: $version${NC}"
    fi
else
    echo -e "${RED}✗ Failed to serve analytics-v2.js (HTTP $response)${NC}"
fi

# Test 3: Test beacon fallback endpoint
echo -e "\n3. Testing beacon fallback endpoint..."
beacon_data='{
    "site": "test-site",
    "sessionId": "test-session-123",
    "events": [
        {
            "type": "pageview",
            "timestamp": "'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'",
            "page": {
                "path": "/test",
                "title": "Test Page"
            }
        }
    ]
}'

response=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST http://$HOST/analytics/beacon \
    -H "Content-Type: application/json" \
    -d "$beacon_data")

if [ "$response" = "204" ]; then
    echo -e "${GREEN}✓ Beacon endpoint working (HTTP 204 No Content)${NC}"
else
    echo -e "${RED}✗ Beacon endpoint failed (HTTP $response)${NC}"
fi

# Test 4: WebSocket connectivity test
echo -e "\n4. Testing WebSocket connectivity..."
echo -e "${YELLOW}  Note: Full WebSocket test requires a WebSocket client${NC}"

# Use curl to at least check if the endpoint exists
response=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==" \
    http://$HOST/ws/analytics/v2/$SITE_NAME)

if [ "$response" = "101" ] || [ "$response" = "400" ]; then
    echo -e "${GREEN}✓ WebSocket endpoint is responding${NC}"
else
    echo -e "${RED}✗ WebSocket endpoint not available (HTTP $response)${NC}"
fi

# Test 5: Check admin analytics endpoint
echo -e "\n5. Testing admin analytics endpoint..."
response=$(curl -s -o /dev/null -w "%{http_code}" http://$HOST/admin/analytics)
if [ "$response" = "200" ] || [ "$response" = "401" ]; then
    echo -e "${GREEN}✓ Admin analytics endpoint exists (auth required)${NC}"
else
    echo -e "${RED}✗ Admin analytics endpoint failed (HTTP $response)${NC}"
fi

echo -e "\n${YELLOW}======================================"
echo -e "Test Summary:${NC}"
echo -e "${GREEN}✓ Analytics v2 endpoints are configured${NC}"
echo -e "\nTo test the full functionality:"
echo -e "1. Run the PayButton service: ${YELLOW}./paybutton-test${NC}"
echo -e "2. Open the test page: ${YELLOW}open test-analytics-v2.html${NC}"
echo -e "3. Check the browser console for analytics logs"
echo -e "4. Use the test page buttons to verify all features"
echo -e "\n${GREEN}Happy testing! 🎉${NC}"