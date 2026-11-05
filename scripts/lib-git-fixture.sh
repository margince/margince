# lib-git-fixture.sh — sourced by every script test that builds a throwaway git
# repository, so nothing from the caller's git setup reaches the fixture.
# backend/gates/gitfixtureisolation_test.go holds that every such test sources it.

# git's environment. `git -C "$dir"` changes the WORKING DIRECTORY only — it does
# not override GIT_DIR. With GIT_DIR exported, `git init` never creates $dir/.git
# and every command lands in the surrounding repository instead: the identity
# writes overwrite its local user.name/user.email, and `commit` puts a real
# commit on its current branch. `rm -rf "$dir"` then takes away the temp dir and
# leaves that behind, with nothing reporting it.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_CEILING_DIRECTORIES

# The caller's git CONFIG. A global commit.gpgsign makes every fixture commit ask
# gpg to sign as the fixture's made-up identity, which fails, so the test fails
# on a developer's machine and passes in CI. A global init.templateDir copies
# hooks into the fixture that `git commit` then EXECUTES, and a global absolute
# core.hooksPath runs the developer's real hooks against a temp directory.
#
# /dev/null for both scopes is NOT sufficient on its own: GIT_CONFIG_COUNT with
# GIT_CONFIG_KEY_n/VALUE_n injects settings that survive both scopes being empty.
# Measured — with COUNT=1 and KEY_0=core.hooksPath, `git config core.hooksPath`
# inside a fresh fixture returns the caller's path and the injected pre-commit
# hook RUNS. Unsetting COUNT disarms the indexed pairs; GIT_CONFIG_PARAMETERS is
# the internal spelling of the same thing.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_TERMINAL_PROMPT=0
unset GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS
