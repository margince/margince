// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// What a rep has decided about letting a KIND of proposal apply itself, and the
// track record the decision is weighed against.
//
// NOTHING HERE APPLIES ANYTHING. The mode is recorded and the counters are
// kept; no reader in this package skips asking because a row says 'auto'.
// Auto-apply needs a decider, and decidingactor.go refuses a system principal
// outright — approvals are decided by people. Until that has an answer, a
// stored 'auto' is a rep's stated preference rather than a promise the product
// can keep, and the surface says so by offering the choice without acting on it.
//
// A policy is always the ACTING USER'S OWN. There is no read or write here for
// somebody else's row: the authority to say "stop asking me about close dates"
// belongs to the person being asked, and a shape that let one colleague set
// another's would be a way to arrange for someone to stop seeing a question.
// That is why these take no user id — the principal is the subject.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AutonomyMode is the rung of the ladder a rep has chosen for one kind.
type AutonomyMode string

const (
	// AutonomyManual asks every time. The mode every kind starts in.
	AutonomyManual AutonomyMode = "manual"
	// AutonomyVeto applies after a stated delay unless the rep stops it.
	AutonomyVeto AutonomyMode = "veto"
	// AutonomyAuto applies on sight.
	AutonomyAuto AutonomyMode = "auto"
)

// AutonomyPolicy is one rep's standing answer for one kind, with the record it
// was earned on.
//
// The zero value is the honest default for a kind nobody has ever decided
// about: manual, no window, nothing counted. A caller therefore never has to
// distinguish "no row" from "a row saying ask me" — they mean the same thing,
// which is why ReadAutonomy returns this rather than an optional.
type AutonomyPolicy struct {
	Kind   string
	Mode   AutonomyMode
	Window time.Duration
	// ApprovedClean counts approvals that changed nothing. It is the one
	// counter that earns a promotion offer: a rep who rewrites the payload
	// every time has told the software the opposite of "stop asking me".
	ApprovedClean  int
	ApprovedEdited int
	Rejected       int
	PromotedAt     *time.Time
	DemotedAt      *time.Time
}

// autonomyDefault is the policy a kind carries before anyone has decided about
// it: manual, no window, nothing counted. The no-row read returns this rather
// than an empty struct, so a caller is never handed a policy whose mode is the
// empty string.
func autonomyDefault(kind string) AutonomyPolicy {
	return AutonomyPolicy{Kind: kind, Mode: AutonomyManual}
}

// ReadAutonomy returns the acting user's policy for one kind.
//
// An unknown kind is refused rather than answered with the default. The map is
// server vocabulary, and a caller asking about "clos_date_correction" has a bug
// this read can either surface now or hide behind a plausible-looking manual
// policy that no staging will ever match.
func (s *Service) ReadAutonomy(ctx context.Context, kind string) (AutonomyPolicy, error) {
	if err := actingForAHuman(ctx); err != nil {
		return AutonomyPolicy{}, err
	}
	if err := assertStageableKind(kind); err != nil {
		return AutonomyPolicy{}, err
	}
	p, _ := principal.Actor(ctx)
	policy := autonomyDefault(kind)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		policy, err = readAutonomyTx(ctx, tx, p.UserID, kind)
		return err
	})
	if err != nil {
		return AutonomyPolicy{}, err
	}
	return policy, nil
}

// ListAutonomy returns the acting user's policy for EVERY stageable kind.
//
// Every kind, not every stored row: the surface offering the ladder has to show
// the kinds a rep has never touched, and a list of only the rows that exist
// would render as "you have no choices to make" on a fresh account.
func (s *Service) ListAutonomy(ctx context.Context) ([]AutonomyPolicy, error) {
	if err := actingForAHuman(ctx); err != nil {
		return nil, err
	}
	p, _ := principal.Actor(ctx)
	kinds := StageableKinds()
	stored := make(map[string]AutonomyPolicy, len(kinds))
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stored, err = scanAutonomy(ctx, tx, p.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]AutonomyPolicy, 0, len(kinds))
	for _, kind := range kinds {
		if policy, ok := stored[kind]; ok {
			out = append(out, policy)
			continue
		}
		out = append(out, autonomyDefault(kind))
	}
	return out, nil
}

// SetAutonomyInput is a rep's choice for one kind.
type SetAutonomyInput struct {
	Kind   string
	Mode   AutonomyMode
	Window time.Duration
}

