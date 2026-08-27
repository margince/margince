// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadEnvFile reads margince.env into a KEY=VALUE slice for the child
// processes.
//
// The server roles take their secrets from the environment (12-factor), which
// on a server is set by the deployment. A desktop installation has no
// deployment, so this file is that layer: it is the ONE place a user turns
// features on, and it is why "full features" does not require a terminal
// incantation.
//
// Values here are secrets. The file is created 0600 and never logged.
// The returns are NAMED so the deferred close can reach them: with unnamed
// results the assignment below compiles and does nothing, which is a swallowed
// error wearing the clothes of a handled one.
func loadEnvFile(path string) (env []string, err error) {
	// #nosec G304 -- path is layout.envPath(), derived from the installation directory; no request or user input reaches it
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		// A failure to close a read-only file cannot corrupt anything, but it
		// must not be silently discarded either.
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := scanner.Text()
		// A byte-order mark is a property of the FILE, so line 1 is the only
		// place it means anything and the only place it is forgiven. On any
		// other line an invisible character is a mistake, and parseEnvLine
		// says so rather than carrying it into a variable name.
		if line == 1 {
			raw = strings.TrimPrefix(raw, byteOrderMark)
		}
		entry, parseErr := parseEnvLine(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, parseErr)
		}
		if entry != "" {
			env = append(env, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return env, nil
}

// byteOrderMark opens a UTF-8 file written by most Windows tooling. Notepad can
// put it here, and so can a PowerShell script: on Windows PowerShell 5.1 —
// the engine every Windows box has — `Set-Content -Encoding UTF8` adds one
// whether or not it was asked to.
//
// Tolerated rather than refused, because it means nothing here and a user cannot
// see it. Refusing it fails the whole start over a line the user can plainly
// read as a comment, which is a diagnosis nobody can act on.
const byteOrderMark = "\ufeff"

// parseEnvLine returns a KEY=VALUE entry, or "" for a blank or comment line.
//
// A malformed line is an error rather than a skip: silently ignoring a
// mistyped setting is how a user ends up convinced a feature is broken when
// the real answer is a missing "=".
func parseEnvLine(raw string) (string, error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	// `export KEY=value` is what a user copying from shell instructions will
	// paste, so accept it rather than rejecting a reasonable habit.
	line = strings.TrimPrefix(line, "export ")

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", fmt.Errorf("expected KEY=value, got %q", raw)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("missing a name before '=' in %q", raw)
	}
	// A name holding a character nobody can see is the silent failure above
	// wearing a disguise, so it is refused for the same reason. Cut splits
	// "\ufeffMARGINCE_PORT=8800" on its "=" quite happily, and the child process
	// then holds a variable whose name opens with a mark: the setting does
	// nothing, and no line of output says why. An environment variable name is
	// plain ASCII by convention, and a character outside it is a paste artifact
	// far more often than an intention.
	for _, r := range key {
		if r < ' ' || r > '~' {
			return "", fmt.Errorf(
				"the name in %q holds %q, which is not a plain-ASCII character — "+
					"a byte-order mark or a pasted space, most likely; retype the line", raw, r)
		}
	}

	value = strings.TrimSpace(value)
	// Quotes are how a value with spaces is written; strip one matched pair.
	if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' ||
		value[0] == '\'' && value[len(value)-1] == '\'') {
		value = value[1 : len(value)-1]
	}
	return key + "=" + value, nil
}

