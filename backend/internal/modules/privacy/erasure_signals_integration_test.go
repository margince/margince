// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// A corrected signal keeps its old evidence in an audit row. An erasure of the
// contact it is about must put that row behind a scrub. The boundary is asked
// with the predicate the history reads use.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestErasingAContactWithholdsTheKeptEvidenceOfTheirSignals(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)

	ws, user := ids.NewV7(), ids.NewV7()
	subject, signal := ids.New[ids.ContactKind](), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@signals.test")
	mustExec(ctx, t, tx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, subject, user)
	mustExec(ctx, t, tx,
		`INSERT INTO signal (id, kind, source_channel, entity_type, entity_id, resolution_state, severity,
		                     summary, evidence, status, detected_at, source, captured_by)
		 VALUES ($1, 'risk', 'web', 'contact', $2, 'resolved', 'info', 'Quiet',
		         '[{"snippet":"Hedda wrote she is leaving"}]'::jsonb, 'open', now(), 'manual', 'test')`, signal, subject)

	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
	})
	if _, err := storekit.Audit(ctx, tx, "update", "signal", signal,
		map[string]any{"evidence": "Hedda wrote she is leaving"}, map[string]any{"evidence": "corrected"}); err != nil {
		t.Fatal(err)
	}

	served := func() int {
		var n int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_log a WHERE a.entity_type = 'signal' AND a.entity_id = $1 AND a.action = 'update'
			   AND `+UnscrubbedImageSQL("a", "$2"), signal, ScrubVerbs()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if served() != 1 {
		t.Fatal("the correction's audit row is not served before any erasure, so the test proves nothing")
	}
	if err := tombstoneSubjectSignals(ctx, tx, subject, "subject request"); err != nil {
		t.Fatalf("tombstoning the subject's signals: %v", err)
	}
	if served() != 0 {
		t.Error("the old evidence in the correction's audit row is still served after the contact was erased")
	}
}
