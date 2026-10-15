#!/usr/bin/env bash
# Runs every exercise (expected to FAIL) and every solution (expected to PASS).
set -u
cd "$(dirname "$0")"

status() { if [ "$1" -eq 0 ]; then echo "PASS"; else echo "FAIL"; fi; }

echo "=== exercises (expect FAIL) ==="
for d in ex01_order_service ex02_shipping_planner ex03_payment_client \
         ex04_rate_poller ex06_batch_processor ex08_url_fetcher; do
  go test "./$d/" >/dev/null 2>&1
  printf '%-24s %s\n' "$d" "$(status $?)"
done
go test -race ./ex07_ttl_cache/ >/dev/null 2>&1
printf '%-24s %s  (-race)\n' "ex07_ttl_cache" "$(status $?)"
( cd ex05_shard_aggregator && go test ./... >/dev/null 2>&1 )
printf '%-24s %s\n' "ex05_shard_aggregator" "$(status $?)"

echo
echo "=== solutions (expect PASS) ==="
go test ./solutions/... >/dev/null 2>&1
printf '%-24s %s\n' "solutions (go test)" "$(status $?)"
go test -race ./solutions/... >/dev/null 2>&1
printf '%-24s %s  (-race)\n' "solutions" "$(status $?)"
( cd solutions/ex05 && go test ./... >/dev/null 2>&1 )
printf '%-24s %s\n' "solutions/ex05" "$(status $?)"
( cd solutions/ex05 && go test -race ./... >/dev/null 2>&1 )
printf '%-24s %s  (-race)\n' "solutions/ex05" "$(status $?)"
