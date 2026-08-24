#!/usr/bin/env bash
# Where a dev stack's identity comes from, spelled once for every script that
# needs it. Source this; don't execute it.
#
# There are three writers of this invariant — dev.sh starts a stack, dev-logs.sh
# reads its log, and the test suite checks both — and the first version of the
# per-worktree change had dev.sh alone knowing the answer. dev-logs.sh kept
# composing the old path, so `make dev-logs` looked for a file `make dev` no
# longer wrote. That is the failure this file exists to make impossible: one
# spelling, sourced, rather than three that happen to agree.

# The registry and log root. ONE directory per machine, deliberately outside the
# repository: it used to be <worktree>/.tmp/dev/, so a second worktree read an
# empty registry and claimed a Redis logical database that was already in use.
# Two stacks then shared one consumer group and quietly ate each other's events.
#
# MARGINCE_DEV_STATE_DIR exists so the tests can point this somewhere
# disposable. Nothing else should set it.
dev_state_root() {
  printf '%s' "${MARGINCE_DEV_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/margince/dev}"
}

# The primary worktree's stack has no slug, so its state directory needs a name
# anyway. It is reserved: DEV_SLUG may not take it, or two stacks would share one
# directory and each stop would remove the other's pids and claim.
DEV_BASE_SLUG_DIR=_base

# A slug reaches four places that each refuse different characters: a filesystem
# path, a CREATE DATABASE identifier, an S3 bucket name, and a Redis key prefix.
# Fold to the narrowest of them rather than validating in four places — a branch
# or directory name is the usual source, and `feat/ai-keys` is an ordinary one.
#
# Underscore folds to a hyphen rather than surviving, and that is not cosmetic:
# the bucket name has to be derived from the slug INJECTIVELY. While `_` was
# legal in a slug and illegal in a bucket, `a_b` and `a-b` were two worktrees
# sharing one bucket — the same defect as a shared Redis database, in the store
# that holds attachment bytes. With `_` gone from the slug alphabet the mapping
# is the prefix and nothing else, so it cannot collide.
dev_sanitize_slug() { # name → a slug matching ^[a-z0-9-]+$
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed 's/[^a-z0-9-]/-/g; s/--*/-/g; s/^-//; s/-*$//'
}

# The bucket holding attachment and transcript bytes, one per stack. It was a
# single name for every stack, so two worktrees wrote each other's object bytes
# into one place. blobstore.New creates a missing bucket, so a new name needs no
# infra change.
#
# The slug alphabet is already S3-legal (see dev_sanitize_slug), so this is a
# prefix and nothing more — an injective mapping by construction rather than by
# a substitution that could fold two slugs together.
dev_bucket_for_slug() { # slug → an S3-legal bucket name
  local slug="$1"
  if [[ -z "$slug" ]]; then
    printf 'margince-dev'
    return 0
  fi
  printf 'margince-dev-%s' "$slug"
}

# The PRIMARY worktree keeps the shared stack — database `margince` on :8080,
# Redis db 0 — because `make migrate`, `make seed-dev` and `make verify-boot`
# target that database by name.
#
# A LINKED worktree gets its own everything, named after itself, with no flag to
# remember: an engineer runs several agent sessions, one worktree each, and none
# of them should have to know DEV_SLUG exists.
#
# The primary/linked test is the git directory, not the path: in a linked
# worktree --absolute-git-dir is <common>/worktrees/<name> and differs from
# --git-common-dir. The latter can print a relative path, so it is resolved
# before the comparison.
dev_derive_slug() { # → "" in the primary worktree, this worktree's name in a linked one
  local gitdir commondir top name digest
  gitdir=$(git rev-parse --absolute-git-dir) || return 1
  commondir=$(cd "$(git rev-parse --git-common-dir)" && pwd -P) || return 1
  [[ "$gitdir" == "$commondir" ]] && return 0

  top="$(git rev-parse --show-toplevel)"
  name="$(dev_sanitize_slug "$(basename "$top")")"

  # The digest is of the FULL PATH and is ALWAYS appended. Two things force that,
  # and the second is the one that matters:
  #
  #   - a basename does not identify a worktree. Two can both be called `feature`,
  #     and one slug between them is one database, one Redis logical database and
  #     one bucket shared by two stacks.
  #   - a slug must be STABLE for as long as a stack runs. An earlier version
  #     appended the digest only when a twin existed, so creating a second
  #     `feature` worktree renamed a RUNNING stack from `feature` to
  #     `feature-<digest>` — after which dev-stop resolved a slug with no record
  #     and could not stop it.
  #
  # So the name is noisier than it could be, and reliably its own.
  digest="$(printf '%s' "$top" | shasum -a 256 | cut -c1-8)"

  # Bounded so the names built from it fit their own limits: this slug becomes
  # `margince_dev_<slug>` (Postgres caps an identifier at 63 bytes) and
  # `margince-dev-<slug>` (S3 caps a bucket name at 63). 24 + 1 + 8 = 33 leaves
  # both prefixes room. A name that sanitises away to nothing — a directory of
  # pure punctuation — must not yield the empty answer that means PRIMARY, so it
  # gets the digest alone.
  if [[ -z "$name" ]]; then
    printf 'wt-%s' "$digest"
    return 0
  fi
  printf '%s-%s' "${name:0:24}" "$digest"
}

