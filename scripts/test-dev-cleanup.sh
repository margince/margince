#!/usr/bin/env bash
# test-dev-cleanup.sh — prove a failed `make dev` leaves nothing running.
#
# The defect this exists for: the three readiness failures in dev.sh cleaned up
# with a bare asynchronous `kill`, and only a LINKED worktree installed the EXIT
# trap that follows TERM with KILL. On the primary stack an api or a Vite that
# did not promptly honour SIGTERM outlived the failed run and kept holding
# :8080 — and the next `make dev` reported the port as already in use, which
# reads as a different fault entirely and costs somebody the afternoon.
#
# Every function under test is LIFTED from dev.sh rather than reimplemented
# here: a copy would prove nothing about production.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dev="$root/scripts/dev.sh"
failures=0

# The fixtures this run started, torn down whatever happens. Two of them are
# `sleep 60` and one deliberately IGNORES TERM: an abort before the explicit
# teardown would leave the first pair for a minute and the third forever, and a
# gate that litters the machine it runs on is one people stop running.
# Recorded as `pid:signature`, and the signature is re-read before the KILL.
# A fixture torn down mid-run frees its pid, the kernel reissues numbers, and
# by the time the trap fires that number can belong to anything on the machine
# — including the editor of whoever is running this. Every fixture here is a
# `sleep` this script started, so its command line is proof of that, and a pid
# that no longer answers to it is somebody else's.
fixtures=""
# signature_of PID — the pid's command line with its whitespace removed.
#
# ONE function, read at both ends. `$fixtures` is a space-separated list, so a
# signature has to hold no spaces to survive it — and stripping at only one end
# compares a squeezed string against a spaced one, which never matches. The trap
# then skips every kill silently, which looks exactly like a run that left
# nothing behind.
signature_of() { # pid
    # `|| true` because `set -o pipefail` is in force and ps exits non-zero for
    # a pid it cannot see. A fixture backgrounded a moment ago is exactly that
    # case on a loaded machine, and without this the gate aborts inside
    # `remember` — before its own teardown is armed, which is the one moment it
    # cannot afford to.
    { ps -o command= -p "$1" 2>/dev/null || true; } | tr -d ' \t\n'
}
reap() {
    local entry pid signature
    for entry in $fixtures; do
        pid="${entry%%:*}"
        signature="${entry#*:}"
        case "$(signature_of "$pid")" in
            *"$signature"*) kill -9 "$pid" 2>/dev/null || true ;;
        esac
    done
}
trap reap EXIT
# remember PID — put a pid under the trap, signed by the command line it has now.
remember() { # pid
    # A SIGNATURE OR NOTHING. An empty one matches every command line the case
    # statement in reap tries it against — `*""*` is true of anything — so a pid
    # ps could not see would have licensed the trap to SIGKILL whatever holds
    # that number later. Retried once, because the miss is a scheduling
    # accident rather than a fact about the process.
    local signature
    signature=$(signature_of "$1")
    if [[ -z "$signature" ]]; then
        sleep 0.1
        signature=$(signature_of "$1")
    fi
    if [[ -z "$signature" ]]; then
        printf '  FAIL a fixture (pid %s) had no command line to sign, so the trap could not tell it from a recycled pid\n' "$1" >&2
        failures=$((failures + 1))
        return
    fi
    fixtures="$fixtures $1:$signature"
}

check() { # want got description
    if [ "$1" = "$2" ]; then
        printf '  ok   %s\n' "$3"
    else
        printf '  FAIL %s\n       want: %s\n       got:  %s\n' "$3" "$1" "$2" >&2
        failures=$((failures + 1))
    fi
}

# lift NAME — eval one function out of dev.sh, so every check below exercises
# production rather than a copy of it. Same awk as test-dev-isolation.sh, and
# for the same reason its comment gives.
lift() { # function name
    eval "$(awk -v fn="$1" '
        index($0, fn "() ") == 1 { inside = 1 }
        inside                   { print }
        inside && $0 == "}"      { exit }
    ' "$dev")"
}

