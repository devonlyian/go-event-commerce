#!/usr/bin/env bash
set -euo pipefail

payload='{"customer_id":"customer-001","amount":129.99}'

echo "POST /orders"
response=$(curl -sS -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d "$payload")

if command -v jq >/dev/null 2>&1; then
  echo "$response" | jq
else
  echo "$response"
fi
