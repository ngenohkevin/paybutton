# PayButton Real-Time Notifications Plan

## Overview
Implement real-time balance notifications for BTC payments to provide instant success messages to websites using this service. No database changes required - service updates external databases but doesn't depend on them.

## Current State
- 60-second polling intervals for balance checking
- Telegram notifications + customer emails when balance detected
- Service used by multiple websites without database requirements
- BTC focus (USDT to be added later)

## Implementation Rules
- **PRESERVE EXISTING CODE** - Keep current polling system as fallback
- **NO OVERWRITING** - Only add new functionality alongside existing code
- **BACKWARD COMPATIBILITY** - Ensure existing websites continue working unchanged

## Implementation Phases

### Phase 1: Quick Win - Faster Polling (Optional Enhancement)
**Goal:** Reduce notification delays from 60s to 15s while keeping original as fallback
**Time:** 10 minutes
**Changes:**
- Add configurable polling interval (default 60s, optional 15s)
- Keep existing 60s polling as default behavior
- Allow opt-in to faster polling via parameter

### Phase 2: WebSocket Integration
**Goal:** Add real-time notifications alongside existing system
**Time:** 30 minutes
**Changes:**
- Add NEW `/ws/balance/:address` endpoint (don't modify existing endpoints)
- WebSocket connection management for active payment sessions
- Integrate WebSocket broadcasts into existing balance detection logic
- Keep all existing notification methods (Telegram, email) working
- Website integration: `ws://your-service/ws/balance/bc1q...`

### Phase 3: Blockonomics Webhook
**Goal:** Add instant notifications while keeping polling as backup
**Time:** 45 minutes
**Changes:**
- Add NEW `/webhook/btc` endpoint (separate from existing endpoints)
- Instant notifications when payments arrive
- Webhook verification and security
- Continue polling as fallback for reliability
- Existing payment flow remains unchanged

### Phase 4: Server-Sent Events (Optional)
**Goal:** Lightweight alternative to WebSocket
**Time:** 20 minutes
**Changes:**
- Add `/events/balance/:address` SSE endpoint
- Better browser compatibility for simple notifications

## Key Integration Points
- `internals/payment_processing/balance_ops.go:135-324` - Main polling logic
- `internals/server/server.go:218-228` - Add WebSocket/SSE endpoints
- `internals/payment_processing/process_payment.go:147-162` - Balance monitoring startup

## Implementation Status

### ✅ Phase 1: Fast Polling - COMPLETED
- Added `/payment-fast` endpoint with 15-second polling
- Original 60s polling preserved as fallback
- Fast mode clearly indicated in Telegram logs

### ✅ Phase 2: WebSocket Integration - COMPLETED  
- Added `/ws/balance/:address` endpoint for real-time updates
- WebSocket broadcasts balance confirmations instantly
- Automatic connection management with cleanup
- Frontend receives immediate "confirmed" status when payment detected

### ✅ Phase 3: Blockchain Webhooks - COMPLETED
- Added `/webhook/btc` endpoint for Blockonomics instant notifications  
- Webhook signature verification with HMAC-SHA256
- Instant WebSocket broadcasts when webhook received
- Automatic polling cleanup when webhook triggers
- Sub-second payment detection and processing

### 🔄 Phase 4: Server-Sent Events - PENDING
- Lightweight alternative to WebSocket for simple notifications

## Expected Benefits
- **Phase 1:** ✅ 15-second notification delays (vs 60s) - IMPLEMENTED
- **Phase 2:** ✅ Instant frontend updates via WebSocket - IMPLEMENTED  
- **Phase 3:** ✅ Sub-second notifications via webhooks - IMPLEMENTED
- **Overall:** Real-time payment experience for all websites

## Usage Example
```javascript
// Website connects to PayButton service
const ws = new WebSocket('ws://your-service/ws/balance/bc1q...');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  if (data.status === 'confirmed') {
    showSuccessMessage(); // Instant success notification
  }
};
```

## Testing Commands
```bash
# Run the service
go run main.go

# Test WebSocket connection
# Use browser console or WebSocket testing tool

# Test webhook endpoint
curl -X POST http://localhost:8080/webhook/btc \
  -H "Content-Type: application/json" \
  -d '{"address":"test_address","amount":0.001}'
```