# shellcheck source=scripts/lib-devstate.sh
. "$root/scripts/lib-devstate.sh"
# still_ours reads this to decide whether a recorded pid is one of OURS. It is a
# variable of dev.sh, so a lifted function needs it supplied — and supplying the
# real repository root is what makes the fixtures below have to look like real
# processes rather than pass on an empty pattern that matches everything.
repo_root="$root"
# And the stack's DATABASE, for the same reason: the api and the worker carry no
# repository path in their argv, so the name in their `--dsn` is the only thing
# that names them. A lifted still_ours refuses outright without it rather than
# quietly reading half of itself.
db="margince_dev_probe"
lift kill_pids
lift still_ours
lift stack_already_up
lift release_unconfirmed_stack

# alive answers whether a pid is still running, right now.
#
# Two functions rather than one, because the two questions have opposite
# deadlines. "Is it still up" is answered by the kernel immediately and a
# process that is going to die does not become MORE alive by waiting — so
# polling there buys nothing and spends two seconds on every passing check.
alive() { # pid → 1 when still running
    kill -0 "$1" 2>/dev/null && echo 1 || echo 0
}
# "Did it go" is the one that has to wait: SIGKILL is delivered asynchronously,
# kill_pids returns as soon as the signal is sent, and asking immediately can
# catch a process the kernel has not reaped yet — a flake, not a finding.
settled() { # pid → 0 once it is gone, 1 if it never goes
    local _
    for _ in $(seq 1 20); do
        kill -0 "$1" 2>/dev/null || { echo 0; return; }
        sleep 0.1
    done
    echo 1
}
echo "dev.sh: the cleanup outlasts a process that ignores TERM"

# A bare `kill` is a REQUEST. The whole point of the helper is that it stops
# waiting: a server wedged in a signal handler, or one whose runtime defers the
# signal, is exactly the orphan that goes on holding the port.
deaf="$(mktemp -d)"
# disowned so the shell does not narrate each kill onto the gate's output.
bash -c 'trap "" TERM; while :; do sleep 0.2; done' &
deaf_pid=$!
remember "$deaf_pid"
disown
kill_pids "$deaf_pid"
# Through alive(), which POLLS: SIGKILL is delivered asynchronously and
# kill_pids returns as soon as it is sent, so a one-shot read here can catch a
# process the kernel has not reaped and fail a gate for a reason that is not
# about the code.
check "0" "$(settled "$deaf_pid")" \
      "a process that ignores TERM is still taken down"
rm -rf "$deaf"

# AND A RECORDED PID THAT HAS ALREADY GONE DOES NOT ABORT THE CLEANUP.
#
# `ps` exits non-zero for a pid it cannot see, `set -o pipefail` is in force, and
# a recorded process that already exited is the ORDINARY case during a failed
# boot. Unswallowed, that status aborts kill_pids under `set -e` — and the EXIT
# trap that called it then leaves the api, the vite and the claim standing,
# which is the cleanup failing on the one path it exists for.
gone_probe() {
    local dead live
    bash -c 'exit 0' & dead=$!
    wait "$dead" 2>/dev/null || true
    bash -c 'trap "" TERM; while :; do sleep 0.2; done' & live=$!
    remember "$live"
    disown
    kill_pids "$dead" "$live"
    check "0" "$(settled "$live")" \
          "a recorded pid that already exited does not abort the cleanup of the ones still standing"
    kill -9 "$live" 2>/dev/null || true
}
gone_probe