// SetAutonomy records the acting user's choice, leaving the counters alone.
//
// The counters are the track record and this is the decision made ON it, so a
// rep changing their mind never rewrites the history that justified the offer.
// Demotion is stamped whenever the mode moves DOWN the ladder, promotion
// whenever it moves up, so the surface can say when a rep last changed their
// standing without keeping a second log of it.
func (s *Service) SetAutonomy(ctx context.Context, in SetAutonomyInput) (AutonomyPolicy, error) {
	if err := actingForAHuman(ctx); err != nil {
		return AutonomyPolicy{}, err
	}
	if err := assertStageableKind(in.Kind); err != nil {
		return AutonomyPolicy{}, err
	}
	if err := assertAutonomyShape(in); err != nil {
		return AutonomyPolicy{}, err
	}
	p, _ := principal.Actor(ctx)
	var out AutonomyPolicy
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := readAutonomyTx(ctx, tx, p.UserID, in.Kind)
		if err != nil {
			return err
		}
		if err := writeAutonomyTx(ctx, tx, p.UserID, in, rungOf(in.Mode)-rungOf(before.Mode)); err != nil {
			return err
		}
		// A settings change is a domain write: it goes on the record with the
		// principal who made it, like every other mutation in this module.
		//
		// 'update' rather than a verb of its own. audit_log.action is a closed
		// CHECK set, and this is an ordinary settings update — what makes the
		// row legible is the evidence naming the kind and both modes, not a
		// twenty-ninth verb that every reader of the audit vocabulary would
		// then have to learn.
		evidence := map[string]any{
			approvalKeyKind: in.Kind,
			"setting":       "autonomy_policy",
			"mode_before":   string(before.Mode),
			"mode_after":    string(in.Mode),
		}
		if _, err := s.audit(ctx, tx, p, "update", p.UserID, evidence); err != nil {
			return err
		}
		out, err = readAutonomyTx(ctx, tx, p.UserID, in.Kind)
		return err
	})
	if err != nil {
		return AutonomyPolicy{}, err
	}
	return out, nil
}

// rungOf orders the ladder so a mode change can be told from a mode swap. It is
// not stored — only the direction of the move is, as promoted_at or demoted_at.
func rungOf(mode AutonomyMode) int {
	switch mode {
	case AutonomyVeto:
		return 1
	case AutonomyAuto:
		return 2
	case AutonomyManual:
		return 0
	}
	return 0
}

// InvalidAutonomyError maps to 422: a policy this installation cannot hold —
// an unknown kind, an unknown mode, or a window on a mode that does not wait.
//
// Its own type rather than a shared sentinel, matching InvalidEditError beside
// it: the handler decides the status code from the type, and a caller who typed
// a kind wrong should be told which field was wrong rather than handed the 403
// an authority failure would produce.
type InvalidAutonomyError struct{ Cause error }

func (e *InvalidAutonomyError) Error() string { return "autonomy: " + e.Cause.Error() }
func (e *InvalidAutonomyError) Unwrap() error { return e.Cause }

// assertStageableKind refuses a kind the server cannot stage.
func assertStageableKind(kind string) error {
	if !KindHasDecisionGrants(kind) {
		return &InvalidAutonomyError{
			Cause: fmt.Errorf("%q is not a kind this installation stages", kind),
		}
	}
	return nil
}

// assertAutonomyShape refuses the combinations the table's CHECK constraints
// would refuse, so a caller gets a 422 naming the field rather than a database
// error naming a constraint.
//
// The constraints stay regardless. This function guards the service call; the
// table guards whatever else ever writes the row, and a check that lives only
// in Go is one a future writer skips without noticing.
func assertAutonomyShape(in SetAutonomyInput) error {
	switch in.Mode {
	case AutonomyManual, AutonomyAuto:
		if in.Window != 0 {
			return &InvalidAutonomyError{
				Cause: fmt.Errorf("only a veto policy waits, so %q takes no window", in.Mode),
			}
		}
	case AutonomyVeto:
		if in.Window <= 0 {
			return &InvalidAutonomyError{
				Cause: errors.New("a veto policy is the chance to intervene, so it needs a window longer than zero"),
			}
		}
	default:
		return &InvalidAutonomyError{
			Cause: fmt.Errorf("%q is not an autonomy mode", in.Mode),
		}
	}
	return nil
}

const autonomyColumns = `kind, mode, veto_window, approved_clean, approved_edited,
	rejected, promoted_at, demoted_at`

// readAutonomyTx reads one row, or the default when the rep has never decided.
func readAutonomyTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, kind string) (AutonomyPolicy, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+autonomyColumns+` FROM approval_autonomy_policy WHERE user_id = $1 AND kind = $2`,
		userID, kind)
	if err != nil {
		return AutonomyPolicy{}, err
	}
	policies, err := collectAutonomy(rows)
	if err != nil {
		return AutonomyPolicy{}, err
	}
	if len(policies) == 0 {
		return autonomyDefault(kind), nil
	}
	return policies[0], nil
}

