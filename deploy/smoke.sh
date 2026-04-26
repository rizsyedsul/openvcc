#!/usr/bin/env bash
# End-to-end smoke test for the docker compose demo.
# Boots the stack, exercises the proxy, simulates an AWS-side outage,
# verifies traffic shifts to Azure, restores AWS, then tears down.

set -euo pipefail

cd "$(dirname "$0")"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
COMPOSE="docker compose -f compose.yaml"

cleanup() {
  echo "==> tearing down"
  $COMPOSE down -v
}
trap cleanup EXIT

echo "==> starting stack"
$COMPOSE up -d --build

echo "==> waiting for engine"
for _ in $(seq 1 60); do
  if curl -sf "$PROXY_URL/healthz" >/dev/null 2>&1 || curl -sf "$PROXY_URL/" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> waiting for both backends to be healthy"
for _ in $(seq 1 30); do
  if curl -sf http://localhost:9090/metrics 2>/dev/null \
      | grep -E '^openvcc_active_backends 2$' >/dev/null; then
    break
  fi
  sleep 1
done

echo "==> baseline: 30 requests should split across both clouds"
declare -A counts=(["aws"]=0 ["azure"]=0)
for _ in $(seq 1 30); do
  cloud=$(curl -sD - "$PROXY_URL/" -o /dev/null | awk -F': ' '/^X-Cloud/ {gsub(/\r/,"",$2); print $2}')
  if [[ -n "$cloud" ]]; then
    counts[$cloud]=$((${counts[$cloud]:-0} + 1))
  fi
done
echo "    aws=${counts[aws]:-0} azure=${counts[azure]:-0}"
[[ "${counts[aws]:-0}" -ge 5 && "${counts[azure]:-0}" -ge 5 ]] || {
  echo "FAIL: distribution looks wrong"; exit 1; }

echo "==> simulating aws outage"
$COMPOSE stop app-aws
sleep 8

echo "==> 30 requests should all go to azure now"
post_aws=0; post_azure=0
for _ in $(seq 1 30); do
  cloud=$(curl -sD - "$PROXY_URL/" -o /dev/null | awk -F': ' '/^X-Cloud/ {gsub(/\r/,"",$2); print $2}')
  case "$cloud" in
    aws) post_aws=$((post_aws+1)) ;;
    azure) post_azure=$((post_azure+1)) ;;
  esac
done
echo "    aws=$post_aws azure=$post_azure"
[[ "$post_aws" -eq 0 && "$post_azure" -ge 25 ]] || {
  echo "FAIL: failover did not redirect traffic"; exit 1; }

echo "==> restoring aws"
$COMPOSE start app-aws
sleep 8

echo "PASS"
