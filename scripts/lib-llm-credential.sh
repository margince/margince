#!/usr/bin/env bash
# lib-llm-credential.sh — which credential a lane that drives the claude CLI is
# about to spend. Source this; don't execute it.
#
# Two lanes drive the CLI and bill real tokens (scripts/e2e-llm.sh and
# scripts/e2e-llm-guards.sh), and this is the one answer both take: three
# variables can carry a credential, the CLI ranks them ANTHROPIC_AUTH_TOKEN (a
# gateway bearer) over ANTHROPIC_API_KEY (a Console key) over
# CLAUDE_CODE_OAUTH_TOKEN (a subscription token from `claude setup-token`), and
# it never says which one it passed over.
#
# A subscription token is the one credential with no second home: presented as
# either of the others it arrives without the OAuth beta header the API requires
# and is refused, and a refused credential reads downstream as a model that
# chose to call nothing. So a lane with two set measures an account nobody
# chose. Resolve it in the CLI's own order, and PRINT THE NAME — a transcript
# that ends in a 401 has to say which credential was on trial. What must not
# happen either way is a run that silently falls back to whatever the operator's
# own shell is logged into, because then the lane is measuring a different
# account's model.

# llm_credential prints the name of the variable the CLI will use, or explains
# on stderr that there is none and fails.
llm_credential() {
  if [[ -n "${ANTHROPIC_AUTH_TOKEN:-}" ]]; then
    echo ANTHROPIC_AUTH_TOKEN
  elif [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
    echo ANTHROPIC_API_KEY
  elif [[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]]; then
    echo CLAUDE_CODE_OAUTH_TOKEN
  else
    echo "no credential is set: CLAUDE_CODE_OAUTH_TOKEN (a subscription token from" >&2
    echo "\`claude setup-token\`), ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN" >&2
    return 1
  fi
}
