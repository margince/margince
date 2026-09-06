#!/usr/bin/env bash
# test-testdb-redis.sh — prove both integration entry points settle on the SAME
# Redis address, and that an override reaches either of them.
#
# The failure this replaces: test-integration-one.sh, invoked the way its own
# usage block invites, left MARGINCE_TEST_REDIS unset. The Redis-using fixtures
# fail loudly and never skip, so nine tests in one package failed naming Redis
# and telling the reader to run `make db-up` — on a machine where Redis was
# already up. The message pointed at the environment and its remedy was already
# done, so it cost a detour to establish the tests had nothing to do with the
# change under test.
#
# The lane's posture is that a thinner run must never read as a passing one.
# This is the mirror of that: a run must never read as broken when it is only
# under-provisioned by its own launcher.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
failures=0

check() { # want got description
    if [ "$1" = "$2" ]; then
        printf '  ok   %s\n' "$3"
    else
        printf '  FAIL %s\n       want: %s\n       got:  %s\n' "$3" "$1" "$2" >&2
        failures=$((failures + 1))
    fi
}

# What the script route resolves, from a shell carrying neither variable.
resolved() { # [REDIS_PORT]
    env -u MARGINCE_TEST_REDIS -u MARGINCE_TEST_REDIS_DB ${1:+REDIS_PORT="$1"} \
        bash -c "source '$root/scripts/lib-testdb.sh'; resolve_test_redis; echo \"\$MARGINCE_TEST_REDIS\""
}

# What the make route resolves, asked of make itself rather than parsed out of
# the Makefile: a regex over the assignment would agree with a line that no
# longer takes effect.
from_make() { # [REDIS_PORT]
    # Asked as a make VARIABLE EXPANSION, through a throwaway target appended
    # with a second -f. `make -p` reports the assignment unexpanded — the answer
    # would be the literal `localhost:$(REDIS_PORT)`, which agrees with nothing
    # and proves nothing.
    ( cd "$root/backend" \
        && make -s --no-print-directory ${1:+REDIS_PORT="$1"} \
             -f Makefile -f <(printf 'margince-test-redis-addr:;@echo $(MARGINCE_TEST_REDIS)\n') \
             margince-test-redis-addr )
}

echo "the two integration entry points agree on the Redis address"

# THE DEFAULT. Two entry points that disagree here send one lane at a Redis the
# other is not using, and the fixtures FLUSHDB — so a disagreement is a
# corruption of somebody else's run, not merely a miss.
script_default="$(resolved)"
make_default="$(from_make)"
check "$make_default" "$script_default" \
    "the script route resolves the same default the make route does ($make_default)"

# AND AN OVERRIDE REACHES BOTH. A default that agreed while an override reached
# only one is the same defect one turn of the knob later.
check "$(from_make 26379)" "$(resolved 26379)" \
    "REDIS_PORT reaches both routes"

# The address is never empty, which is the specific shape the failure took: the
# variable was ABSENT rather than wrong, and the fixtures reported that as a
# provisioning problem. A check that only compared the two routes would pass on
# two empties.
check "present" "${script_default:+present}" \
    "the resolved address is not empty"

if [ "$failures" -ne 0 ]; then
    echo "FAIL: $failures check(s) failed" >&2
    exit 1
fi
echo "OK: both entry points resolve one Redis address"