# A PID THAT CHANGED HANDS DURING THE WAIT IS NOT KILLED.
#
# There are five seconds between the TERM and the SIGKILL, and a process that
# honours the TERM frees its pid inside them. The kernel reissues numbers, so a
# SIGKILL sent on the strength of the number alone lands on whatever took it —
# somebody's editor, their own shell.
#
# Asserted on the SOURCE rather than by racing the kernel for a recycled pid: a
# fixture that had to win that race would be flaky in the direction that matters,
# passing on the machine where the reuse never happened. What the reading has to
# show is that a signature is taken BEFORE the TERM and compared BEFORE the
# SIGKILL, and that the loop selects on the comparison rather than on liveness.
kill_signs="$(awk '/^kill_pids\(\)/{inside=1} inside{print} inside && /^}/{exit}' "$dev")"
check "1" "$([[ "$kill_signs" == *'ps -o command='*'kill "${pids[@]}"'* ]] && echo 1 || echo 0)" \
      "kill_pids reads each pid's command line before it sends the TERM"
check "0" "$(printf '%s' "$kill_signs" | grep -c 'kill -0')" \
      "and the wait selects on that signature rather than on the number still being alive"
check "1" "$([[ "$kill_signs" == *'*"$signature"*'*'kill -9'* ]] && echo 1 || echo 0)" \
      "so the SIGKILL reaches only a pid still answering to what was signed"

echo "dev.sh: an unconfirmed stack takes its processes with it"

# ours VAR NAME — start a background process this repo's own detector recognises
# as one of ours, and put its pid in VAR.
#
# still_ours reads the COMMAND LINE, so a bare `sleep` is not enough: argv[0]
# carries the repo root, exactly as a real api or vite does. That is the point
# of the fixture — a process the detector cannot recognise would make every
# "already up" check below pass by never finding anything.
#
# The pid comes back through a named variable rather than on stdout: a
# command substitution runs the `&` in a subshell, and the child does not
# outlive it — which looks exactly like a process that was correctly killed.
ours() { # varname name
    bash -c 'exec -a "$0" sleep 60' "$root/$2" &
    printf -v "$1" '%s' "$!"
    remember "$!"
    disown
}

# The primary worktree, which claims nothing. It is the case the trap used to
# skip: slug empty, no reservation, and three processes all the same.
#
# ALL THREE, because the release names all three and a fixture that left the
# worker out would let a regression drop it silently — the message this gate
# closes with says the worker comes down too.
sandbox() {
    state_root="$(mktemp -d)"
    mkdir -p "$state_root/$DEV_BASE_SLUG_DIR"
    ours be_pid dev-api
    ours fe_pid dev-vite
    ours worker_pid dev-worker
    printf 'SLUG=\nBACKEND_PID=%s\nFE_PID=%s\nWORKER_PID=%s\n' \
        "$be_pid" "$fe_pid" "$worker_pid" >"$state_root/$DEV_BASE_SLUG_DIR/env"
}
stop_sandbox() {
    kill "$be_pid" "$fe_pid" "$worker_pid" 2>/dev/null || true
    rm -rf "$state_root"
}

sandbox
slug=''
stack_recorded=0
STACK_CLAIM_PREEXISTING=0
MARGINCE_DEV_STATE_DIR="$state_root" release_unconfirmed_stack
check "0 0 0" "$(settled "$be_pid") $(settled "$fe_pid") $(settled "$worker_pid")" \
      "the primary stack's api, vite AND worker do not outlive a failed boot"
check "0" "$([[ -f "$state_root/$DEV_BASE_SLUG_DIR/env" ]] && echo 1 || echo 0)" \
      "and the record they would have been found by goes with them"
stop_sandbox

# What this run did not start, it must not stop. A second `make dev` on the
# primary stack fails its port check against the FIRST one — and taking that
# stack's processes down, or deleting the file `make dev-stop` finds it by,
# turns a clear "port in use" into a stack nobody can account for.
sandbox
slug=''
stack_recorded=0
# The DETECTOR decides, not the fixture: hard-coding the flag would prove the
# release honours it and nothing about whether a second `make dev` ever sets it.
STACK_CLAIM_PREEXISTING=0
if MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
    STACK_CLAIM_PREEXISTING=1
fi
check "1" "$STACK_CLAIM_PREEXISTING" \
      "a running stack is RECOGNISED as already up, from the record it left"

