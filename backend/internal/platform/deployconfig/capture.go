// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

// The `capture:` block of margince.yaml: the suppression lists the mail
// pipeline's gates read.

// Capture is the deployment's mail-capture pipeline tuning (ADR-0063).
type Capture struct {
	// FreemailExtra and FreemailNever MOVED to the workspace's own
	// consumer-mail list (CAP-PARAM-5), editable in Settings and read per
	// transaction so a correction takes effect on the next message.
	//
	// They are still decoded, and deliberately: the file is parsed with
	// KnownFields(true), so deleting them outright would turn an upgrade into a
	// refusal to boot, reported by the yaml library in a message that names no
	// remedy and for a list the operator can no longer reach because the
	// process will not start. Values here are IGNORED; Warnings says so.
	FreemailExtra []string `yaml:"freemail_extra"`
	FreemailNever []string `yaml:"freemail_never"`

	// TransactionalExtra appends deployment-specific mail-infrastructure
	// eSLDs to the pinned baseline (CAP-PARAM-6, ADR-0072): mail from these
	// senders keeps the activity but derives no counterparty at all.
	TransactionalExtra []string `yaml:"transactional_extra"`
	// TransactionalNever is the operator allowlist of registrable domains
	// that must never be suppressed as transactional (CAP-PARAM-6) — it wins
	// over every baseline/prefix rule.
	TransactionalNever []string `yaml:"transactional_never"`

	// TracePayloads keeps each traced message's sender and a bounded subject in
	// the 24-hour capture trace, INCLUDING messages dropped because every party
	// was internal.
	//
	// ON unless the file says otherwise, which is why this is a pointer: for a
	// plain bool an absent key and an explicit `false` are the same zero value,
	// so a default of on could not be turned off. nil means the operator said
	// nothing and gets the default.
	//
	// It ships on because the trace exists to answer "why did this message not
	// arrive", and without the sender it cannot: a page of decisions naming
	// nobody tells a member their mail is a black box rather than telling them
	// what the pipeline threw away. The cost is one address and one clamped
	// subject (320 and 300 characters, trace.go), never a body, deleted with
	// the row by the hourly sweep and reachable by an erasure inside the
	// window. A member reads only their OWN rows — no grant widens them — so
	// this is somebody's own mail shown back to them.
	//
	// Settable ONLY here, and only to turn it OFF. There is no API and no
	// Settings control: a member must not be able to switch on retention of
	// their colleagues' subjects, and an operator turning it off is a decision
	// with a name attached.
	//
	// It does NOT touch the system_log breadcrumb, which keeps carrying the
	// connector, the source system and the external id and nothing else
	// (ADR-0082 §1). Nor does it reach ai_call_payload, which this pipeline's
	// verdict task is pinned out of (`no_payload: true`, ADR-0074).
	TracePayloads *bool `yaml:"trace_payloads"`
}

// TracesPayloads answers the question the field spells as three states: an
// operator who said nothing gets the default, which is on.
//
// Every reader goes through this rather than dereferencing the pointer, so
// there is one place the default lives and a nil can never panic at a call
// site that forgot it.
//
// Held by: TestOnlyTheResolverDereferencesTracePayloads (capture_test.go)
func (c Capture) TracesPayloads() bool {
	return c.TracePayloads == nil || *c.TracePayloads
}

// Warnings names the settings this block still accepts but no longer acts on,
// one sentence each, for a role to log at boot. Empty when the file says
// nothing stale — an operator who never set these hears nothing.
func (c Capture) Warnings() []string {
	var out []string
	if len(c.FreemailExtra) > 0 || len(c.FreemailNever) > 0 {
		out = append(out,
			"capture.freemail_extra / capture.freemail_never are ignored: the consumer-mail list moved to the workspace, editable under Settings or at POST /v1/capture/consumer-mail-domains. Remove them from margince.yaml.")
	}
	return out
}
