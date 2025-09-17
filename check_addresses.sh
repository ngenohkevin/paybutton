#!/bin/bash

# Script to check specific Bitcoin addresses for the Sep 17 transactions
echo "🔍 Checking addresses for missing transactions..."
echo ""

# These are addresses that might have received the missing transactions
# You can add more addresses here if you know which ones were generated
addresses=(
    # Add your known addresses here
    "bc1qrnam0k28ms6pq6u2lvj6sw3l005kzfa5akd6c0"
    # Add more addresses as needed
)

# Function to check balance using blockchain.info
check_address() {
    local addr=$1
    echo "Checking $addr..."

    # Check on blockchain.info
    balance=$(curl -s "https://blockchain.info/q/addressbalance/$addr" 2>/dev/null)

    if [ -z "$balance" ] || [ "$balance" == "0" ]; then
        echo "  ❌ No balance found"
    else
        btc=$(echo "scale=8; $balance / 100000000" | bc)
        echo "  ✅ Balance: $btc BTC"

        # Get transaction history
        echo "  📜 Recent transactions:"
        curl -s "https://blockchain.info/rawaddr/$addr?limit=5" 2>/dev/null | \
            jq -r '.txs[] | "    - \(.time | strftime("%Y-%m-%d %H:%M")) : \(.out[] | select(.addr == "'$addr'") | .value / 100000000) BTC"' 2>/dev/null || \
            echo "    Could not fetch transaction details"
    fi
    echo ""
}

# Check each address
for addr in "${addresses[@]}"; do
    check_address "$addr"
done

echo "🔍 To find more addresses:"
echo "1. Check your Blockonomics dashboard at https://www.blockonomics.co"
echo "2. Look for addresses with payments that aren't in your wallet"
echo "3. Check the address generation logs from Sep 17"