# THIS RUN'S OWN PROCESSES, started beside the stack it found. In production
# these are different processes from the recorded ones — the second `make dev`
# starts its own api and vite before failing its readiness probe — and the
# fixture has to keep them apart or it cannot tell "left the other stack alone"
# from "killed nothing at all".
recorded_be="$be_pid" recorded_fe="$fe_pid" recorded_worker="$worker_pid"
ours be_pid dev-api
ours fe_pid dev-vite
ours worker_pid dev-worker

MARGINCE_DEV_STATE_DIR="$state_root" release_unconfirmed_stack
check "1 1 1" "$(alive "$recorded_be") $(alive "$recorded_fe") $(alive "$recorded_worker")" \
      "and that stack is left running"
# AND THIS RUN'S OWN COME DOWN ANYWAY. Whose RECORD it is decides whether the
# file may be deleted and says nothing about the processes in front of it: a
# second run that found a stack up, started its own pair and then failed used to
# return before the kill, leaving exactly the orphan holding a port that this
# cleanup exists to prevent.
check "0 0 0" "$(settled "$be_pid") $(settled "$fe_pid") $(settled "$worker_pid")" \
      "while what THIS run started comes down with it"
check "1" "$([[ -f "$state_root/$DEV_BASE_SLUG_DIR/env" ]] && echo 1 || echo 0)" \
      "and keeps the record make dev-stop finds it by"
be_pid="$recorded_be" fe_pid="$recorded_fe" worker_pid="$recorded_worker"

# The api can die while its vite goes on serving :8080. Asking only about the
# api answers "nothing here" for a stack that is still holding the port, and the
# release then deletes the one record able to name the survivor.
kill "$be_pid" 2>/dev/null || true
while kill -0 "$be_pid" 2>/dev/null; do sleep 0.1; done
STACK_CLAIM_PREEXISTING=0
if MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
    STACK_CLAIM_PREEXISTING=1
fi
check "1" "$STACK_CLAIM_PREEXISTING" \
      "a stack whose api died but whose vite is still serving is still already up"

# AND THE WORKER ALONE. The two cases above are both answered by an api or a
# vite, so an implementation that never looked at WORKER_PID would pass them
# both — and a worker outliving its stack is the one of the three that holds no
# port, so nothing else reports it. It goes on draining the queue against a
# database the next run is about to migrate.
kill "$fe_pid" 2>/dev/null || true
while kill -0 "$fe_pid" 2>/dev/null; do sleep 0.1; done
STACK_CLAIM_PREEXISTING=0
if MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
    STACK_CLAIM_PREEXISTING=1
fi
check "1" "$STACK_CLAIM_PREEXISTING" \
      "and a stack down to its worker alone is STILL already up"
stop_sandbox

# And a stack that answered its own readiness probe is finished, not unconfirmed.
sandbox
slug=''
stack_recorded=1
STACK_CLAIM_PREEXISTING=0
MARGINCE_DEV_STATE_DIR="$state_root" release_unconfirmed_stack
check "1 1 1" "$(alive "$be_pid") $(alive "$fe_pid") $(alive "$worker_pid")" \
      "a CONFIRMED stack is not torn down by its own exit"
stop_sandbox

# TWO RUNS, ONE SLUG. The second reads the state file before the first has
# finished writing it, sees no live pid, and claims — and claim_stack stamps its
# own starter into the file, so the starter guard then reads the second run as
# the owner of a record the first is still booting against. It fails its port
# check moments later, and without this the release deletes the record naming
# the stack that won.
state_root="$(mktemp -d)"
mkdir -p "$state_root/$DEV_BASE_SLUG_DIR"
ours winner_be dev-api
ours winner_fe dev-vite
printf 'SLUG=\nSTARTER_PID=%s\nBACKEND_PID=%s\nFE_PID=%s\n' "$$" "$winner_be" "$winner_fe" \
    >"$state_root/$DEV_BASE_SLUG_DIR/env"
