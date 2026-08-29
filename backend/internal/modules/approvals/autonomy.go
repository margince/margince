// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The track record a rep builds by deciding, one kind of proposal at a time.
//
// Trust is not a property of the software. It is one person's experience of one
// kind of proposal: a rep who has approved fourteen close-date confirmations
// unchanged has evidence about close dates and none about outbound mail. So the
// grain is (rep, kind), and approval_autonomy_policy holds one row per pair.
//
// WHAT IS HERE. Decisions are counted as they are made, so the record exists by
// the time there is something to weigh it for. Beside the counters sit the mode
// a rep has chosen per kind, read by the auto-applier and written by the rep
// themselves: an agent principal carrying the owner's authority is the decider
// approvals were missing, so a mode column that once described a promise the
// product could not keep now describes one it keeps.
//
// The counters are stored rather than counted from the approval table, which
// was the first design. Approvals expire and are swept, and a retention policy
// will eventually delete decided rows — so a record derived from a table that
// forgets would reset a rep's earned standing whenever housekeeping ran.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countDecisionTx records one decision against the deciding rep's track record
// for this kind.
//
// Called from inside the decision transaction, so a counted decision and the
// decision itself are one commit: a counter that could survive a rolled-back
// approval would offer autonomy on evidence of a decision nobody made.
//
// The row is created on first decision rather than when a rep is created. A
// policy row means "this rep has some history with this kind", so seeding one
// per rep per kind would fill the table with rows saying nothing and make
// "never decided" unreadable.
func countDecisionTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, kind string, outcome decisionOutcome) error {
	column, err := outcome.column()
	if err != nil {
		return err
	}
	// The column is chosen from the closed set below, never taken from a
	// caller's string, so it is safe to format in — and it must be, because a
	// counter name is an identifier rather than a value.
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
	// outcomeApprovedClean is an approval that changed nothing. It is the one
	// that will earn a promotion offer: a rep who rewrites the payload every
	// time has told the software the opposite of "stop asking me".
	outcomeApprovedClean decisionOutcome = iota
	// outcomeApprovedEdited is an approval whose payload the rep rewrote:
	// agreement with the intent, disagreement with the detail. Counted apart
	// from a clean approval because folding the two together would promote
	// fastest exactly the kind that should keep asking.
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

// decisionOutcomeOf reads what the decision already knows into the shape the
// ladder counts: the verdict, and the edited payload the caller either has or
// does not. It takes the payload rather than a second flag so the two cannot be
// swapped at a call site where both spellings would compile.
//
// Named apart from bundle.go's outcomeOf, which answers a different question —
// what a bulk call did to a row it could not decide.
func decisionOutcomeOf(approve bool, edited json.RawMessage) decisionOutcome {
	if !approve {
		return outcomeRejected
	}
	if len(edited) > 0 {
		return outcomeApprovedEdited
	}
	return outcomeApprovedClean
}

// AutoApplyKinds are the kinds a rep may put on 'auto'.
//
// A closed set, and deliberately not every kind. What may apply without asking
// is what the product can put back: each of these writes fields on ONE record
// of a type the restore path serves, so an apply the rep disagrees with is one
// Undo away. Outbound sends and merges are absent for the opposite reason —
// nothing reverses a message a customer has read, or a merge that has already
// discarded the loser's link rows.
//
// It is a set here rather than a column on the policy row because it is a fact
// about the KIND, not about one rep's history with it. A row naming a kind
// outside this set is inert: SetAutoApply refuses to write it, so no reader has
// to defend against one.
var AutoApplyKinds = map[string]bool{
	"close_date_correction": true,
	"org_name_promotion":    true,
	"lifecycle_change":      true,
}

// SortedAutoApplyKinds is AutoApplyKinds in a stable order.
//
// Exported because three callers need the same order for the same reason and a
// map gives none: the settings screen, whose rows must not move between visits;
// the sweep, which walks the kinds it may apply; and the gate that holds the
// frontend's copy of this set against it. Ranging over the map at each of them
// is three orders, and the two that face a reader would both be wrong sometimes.
func SortedAutoApplyKinds() []string {
	kinds := make([]string, 0, len(AutoApplyKinds))
	for kind := range AutoApplyKinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// AutonomyMode is one rung of the ladder.
type AutonomyMode string

const (
	// ModeManual asks every time. What every rep has until they choose.
	ModeManual AutonomyMode = "manual"
	// ModeAuto applies on sight, undoably.
	ModeAuto AutonomyMode = "auto"
)

// AutoApplyMode reports whether the rep this call acts for has put this kind on
// automatic.
//
// NO OBJECT GATE, and it needs none: a policy row is one person's answer about
// their own queue, and this reads the row of the principal on the context. It
// takes no user id, so there is no row a caller could ask for but not be —
// which is a stronger bound than a grant, because a grant could be held over
// somebody else. auth.Require has no object to check here in any case;
// `approval` is not in the closed core set.
//
// The auto-applier calls it having already bound the OWNER as the acting
// principal, so the policy it reads is the owner's own — the same row that rep
// would see in their settings.
//
// Absence is 'manual', which is why a missing row is not an error: a rep who
// has never decided this kind has no policy row, and "never chose" and "chose
// to be asked" are the same answer. Reading them differently would make the
// first decision of a kind behave unlike every one after it.
//
// The kind is checked against AutoApplyKinds here as well as on the write. A
// set that shrinks — a kind that stops being reversible — must stop applying
// for the reps who already said yes, and a reader trusting an old row would go
// on applying it.
func (s *Service) AutoApplyMode(ctx context.Context, kind string) (AutonomyMode, error) {
	if !AutoApplyKinds[kind] {
		return ModeManual, nil
	}
	rep, ok := principal.Actor(ctx)
	if !ok || rep.UserID.IsZero() {
		return ModeManual, fmt.Errorf("a policy belongs to a person, and this call names none: %w",
			apperrors.ErrPermissionDenied)
	}
	var mode string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT mode FROM approval_autonomy_policy WHERE user_id = $1 AND kind = $2`,
			rep.UserID, kind).Scan(&mode)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ModeManual, nil
	}
	if err != nil {
		return ModeManual, fmt.Errorf("crmapprovals: reading the autonomy mode: %w", err)
	}
	return AutonomyMode(mode), nil
}

// KindAutonomy is one kind's standing with one rep: whether it applies without
// asking, and the record that says whether it should.
type KindAutonomy struct {
	Kind string
	Mode AutonomyMode
	// ApprovedClean, ApprovedEdited and Rejected are what this rep has done with
	// this kind so far. They travel with the mode because the choice is only
	// meaningful beside them: "apply these without asking" is a different
	// question after fourteen clean approvals than after none.
	ApprovedClean  int
	ApprovedEdited int
	Rejected       int
}

// AutoApplySettings reports every kind a rep may put on automatic, each with the
// mode it currently stands at and the track record behind it.
//
// It returns the whole of AutoApplyKinds rather than the rows the table holds. A
// rep who has never met a kind has no row, and a settings screen that listed
// only rows would hide exactly the choices nobody has made yet — the ones a rep
// opens the screen to make. Absence is 'manual', the same reading AutoApplyMode
// takes.
//
// NO OBJECT GATE, for the reason the single-kind read gives: this reads the rows
// of the principal on the context and takes no user id, so there is no row a
// caller could ask for but not be.
func (s *Service) AutoApplySettings(ctx context.Context) ([]KindAutonomy, error) {
	rep, ok := principal.Actor(ctx)
	if !ok || rep.UserID.IsZero() {
		return nil, fmt.Errorf("a policy belongs to a person, and this call names none: %w",
			apperrors.ErrPermissionDenied)
	}
	var settings []KindAutonomy
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		settings, err = autonomySettingsInTx(ctx, tx, rep.UserID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("crmapprovals: reading the autonomy settings: %w", err)
	}
	return settings, nil
}

// autonomySettingsInTx is the read both entry points share.
//
// It takes the caller's transaction so a write and the answer it returns are one
// commit: a rep told the change failed while it had in fact landed would leave
// automatic application on with the screen saying off, and that is the one
// direction this setting must not be wrong in.
func autonomySettingsInTx(ctx context.Context, tx pgx.Tx, repID ids.UUID) ([]KindAutonomy, error) {
	stored := make(map[string]KindAutonomy, len(AutoApplyKinds))
	rows, err := tx.Query(ctx,
		`SELECT kind, mode, approved_clean, approved_edited, rejected
		   FROM approval_autonomy_policy WHERE user_id = $1`, repID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k KindAutonomy
		var mode string
		if err := rows.Scan(&k.Kind, &mode, &k.ApprovedClean, &k.ApprovedEdited,
			&k.Rejected); err != nil {
			return nil, err
		}
		k.Mode = AutonomyMode(mode)
		stored[k.Kind] = k
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	kinds := SortedAutoApplyKinds()
	settings := make([]KindAutonomy, 0, len(kinds))
	for _, kind := range kinds {
		row, held := stored[kind]
		if !held {
			row = KindAutonomy{Kind: kind, Mode: ModeManual}
		}
		// The stored mode is reported as it stands. The table's CHECK admits a
		// third rung, 'veto', that nothing writes yet — and rewriting it to
		// 'manual' here would be this surface quietly disagreeing with
		// AutoApplyMode, which returns what the row holds. A rep on veto would
		// then read "asks every time" on the screen while the applier read
		// something else, and the screen would be the one that was wrong.
		//
		// A kind that LEAVES AutoApplyKinds needs no defence here: the loop only
		// visits kinds still in the set, so a row for a dropped kind is never
		// reached, and AutoApplyMode refuses it on its own way in.
		settings = append(settings, row)
	}
	return settings, nil
}

// SetAutoApply turns automatic application on or off for one rep and one kind.
//
// NO OBJECT GATE, for the same reason the read has none: it writes the row of
// the principal on the context and takes no user id, so a rep cannot put a
// colleague on automatic — there is no cross-user write for a grant to
// authorize. `approval` is not a core RBAC object, so there is no grant to
// check even if one were wanted.
//
// The row is upserted because a rep may choose before they have ever decided
// this kind — turning a suggestion off is a reasonable first move, and it must
// not depend on having earned a counter row first.
//
// A kind outside AutoApplyKinds has no setting to write, and says so with
// ErrNotFound rather than storing the row. Storing it would record a preference
// the product will not honour, and the next reader would have to decide whether
// to trust it; refusing says so while the caller can still do something about
// it. Not a new sentinel: the registry is extended alongside the error contract
// it implements, never for one call site.
//
// It answers with the rep's WHOLE settings, read in the same transaction as the
// write. Returning nothing and letting the caller read again would leave a gap
// where the write has committed and the answer has not: the rep is told the
// change failed, believes automatic application is off, and it is on. That is
// the one direction this setting must not be wrong in, so the answer cannot
// come from a second transaction.
func (s *Service) SetAutoApply(ctx context.Context, kind string, on bool) ([]KindAutonomy, error) {
	if !AutoApplyKinds[kind] {
		return nil, fmt.Errorf("%q does not apply automatically, so it has no setting: %w",
			kind, apperrors.ErrNotFound)
	}
	rep, ok := principal.Actor(ctx)
	if !ok || rep.UserID.IsZero() {
		return nil, fmt.Errorf("a policy belongs to a person, and this call names none: %w",
			apperrors.ErrPermissionDenied)
	}
	mode := ModeManual
	if on {
		mode = ModeAuto
	}
	var settings []KindAutonomy
	// veto_window stays NULL: the table's CHECK allows one only on 'veto', and
	// neither rung this writes waits.
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO approval_autonomy_policy (user_id, kind, mode)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (user_id, kind) DO UPDATE SET mode = EXCLUDED.mode`,
			rep.UserID, kind, string(mode))
		if err != nil {
			return fmt.Errorf("crmapprovals: setting the autonomy mode: %w", err)
		}
		settings, err = autonomySettingsInTx(ctx, tx, rep.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return settings, nil
}