// ensureEnvFile writes the annotated template on first run and leaves an
// existing file alone, so a user's settings are never overwritten.
func ensureEnvFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(envTemplate), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// envTemplate documents every setting a desktop installation can turn on.
//
// Everything is commented out, so a fresh install runs the safe defaults and
// the file doubles as the reference for what is even possible — a user should
// not have to read Go source to discover that Gmail capture exists.
const envTemplate = `# Margince settings.
#
# Uncomment a line to turn a feature on, then restart Margince.
# This file holds secrets: it is created readable only by you (chmod 600).
# It is never overwritten by an update.

# ---------------------------------------------------------------------------
# Licence and posture
#
# This installation runs in the "dev" posture, which is what lets it start
# without a licence token. That is the honest description of one person running
# their own copy, and it does NOT arm the destructive admin endpoints — those
# need allow_data_reset in margince.yaml, which is absent.
#
# If you have been issued a licence for this installation, name it here and
# switch the posture to production; a serving role then holds itself to it.
# ---------------------------------------------------------------------------
# MARGINCE_LICENSE=
# MARGINCE_ENV=production

# ---------------------------------------------------------------------------
# Where Margince listens. Keep this stable so your browser bookmark keeps
# working. Margince refuses to start if the port is already in use rather
# than silently moving.
# ---------------------------------------------------------------------------
# MARGINCE_PORT=8800

# ---------------------------------------------------------------------------
# AI (bring your own key)
#
# Until models are bound the AI surfaces run on the offline fake: they respond,
# but the answers are canned. Binding them is done IN THE APP, under
# Settings -> AI, and it takes TWO things: pick a provider and model for each
# tier, and put that provider's key in under Model provider keys. Neither is
# enough on its own — a key with no tier bound to its vendor changes nothing.
#
# Both take effect without a restart, within the routing refresh interval rather
# than the instant you save, and neither needs a file in this folder.
#
# A key set below only does the SECOND half: the next start seals it into the
# app's key vault and records where, after which the line here can go. It saves
# pasting a long key into a browser and nothing else — the tiers still have to be
# bound in the app, and Settings -> AI is the only path that can rotate a key
# later.
# ---------------------------------------------------------------------------
# ANTHROPIC_API_KEY=
# OPENAI_API_KEY=
# GEMINI_API_KEY=
# OPENAI_COMPATIBLE_API_KEY=

# ---------------------------------------------------------------------------
# Attachments and logos (object storage)
#
# Already working: attachments and company logos are kept in data/blobs inside
# this folder, so nothing here needs setting. Point MARGINCE_BLOBSTORE_PATH
# somewhere else to move them (an external disk, say), or set the endpoint
# below to keep them in an S3-compatible service instead — the endpoint wins
# over the path when both are set. A local store holds bytes for THIS machine
# only: no replication, no signed URLs, and a second api replica cannot read a
# directory it does not mount.
# ---------------------------------------------------------------------------
# MARGINCE_BLOBSTORE_PATH=
# MARGINCE_BLOBSTORE_ENDPOINT=
# MARGINCE_BLOBSTORE_ACCESS_KEY=
# MARGINCE_BLOBSTORE_SECRET_KEY=
# MARGINCE_BLOBSTORE_BUCKET=margince
# MARGINCE_BLOBSTORE_REGION=
# MARGINCE_BLOBSTORE_USE_SSL=true

# ---------------------------------------------------------------------------
# Public address
#
# Required to send marketing email (the unsubscribe link must be reachable)
# and for any OAuth callback. A machine only you can reach cannot serve
# either, so this needs a real externally-reachable URL.
# ---------------------------------------------------------------------------
# MARGINCE_PUBLIC_BASE_URL=

# ---------------------------------------------------------------------------
# Email + calendar capture (Gmail / Outlook)
#
# Needs OAuth credentials from Google or Microsoft, the signing key below,
# and MARGINCE_PUBLIC_BASE_URL above.
# MARGINCE_CONNECTOR_STATE_KEY must be at least 32 characters; generate one
# with:  openssl rand -hex 32
# ---------------------------------------------------------------------------
# MARGINCE_CONNECTOR_STATE_KEY=
# MARGINCE_GMAIL_CLIENT_ID=
# MARGINCE_GMAIL_CLIENT_SECRET=
# MARGINCE_GMAIL_PUBSUB_TOPIC=
# MARGINCE_GMAIL_PUSH_TOKEN=
# MARGINCE_GMAIL_PUSH_AUDIENCE=
# MARGINCE_GMAIL_PUSH_SERVICE_ACCOUNT=
# MARGINCE_GRAPH_CLIENT_ID=
# MARGINCE_GRAPH_CLIENT_SECRET=
# MARGINCE_GRAPH_TENANT=

# ---------------------------------------------------------------------------
# Stored connector credentials
#
# Encrypts the OAuth tokens Margince holds for the connectors above. Set this
# BEFORE connecting an account: changing it later makes existing stored
# credentials unreadable. Generate with:  openssl rand -base64 32
# ---------------------------------------------------------------------------
# MARGINCE_KEYVAULT_ROOT_KEY=

# ---------------------------------------------------------------------------
# Outbound webhooks
#
# Unset, the webhook subscription endpoints answer 503.
# Generate with:  openssl rand -base64 32
# ---------------------------------------------------------------------------
# MARGINCE_WEBHOOK_KEY=

# ---------------------------------------------------------------------------
# Incumbent CRM mirror (HubSpot overlay)
# ---------------------------------------------------------------------------
# MARGINCE_HUBSPOT_APP_SECRET=

# ---------------------------------------------------------------------------
# Diagnostics
#
# Logs are written to data/logs/ regardless; this controls how much detail.
# ---------------------------------------------------------------------------
# MARGINCE_LOG_LEVEL=info
# MARGINCE_LOG_FORMAT=text

# ---------------------------------------------------------------------------
# Agent access tokens (MCP). 0 = the 30-day default, maximum 2160h (90 days).
# ---------------------------------------------------------------------------
# MARGINCE_OAUTH_ACCESS_TOKEN_TTL=720h
`
