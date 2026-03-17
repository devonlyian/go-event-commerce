#!/usr/bin/env bash
set -euo pipefail

payload='{"customer_id":"customer-001","amount":129.99}'

echo "POST /orders"
response=$(curl -sS -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d "$payload")

if command -v jq >/dev/null 2>&1; then
  echo "$response" | jq
  order_id=$(echo "$response" | jq -r '.order.id // empty')
else
  echo "$response"
  order_id=$(printf '%s' "$response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
fi

if [[ -z "${order_id:-}" ]]; then
  echo "failed to extract order id"
  exit 1
fi

echo
echo "GET /orders/$order_id (polling for payment result)"
for _ in {1..15}; do
  current=$(curl -sS "http://localhost:8080/orders/$order_id")
  if command -v jq >/dev/null 2>&1; then
    echo "$current" | jq
    status=$(echo "$current" | jq -r '.order.status // empty')
  else
    echo "$current"
    status=$(printf '%s' "$current" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')
  fi

  if [[ "$status" == "paid" || "$status" == "payment_failed" ]]; then
    exit 0
  fi

  sleep 1
done

echo "payment result was not reflected within timeout"
exit 1
