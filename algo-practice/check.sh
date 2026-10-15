#!/usr/bin/env bash
# Runs every exercise (expected to FAIL until solved) and every solution (expected to PASS).
set -u
cd "$(dirname "$0")"

status() { if [ "$1" -eq 0 ]; then echo "PASS"; else echo "FAIL"; fi; }

echo "=== exercises (FAIL until you solve them) ==="
for d in ex0*/; do
  d=${d%/}
  go test "./$d/" >/dev/null 2>&1
  printf '%-26s %s\n' "$d" "$(status $?)"
done

echo
echo "=== solutions (expect PASS) ==="
go test ./solutions/... >/dev/null 2>&1
printf '%-26s %s\n' "solutions (go test)" "$(status $?)"
go test -race ./solutions/... >/dev/null 2>&1
printf '%-26s %s  (-race)\n' "solutions" "$(status $?)"