# dev_resolve_slug [supplied] — the slug this invocation runs under.
#
# An explicit value always wins and is VALIDATED rather than sanitised: silently
# rewriting what somebody typed would point them at a database they did not name.
# `_base` is refused because it is the primary stack's own directory.
dev_resolve_slug() { # [supplied] → slug on stdout
  local supplied="${1:-}"
  if [[ -z "$supplied" ]]; then
    dev_derive_slug
    return
  fi
  if ! [[ "$supplied" =~ ^[a-z0-9-]+$ ]]; then
    echo "FAIL: DEV_SLUG must match ^[a-z0-9-]+$ (got '$supplied'). Use a hyphen where you would write an underscore — the slug also names an S3 bucket, which admits no underscore." >&2
    return 1
  fi
  if [[ "$supplied" == "$DEV_BASE_SLUG_DIR" ]]; then
    echo "FAIL: DEV_SLUG=${DEV_BASE_SLUG_DIR} is reserved for the primary worktree's own stack — pick another name, or drop DEV_SLUG to use that stack." >&2
    return 1
  fi
  printf '%s' "$supplied"
}

# dev_state_dir SLUG — where this stack's log, pids and claim live.
dev_state_dir() { # slug
  printf '%s/%s' "$(dev_state_root)" "${1:-$DEV_BASE_SLUG_DIR}"
}

# dev_app_base_url — the URL this worktree's stack actually serves the app on.
#
# `make seed-dev` and `make verify-boot` both defaulted to :8080, which was the
# only answer while there was one stack. From a linked worktree it is the wrong
# one: `make dev` there serves a claimed port against `margince_dev_<slug>`, so
# seeding :8080 either seeds the PRIMARY worktree's stack — silently, with the
# records landing in a database nobody was looking at — or fails against nothing.
#
# Falls back to :8080 when this worktree has no recorded stack, which keeps the
# primary worktree and CI on the URL they already use.
dev_app_base_url() {
  local slug env_file FE_PORT
  slug="$(dev_resolve_slug "${DEV_SLUG:-}")" || return 1
  env_file="$(dev_state_dir "$slug")/env"
  if [[ -f "$env_file" ]]; then
    FE_PORT=''
    # shellcheck disable=SC1090
    . "$env_file"
    if [[ -n "$FE_PORT" ]]; then
      printf 'http://localhost:%s' "$FE_PORT"
      return 0
    fi
  fi
  # No recorded stack. For the PRIMARY worktree :8080 is still the answer — that
  # stack is the shared one and its port is fixed. For a LINKED worktree there is
  # no answer to fall back to: :8080 belongs to a different stack, and returning
  # it would send the API half of a seed there while dev_database_name sends the
  # SQL half to margince_dev_<slug>. That split is the defect this pair exists to
  # close, so refuse instead of half-answering.
  if [[ -z "$slug" ]]; then
    printf 'http://localhost:8080'
    return 0
  fi
  echo "FAIL: no dev stack recorded for this worktree (slug '${slug}') — start it with 'make dev' first, or set API_BASE explicitly. Falling back to :8080 would seed a different worktree's stack." >&2
  return 1
}

# dev_database_name — the Postgres database THIS worktree's stack runs against.
#
# The SQL half of `make seed-dev` (seed-dev.sql, seed-reset.sql) goes through
# psql with an explicit -d, and that was `margince` for everyone. Once `make dev`
# in a linked worktree started using margince_dev_<slug>, the two halves of one
# seed landed in two different databases: the API records in the worktree's, the
# fixture rows in the primary's. It surfaced as a NOT NULL violation on
# app_user.workspace_id — the fixture ran against a database whose demo workspace
# the API half had never created.
dev_database_name() {
  local slug
  slug="$(dev_resolve_slug "${DEV_SLUG:-}")" || return 1
  if [[ -z "$slug" ]]; then
    printf 'margince'
    return 0
  fi
  printf 'margince_dev_%s' "$slug"
}