# This run started its own pair and lost the race.
ours be_pid dev-api
ours fe_pid dev-vite
worker_pid=''
slug=''
stack_recorded=0
STACK_CLAIM_PREEXISTING=0
MARGINCE_DEV_STATE_DIR="$state_root" release_unconfirmed_stack
check "1" "$([[ -f "$state_root/$DEV_BASE_SLUG_DIR/env" ]] && echo 1 || echo 0)" \
      "the record of a stack this run did not start survives its own failed boot"
check "1 1" "$(alive "$winner_be") $(alive "$winner_fe")" \
      "and so does that stack"
kill "$winner_be" "$winner_fe" "$be_pid" "$fe_pid" 2>/dev/null || true
rm -rf "$state_root"

# A SIBLING CHECKOUT IS NOT THIS ONE. /…/repo-old starts with /…/repo, so a
# substring match on the repository path reads that stack's processes as this
# stack's — and a recycled pid landing on one would answer "already up",
# stopping this run from cleaning up after itself.
#
# Driven against a SYNTHETIC root, not the real one: a fixture built from the
# real repository root proves the reading only on a machine whose path happens
# to be shaped like the assertion. CI's is not this one's, and a case that
# passes for a reason the test does not control is a case nobody can trust.
state_root="$(mktemp -d)"
mkdir -p "$state_root/$DEV_BASE_SLUG_DIR"
probe_root="$(mktemp -d)/checkout"
bash -c 'exec -a "$0" sleep 60' "$probe_root-old/api" &
sibling=$!
remember "$sibling"
disown
printf 'SLUG=\nBACKEND_PID=%s\n' "$sibling" >"$state_root/$DEV_BASE_SLUG_DIR/env"
slug=''
recognised=0
if repo_root="$probe_root" MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
    recognised=1
fi
check "0" "$recognised" \
      "a process from a SIBLING checkout is not read as this worktree's stack"
# And the same fixture under the root ITSELF is recognised, or the check above
# passes against a reading that matches nothing.
bash -c 'exec -a "$0" sleep 60' "$probe_root/api" &
own=$!
remember "$own"
disown
printf 'SLUG=\nBACKEND_PID=%s\n' "$own" >"$state_root/$DEV_BASE_SLUG_DIR/env"
recognised=0
if repo_root="$probe_root" MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
    recognised=1
fi
check "1" "$recognised" \
      "and one under the root itself still is"
kill "$sibling" "$own" 2>/dev/null || true
rm -rf "$state_root"

# THE API AND THE WORKER carry no repository path at all — they are launched as
# `./bin/api` and `./bin/worker` from the repo directory — so the path arms above
# cannot see them. What names them is the database in the `--dsn` they were given,
# and this is the case the two fixtures above cannot make: theirs carry a path.
#
# It is also the arm that was reading the repository's NAME. `*margince*` matched
# that word anywhere on the command line, so it answered yes for every sibling
# checkout's path too — and being first in the disjunction, it decided every case
# before the path narrowing was consulted.
dsn_probe() { # varname database
    bash -c 'exec -a "$0" sleep 60' "./bin/api --dsn postgres://u@h:5432/$2 --addr :18080" &
    printf -v "$1" '%s' "$!"
    remember "$!"
    disown
}
dsn_case() { # want database description
    state_root="$(mktemp -d)"
    mkdir -p "$state_root/$DEV_BASE_SLUG_DIR"
    dsn_probe probe "$2"
    printf 'SLUG=\nBACKEND_PID=%s\n' "$probe" >"$state_root/$DEV_BASE_SLUG_DIR/env"
    recognised=0
    if repo_root="$(mktemp -d)/nowhere" MARGINCE_DEV_STATE_DIR="$state_root" stack_already_up; then
        recognised=1
    fi
    check "$1" "$recognised" "$3"
    kill "$probe" 2>/dev/null || true
    rm -rf "$state_root"
}
# repo_root is pointed at a directory no fixture is under, so each of these is
# decided by the database name alone — otherwise a path arm could be answering
# and the arm under test would go unread.
dsn_case 1 "$db" "an api carrying THIS stack's database is recognised with no path in its argv"
dsn_case 0 "margince" "and one carrying the primary stack's database is not"
dsn_case 0 "${db}_2" "nor one whose database merely STARTS with this stack's"

