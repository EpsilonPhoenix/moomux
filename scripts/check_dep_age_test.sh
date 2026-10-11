#!/usr/bin/env bash
# Regression test for check_dep_age.sh. Run directly:
#   ./scripts/check_dep_age_test.sh
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check_dep_age.sh"
fail=0

# No cooldown: everything passes.
if ! MIN_AGE_HOURS=0 "$script" >/dev/null; then
  echo "FAIL: MIN_AGE_HOURS=0 should pass"
  fail=1
fi

# A cooldown longer than Go has existed: everything is too new, and the
# report must name both a go.mod module and a CI tool pin.
if out="$(MIN_AGE_HOURS=1000000 "$script" 2>&1)"; then
  echo "FAIL: MIN_AGE_HOURS=1000000 should fail"
  fail=1
fi
for want in golang.org/x/sys@ golang.org/x/vuln@; do
  case "$out" in
    *"$want"*) echo "ok: flags $want" ;;
    *) echo "FAIL: report missing $want:"; echo "$out"; fail=1 ;;
  esac
done

exit "$fail"
