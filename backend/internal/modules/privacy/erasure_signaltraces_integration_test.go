// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What an erasure and a retention anonymize leave on signals and link
// suggestions: the dedupe key, the free-text reason and the audit images.
// Each is asked of both acts. They share one scrub. An arm that stopped
// calling it would fail nothing else.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var scrubCauses = []string{causeContactErasure, causeRetention}

const reasonQuotingTheSubject = "Hedda Subject wrote from hedda@signals.test"

type signalTraceFixture struct {
	tx                      pgx.Tx
	ctx                     context.Context
	subject, other          ids.ContactID
	subjectSignal, otherSig ids.UUID
	activity, otherActivity ids.UUID
}

func seedSignalTraces(t *testing.T) signalTraceFixture {
	t.Helper()
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	f := signalTraceFixture{
		tx: tx, subject: ids.New[ids.ContactKind](), other: ids.New[ids.ContactKind](),
		subjectSignal: ids.NewV7(), otherSig: ids.NewV7(), activity: ids.NewV7(), otherActivity: ids.NewV7(),
	}
	ws, user := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@signals.test")
	for _, c := range []ids.ContactID{f.subject, f.other} {
		mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Someone', 'manual', 'human:test')`, c)
	}
	for _, a := range []ids.UUID{f.activity, f.otherActivity} {
		mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
			VALUES ($1, 'email', now(), 'manual', 'user:'||$2::text)`, a, user)
	}
	seedSignal := func(id ids.UUID, about ids.ContactID) {
		mustExec(ctx, t, tx, `INSERT INTO signal (id, kind, source_channel, entity_type, entity_id, resolution_state,
				severity, summary, evidence, fingerprint, status, detected_at, source, captured_by)
			VALUES ($1::uuid, 'ext:erasetest:loss', 'derived', 'contact', $2, 'resolved', 'info', 'Quiet',
				'[{"snippet":"Hedda wrote she is leaving"}]'::jsonb, 'ext:erasetest:loss|hedda@signals.test|'||$1::uuid::text,
				'open', now(), 'erasetest', 'test')`, id, about)
	}
	seedSignal(f.subjectSignal, f.subject)
	seedSignal(f.otherSig, f.other)
	suggest := func(activity ids.UUID, target ids.ContactID) {
		mustExec(ctx, t, tx, `INSERT INTO activity_link_suggestion (activity_id, entity_type, entity_id, unit, reason, score, captured_by)
			VALUES ($1, 'contact', $2, 'erasetest', $3, 0.9, 'test')`, activity, target, reasonQuotingTheSubject)
	}
	suggest(f.otherActivity, f.subject)
	suggest(f.activity, f.other)
	f.ctx = principal.WithWorkspaceID(ctx, ws)
	f.ctx = principal.WithCorrelationID(f.ctx, ids.NewV7())
	f.ctx = principal.WithActor(f.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user})
	return f
}

func (f signalTraceFixture) text(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestScrubbingASubjectsSignalsReplacesTheDedupeKeyAndKeepsOthers(t *testing.T) {
	for _, cause := range scrubCauses {
		t.Run(cause, func(t *testing.T) {
			f := seedSignalTraces(t)
			if err := scrubSubjectSignals(f.ctx, f.tx, f.subject, "subject request", cause); err != nil {
				t.Fatal(err)
			}
			got := f.text(t, `SELECT fingerprint FROM signal WHERE id = $1`, f.subjectSignal)
			if strings.Contains(got, "hedda") || got != "erased:"+f.subjectSignal.String() {
				t.Errorf("the erased subject's signal keeps the fingerprint %q", got)
			}
			evidence := f.text(t, `SELECT summary || evidence::text FROM signal WHERE id = $1`, f.subjectSignal)
			if strings.Contains(evidence, "Hedda") {
				t.Errorf("the erased subject's signal still holds %q", evidence)
			}
			if kept := f.text(t, `SELECT fingerprint FROM signal WHERE id = $1`, f.otherSig); !strings.Contains(kept, "hedda@signals.test") {
				t.Errorf("a signal about another contact lost its fingerprint: %q", kept)
			}
		})
	}
}

func TestScrubbingASubjectReplacesTheReasonOfTheSuggestionsNamingThem(t *testing.T) {
	for _, cause := range scrubCauses {
		t.Run(cause, func(t *testing.T) {
			f := seedSignalTraces(t)
			if err := scrubSubjectSignals(f.ctx, f.tx, f.subject, "subject request", cause); err != nil {
				t.Fatal(err)
			}
			if got := f.text(t, `SELECT reason FROM activity_link_suggestion WHERE entity_id = $1`, f.subject); got != erasedText {
				t.Errorf("a suggestion naming the subject reads %q", got)
			}
			if got := f.text(t, `SELECT reason FROM activity_link_suggestion WHERE entity_id = $1`, f.other); got != reasonQuotingTheSubject {
				t.Errorf("a suggestion naming another contact was changed to %q", got)
			}
		})
	}
}

func TestErasingMessagesReplacesTheReasonOfTheSuggestionsOnThem(t *testing.T) {
	f := seedSignalTraces(t)
	if err := redactLinkSuggestionReasons(f.ctx, f.tx, nil, []ids.UUID{f.activity}); err != nil {
		t.Fatal(err)
	}
	if got := f.text(t, `SELECT reason FROM activity_link_suggestion WHERE activity_id = $1`, f.activity); got != erasedText {
		t.Errorf("a suggestion on an erased message reads %q", got)
	}
	if got := f.text(t, `SELECT reason FROM activity_link_suggestion WHERE activity_id = $1`, f.otherActivity); got != reasonQuotingTheSubject {
		t.Errorf("a suggestion on another message was changed to %q", got)
	}
}

func TestScrubbingASubjectWithholdsTheAuditImagesOfTheirCorrectedSignals(t *testing.T) {
	for _, cause := range scrubCauses {
		t.Run(cause, func(t *testing.T) {
			f := seedSignalTraces(t)
			if _, err := storekit.Audit(f.ctx, f.tx, "update", "signal", f.subjectSignal,
				map[string]any{"evidence": "Hedda wrote she is leaving"}, map[string]any{"evidence": "corrected"}); err != nil {
				t.Fatal(err)
			}
			served := func() string {
				return f.text(t, `SELECT count(*)::text FROM audit_log a WHERE a.entity_type = 'signal' AND a.entity_id = $1
					AND a.action = 'update' AND `+UnscrubbedImageSQL("a", "$2"), f.subjectSignal, ScrubVerbs())
			}
			if served() != "1" {
				t.Fatal("the correction's audit row is not served before the scrub, so the test proves nothing")
			}
			if err := scrubSubjectSignals(f.ctx, f.tx, f.subject, "subject request", cause); err != nil {
				t.Fatal(err)
			}
			if served() != "0" {
				t.Error("the old evidence in the correction's audit row is still served")
			}
		})
	}
}

func TestErasureLeavesTheLinkSecretsForThePassAfterTheCommit(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	ws, user, activity, delivery := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@signals.test")
	mustExec(ctx, t, tx, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
		VALUES ($1, 'email', now(), 'manual', 'user:'||$2::text)`, activity, user)
	mustExec(ctx, t, tx, `INSERT INTO comms_outbound (id, activity_id, provider, message_id, recipients, cc,
			subject, body, references_chain, status, sender_kind, template_key, template_version, payload_ref, payload_expires_at)
		VALUES ($1, $2, 'gmail', $3, jsonb_build_array('hedda@signals.test'), '[]'::jsonb, 'Confirm', 'link',
			'[]'::jsonb, 'pending', 'controller', 'confirm', 1, 'vault-ref-1', now() + interval '2 days')`,
		delivery, activity, delivery.String()+"@margince.test")

	if err := erasePayloads(ctx, tx, []ids.UUID{activity}); err != nil {
		t.Fatal(err)
	}
	var ref string
	var due bool
	if err := tx.QueryRow(ctx, `SELECT payload_ref, payload_expires_at <= now() FROM comms_outbound WHERE id = $1`, delivery).Scan(&ref, &due); err != nil {
		t.Fatal(err)
	}
	if ref != "vault-ref-1" || !due {
		t.Errorf("the delivery reads ref %q, due %v; want the reference kept and the payload due now", ref, due)
	}
}