# AND THAT PRODUCTION EVER ASKS. Everything above drives stack_already_up and
# the release directly, so it proves the pair agree — and would go on proving it
# after the `up` branch stopped wiring them together. The flag would then read 0
# on every second launch, and a failed one would take down a stack it did not
# start: the exact defect, with a green gate over it.
#
# So the WIRING is read out of dev.sh: the flag set from the detector's own
# answer, and set nowhere else.
# FOUR, because BOTH `up` paths ask: the claimed stack asks before claim_stack
# rewrites the file it reads, and the primary stack — which claims nothing — asks
# where it arms. Each writes a default and then the detector's answer. A fifth
# write is a third path nobody has thought about; a third is a path that stopped
# asking, which is the shape of the original defect.
preexisting_writes="$(grep -cE '^[[:space:]]*STACK_CLAIM_PREEXISTING=' "$dev" || true)"
check "4" "$preexisting_writes" \
      "both up paths write the preexisting flag, each a default and an answer"
# And every non-default write is the detector's own answer rather than a
# hard-coded 1, which would claim a preexisting stack on every launch and
# disable the release for good.
# Counted by ADJACENCY rather than with grep -A: overlapping context windows are
# merged into one, so a second pair a line apart would be reported as one match
# and a path that stopped asking would look identical to a path that never did.
answers="$(grep -cE '^[[:space:]]*STACK_CLAIM_PREEXISTING=1' "$dev" || true)"
wiring="$(awk '
    /^[[:space:]]*if .*stack_already_up; then$/ { armed = 1; next }
    /^[[:space:]]*STACK_CLAIM_PREEXISTING=1$/ { if (armed) n++ }
    { armed = 0 }
    END { print n + 0 }' "$dev")"
check "2 2" "$answers $wiring" \
      "and each one that is not the default sits directly under a stack_already_up test"

# EVERY RECORDED STACK IS SWEPT, not only the one this invocation happens to be.
#
# The sweep walks every state file on the machine, and each names a different
# slug with a database of its own. Asked with the sweeping run's `db`, still_ours
# answers about a stack nobody is sweeping: every other slug's processes read as
# not-ours and survive a command that promises to clear the machine.
sweep_probe() { # want database description — fixture argv carries `db`'s DSN
    local want="$1" ask="$2" description="$3" pid recognised=0
    bash -c 'exec -a "$0" sleep 60' "./bin/api --dsn postgres://u@h:5432/margince_dev_other" &
    pid=$!
    remember "$pid"
    disown
    if db="$ask" still_ours "$pid" "$ask"; then recognised=1; fi
    check "$want" "$recognised" "$description"
    kill "$pid" 2>/dev/null || true
}
sweep_probe 1 margince_dev_other "a recorded stack is recognised by ITS OWN database"
sweep_probe 0 margince_dev_mine  "and not by the database of the run doing the sweeping"

echo "dev.sh: one cleanup, and every failure path relies on it"

# The checks above prove the CLEANUP. They cannot prove the primary stack ever
# arms it — which is precisely what was missing — so the guard the arming sits
# under is read out of dev.sh itself.
#
# Read as DEPTH and guard together, not as "the last unindented `if` seen". An
# arming nested one level deeper — inside the claiming branch, which is where it
# used to live, or inside any other block — is the defect, and a scan that only
# remembered the last top-level guard would report the same answer for it.
nearest_guard() { # file → "<depth>:<the block the arming sits in>"
    awk '
        /^[[:space:]]*if /            { depth++; guard[depth] = $0 }
        /^[[:space:]]*elif /          { guard[depth] = $0 }
        /^[[:space:]]*fi([[:space:]]|$)/ { delete guard[depth]; depth-- }
        /trap .release_unconfirmed_stack. EXIT/ {
            g = guard[depth]
            sub(/^[[:space:]]+/, "", g)
            print depth ":" g
            found = 1
            exit
        }
        END { if (!found) print "NONE" }
    ' "$1"
}
check '1:if [[ "$cmd" == "up" ]]; then' "$(nearest_guard "$dev")" \
      "the cleanup is armed for every up, at the top level, not only for a stack that claims"

