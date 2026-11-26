# lib-git-fixture.sh — sourced by every script test that builds a throwaway git
# repository, before its first git call, so nothing from the caller's git setup
# reaches the fixture. backend/gates/gitfixtureisolation_test.go holds that.

# `git -C "$dir"` does not override an exported GIT_DIR, so without this every
# fixture command lands in the surrounding repository.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_CEILING_DIRECTORIES

# A global commit.gpgsign fails the fixture's commits, and a template dir or
# hooks path runs the caller's hooks in it. GIT_CONFIG_COUNT and
# GIT_CONFIG_PARAMETERS inject settings past both empty scopes.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_TERMINAL_PROMPT=0
unset GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_TEMPLATE_DIR

# The emptied system scope also drops the CI image's trust in a checkout that
# root does not own. Only this environment entry stands in for it.
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
