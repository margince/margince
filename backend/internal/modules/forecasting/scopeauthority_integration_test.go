// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package forecasting

// The scope gate on the WRITE, through the real store against a real database —
// the half a unit test over requireScopeAuthority cannot reach, which is that
// the entry points actually run it before touching the table.
//
// A call is an assertion about a scope, made by whoever is accountable for it,
// and the object grant was the whole gate: a manager holding forecast.create
// recorded a commitment against a rival team's scope — stored with that team's
// scope_id, superseding their standing call, and unremovable, since no seat
// holds forecast.update or forecast.delete.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// asBounded is the seat the tampering was done from: forecast.create and
// forecast.read outright, and row scope BELOW all — which is what `manager`
// actually holds. e.as() carries RowScopeAll, and an unbounded seat answers for
// the installation by definition, so it could not show this rule working.
func (e *snapshotEnv) asBounded(teams ...ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		TeamIDs: teams,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects:  map[string]principal.ObjectGrant{"forecast": {Read: true, Create: true}},
			RowScope: principal.RowScopeTeam,
		},
	})
}

func TestAForecastCallIsRefusedAgainstAScopeTheCallerDoesNotAnswerFor(t *testing.T) {
	e := setupSnapshot(t)
	zone := time.UTC
	period, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.May, 14, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatal(err)
	}
	call := func(scope Scope) NewCall {
		return NewCall{Period: period, Scope: scope, AmountMinor: 100000, Currency: "EUR", Note: "revised"}
	}
	mine, theirs := ids.NewV7(), ids.NewV7()
	someoneElse := ids.NewV7()
	ctx := e.asBounded(mine)

	// THE TAMPER. Both halves of it: a rival team's scope and another
	// individual's. The exact sentinel, because ErrNotFound is the refusal this
	// gate owes — whether a rival team has called this period is itself the
	// disclosure, so a permission denial would leak the existence it withholds.
	for _, scope := range []Scope{{Kind: ScopeTeam, ID: &theirs}, {Kind: ScopeOwner, ID: &someoneElse}} {
		if _, err := e.store.RecordCall(ctx, call(scope)); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("recording a call against %s scope %v → %v, want ErrNotFound — the row would be "+
				"stored under that scope and supersede its standing call, with no seat holding "+
				"forecast.update or forecast.delete to remove it", scope.Kind, *scope.ID, err)
		}
	}
	// A scope_kind nobody recognises is a malformed request for EVERY caller.
	// Checked after the authority instead of before, this seat was told the
	// thing it named does not exist while an all-scope seat was told its field
	// was wrong — one defect reported two ways, decided by the reader.
	var parse *values.ParseError
	_, err = e.store.RecordCall(ctx, call(Scope{Kind: "managed_teams", ID: &mine}))
	if !errors.As(err, &parse) || parse.Field != "scope_kind" {
		t.Errorf("recording against a resolved-only kind → %v, want a scope_kind ParseError", err)
	}

	// THE POSITIVE CONTROLS, and they are what make the refusals above mean
	// anything: without them this test also passes when the store refuses every
	// scope and nobody can call a forecast at all.
	own := Scope{Kind: ScopeOwner, ID: &e.rep}
	if _, err := e.store.RecordCall(ctx, call(own)); err != nil {
		t.Fatalf("the caller was refused their OWN owner scope: %v", err)
	}
	if _, err := e.store.RecordCall(ctx, call(Scope{Kind: ScopeTeam, ID: &mine})); err != nil {
		t.Fatalf("the caller was refused a team they are a member of: %v", err)
	}
	// The workspace scope names no subject, so the object grant is what bounds
	// it — unchanged by this gate, and asserted so a later narrowing is a
	// deliberate edit rather than a surprise.
	if _, err := e.store.RecordCall(ctx, call(Scope{Kind: ScopeWorkspace})); err != nil {
		t.Fatalf("the caller was refused the workspace scope: %v", err)
	}
	// And the write it was allowed reads back, so the refusals above are this
	// gate working rather than the store refusing everything.
	back, err := e.store.CurrentCall(ctx, period, own)
	if err != nil {
		t.Fatalf("reading back the caller's own standing call: %v", err)
	}
	if back.AmountMinor != 100000 || back.Note != "revised" {
		t.Errorf("read back amount %d note %q, want 100000 and \"revised\"", back.AmountMinor, back.Note)
	}

	// THE READ IS DELIBERATELY NOT NARROWED HERE, and this asserts that on
	// purpose rather than leaving it to be discovered. Which population a caller
	// may MEASURE is settled once in the composition layer, against the caller's
	// lens and a live-membership read this module cannot make, and it is WIDER
	// than the recording rule: a team manager may read a teammate's figures.
	// Re-asking the recording question here would refuse the very manager that
	// resolver had just admitted, so a future narrowing of this call has to fail
	// this line and be argued for.
	if _, err := e.store.CurrentCall(ctx, period, Scope{Kind: ScopeOwner, ID: &someoneElse}); err != nil &&
		!errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading another owner's standing call → %v, want either the row or "+
			"no-standing-call; the population question belongs to the composition layer", err)
	}
}