check "1" "$(grep -c "trap 'release_unconfirmed_stack' EXIT" "$dev")" \
      "and armed in exactly one place"

# The scan can still SEE the defect. A census that only ever reports clean is
# indistinguishable from one that stopped reading, so plant the arrangement that
# actually shipped — the arming inside the claiming branch — and require it.
planted="$(mktemp)"
printf 'if [[ -z "$slug" ]]; then\n  fe_port=8080\nelif [[ "$cmd" == "up" ]]; then\n  trap %s EXIT\nfi\n' \
    "'release_unconfirmed_stack'" >"$planted"
check '1:elif [[ "$cmd" == "up" ]]; then' "$(nearest_guard "$planted")" \
      "and it reports the claiming branch when the arming is nested back inside it"

# And a DEEPER nesting, which the last-guard-seen reading could not tell from
# the real thing: same top-level guard, arming reached only sometimes.
nested="$(mktemp)"
printf 'if [[ "$cmd" == "up" ]]; then\n  if [[ -n "$slug" ]]; then\n    trap %s EXIT\n  fi\nfi\n' \
    "'release_unconfirmed_stack'" >"$nested"
check '2:if [[ -n "$slug" ]]; then' "$(nearest_guard "$nested")" \
      "and it names the inner block when the arming is buried one level down"
rm -f "$planted" "$nested"

# No failure path spells a kill of its own. Two cleanups are one cleanup and one
# omission: the bare `kill` these replace was asynchronous, so the exit raced the
# processes it had only asked to stop.
# EVERY recorded pid, not just the api's. A readiness path that reintroduced
# `kill "$fe_pid"` alone would have left this silent, which is the shape of
# census failure this file is otherwise careful about.
# ONE pattern, read twice. The census below and the planted proof that it can
# see anything have to be the same reading, or the proof is about a pattern the
# census does not use — and the census is the half that goes quiet.
#
# A signal can be attached (`-TERM`, `-9`) or DETACHED (`-s TERM`), and the
# detached spelling is the one an option-only pattern walks straight past: `-s`
# matches as an option and `TERM` then has to be the pid. It is also exactly the
# edit somebody reaches for when a plain kill "did not work".
kill_of_recorded='^[[:space:]]*kill( +(-[^ ]+)( +[^ "$]+)?)* +"?\$\{?(be_pid|fe_pid|worker_pid)'
offenders="$(grep -nE "$kill_of_recorded" "$dev" || true)"
check "" "$offenders" \
      "no readiness failure kills a recorded pid itself — they all exit through the trap"

# And the scan can see one. Planting each spelling proves the pattern matches
# what a failure path would actually be written as, rather than only the one
# form the three paths happened to use.
planted_kill="$(printf '  kill "$fe_pid" 2>/dev/null || true\n  kill -TERM $worker_pid\n  kill "${be_pid}"\n  kill -s TERM "$fe_pid"\n  kill -s KILL ${worker_pid}\n' \
    | grep -cE "$kill_of_recorded" || true)"
check "5" "$planted_kill" \
      "and the scan recognises a bare kill, an attached signal, a braced pid, and both detached-signal spellings"

if [ "$failures" -gt 0 ]; then
    printf 'FAIL: %d check(s)\n' "$failures" >&2
    exit 1
fi
printf '==> dev cleanup: a failed boot takes its api, its vite and its worker with it\n'
