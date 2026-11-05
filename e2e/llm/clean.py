#!/usr/bin/env python3
"""Start a CLI candidate clean: none of the repository's rules, nor the operator's
settings, memory, plugins, skills or hooks.

The lane measures whether the tool copy is enough for a model that knows nothing
of Margince. A CLI started from the repository with the operator's setup has read
the rulebook and the operator's notes first, so its pass says nothing about the copy.

Run as a program, it is `claude` with the arguments given, started clean, and its
exit code. check.unclean reads the init line the CLI writes and stops a run that
loaded anything anyway.
"""

import contextlib
import filecmp
import os
import shutil
import subprocess
import sys
import tempfile

# No user, project or local settings: the operator's plugins, hooks and output
# style live there. Skills are off too; with --tools "" none could be called.
CLAUDE_FLAGS = ("--setting-sources", "", "--disable-slash-commands")
CLAUDE_ENV = {"CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1"}

# The one file codex needs from the operator's home: the login.
CODEX_AUTH = "auth.json"


@contextlib.contextmanager
def claude_launch():
    """(flags, environment, working directory) for a clean `claude` process."""
    with tempfile.TemporaryDirectory(prefix="e2e-llm-claude.") as empty:
        yield CLAUDE_FLAGS, {**os.environ, **CLAUDE_ENV}, empty


def run_claude(args):
    with claude_launch() as (flags, env, empty):
        return subprocess.run(["claude", *args, *flags], cwd=empty, env=env, check=False).returncode


def _private_copy(source, target):
    # Created 0600 before a byte is written: the login is a credential.
    created = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with open(source, "rb") as src, os.fdopen(created, "wb") as dst:
        os.fchmod(dst.fileno(), 0o600)
        shutil.copyfileobj(src, dst)


def _operator_codex_home():
    return os.environ.get("CODEX_HOME") or os.path.join(os.path.expanduser("~"), ".codex")


@contextlib.contextmanager
def codex_home():
    """A CODEX_HOME holding only a copy of the operator's login.

    The operator's home holds memories, plugins, skills and a global AGENTS.md,
    and --ignore-user-config skips only config.toml, so codex gets a home of its own.
    """
    login = os.path.join(_operator_codex_home(), CODEX_AUTH)
    with tempfile.TemporaryDirectory(prefix="e2e-llm-codex-home.") as home:
        copy = os.path.join(home, CODEX_AUTH)
        if os.path.isfile(login):
            _private_copy(login, copy)
        try:
            yield home
        finally:
            # A refresh rotates the token in the copy and voids the one it replaced,
            # so the refreshed login goes back or the operator is signed out.
            if os.path.isfile(login) and os.path.isfile(copy) and not filecmp.cmp(login, copy, shallow=False):
                staged = os.path.join(os.path.dirname(login), f".{CODEX_AUTH}.e2e-llm")
                _private_copy(copy, staged)
                os.replace(staged, login)


def codex_env(home):
    # codex also reads skills from ~/.agents/skills, so HOME is the clean home too.
    return {**os.environ, "CODEX_HOME": home, "HOME": home}


if __name__ == "__main__":
    sys.exit(run_claude(sys.argv[1:]))