// scanAutonomy reads every row this rep has, keyed by kind.
func scanAutonomy(ctx context.Context, tx pgx.Tx, userID ids.UUID) (map[string]AutonomyPolicy, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+autonomyColumns+` FROM approval_autonomy_policy WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	policies, err := collectAutonomy(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]AutonomyPolicy, len(policies))
	for _, policy := range policies {
		out[policy.Kind] = policy
	}
	return out, nil
}

// collectAutonomy reads autonomyColumns into policies. Both reads above go
// through it, so a column added to that list is scanned the same way for the
// single-kind read and the list.
func collectAutonomy(rows pgx.Rows) ([]AutonomyPolicy, error) {
	defer rows.Close()
	var out []AutonomyPolicy
	for rows.Next() {
		var policy AutonomyPolicy
		var mode string
		var window *time.Duration
		if err := rows.Scan(&policy.Kind, &mode, &window, &policy.ApprovedClean,
			&policy.ApprovedEdited, &policy.Rejected, &policy.PromotedAt, &policy.DemotedAt); err != nil {
			return nil, err
		}
		policy.Mode = AutonomyMode(mode)
		if window != nil {
			policy.Window = *window
		}
		out = append(out, policy)
	}
	return out, rows.Err()
}

// writeAutonomyTx upserts the mode, leaving the counters untouched.
//
// The ladder stamp is written from `moved`, which the caller computed against
// the mode already stored: a move up stamps promoted_at, a move down stamps
// demoted_at, and choosing the mode already in force stamps neither, because
// re-confirming a setting is not a change of standing.
func writeAutonomyTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, in SetAutonomyInput, moved int) error {
	var window *time.Duration
	if in.Mode == AutonomyVeto {
		window = &in.Window
	}
	promoted, demoted := moved > 0, moved < 0
	_, err := tx.Exec(ctx,
		`INSERT INTO approval_autonomy_policy (user_id, kind, mode, veto_window, promoted_at, demoted_at)
		 VALUES ($1, $2, $3, $4,
		         CASE WHEN $5 THEN now() END,
		         CASE WHEN $6 THEN now() END)
		 ON CONFLICT (user_id, kind) DO UPDATE SET
		   mode = EXCLUDED.mode,
		   veto_window = EXCLUDED.veto_window,
		   promoted_at = CASE WHEN $5 THEN now() ELSE approval_autonomy_policy.promoted_at END,
		   demoted_at = CASE WHEN $6 THEN now() ELSE approval_autonomy_policy.demoted_at END`,
		userID, in.Kind, string(in.Mode), window, promoted, demoted)
	return err
}

// countDecisionTx records one decision against the deciding rep's track record
// for this kind.
//
// Called from inside the decision transaction, so a counted decision and the
// decision itself are one commit: a counter that could survive a rolled-back
// approval would offer autonomy on evidence that never happened.
//
// The row is created on first decision rather than when a rep is created. A
// policy row means "this rep has some history with this kind" — seeding one per
// rep per kind would fill the table with rows saying nothing, and make "never
// decided" unreadable.
func countDecisionTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, kind string, outcome decisionOutcome) error {
	column, err := outcome.column()
	if err != nil {
		return err
	}
	// The column is chosen from a closed set above, never taken from a caller's
	// string, so it is safe to format in — and it must be, because a counter
	// name is an identifier rather than a value.
	_, err = tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO approval_autonomy_policy (user_id, kind, %[1]s)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (user_id, kind) DO UPDATE
		   SET %[1]s = approval_autonomy_policy.%[1]s + 1`, column),
		userID, kind)
	return err
}

// decisionOutcome is what a rep did, in the three shapes the ladder counts.
type decisionOutcome int

const (
	// outcomeApprovedClean is an approval that changed nothing.
	outcomeApprovedClean decisionOutcome = iota
	// outcomeApprovedEdited is an approval whose payload the rep rewrote:
	// agreement with the intent, disagreement with the detail.
	outcomeApprovedEdited
	outcomeRejected
)

// column names the counter this outcome increments.
func (o decisionOutcome) column() (string, error) {
	switch o {
	case outcomeApprovedClean:
		return "approved_clean", nil
	case outcomeApprovedEdited:
		return "approved_edited", nil
	case outcomeRejected:
		return "rejected", nil
	}
	return "", errors.New("crmapprovals: no counter for this decision outcome")
}

// decisionOutcomeOf reads the two facts the decision already knows into the
// shape the ladder counts. Named apart from bundle.go's outcomeOf, which
// answers a different question — what a bulk call did to a row it could not
// decide — and would otherwise read as the same one.
func decisionOutcomeOf(approve bool, edited bool) decisionOutcome {
	if !approve {
		return outcomeRejected
	}
	if edited {
		return outcomeApprovedEdited
	}
	return outcomeApprovedClean
}
