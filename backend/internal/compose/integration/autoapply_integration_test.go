// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Applying a proposal because its owner said to, and the four cases where it
// must not.
//
// Every claim here is SQL: whose policy was consulted, whether that person is
// still live, and what the decision wrote. A unit test with hand-built rows
// could not fail on any of them — and the refusals are the half that matters,
// because a sweep that applied nothing would pass a refusal-only suite while
// doing nothing at all. So the admit case runs beside them.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stageCtx is the server-proposal context the sweeps stage under: a system
// principal, which is what makes the row server-proposed (passport_id NULL) and
// therefore eligible to apply at all.
func stageCtx(e *Env) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:close-date-sweep",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// stageCloseDateCorrection stages one proposal of an auto-eligible kind against
// a deal, through the real staging path.
func stageCloseDateCorrection(t *testing.T, svc *approvals.Service, e *Env, deal ids.UUID) ids.ApprovalID {
	t.Helper()
	// Marshalled from the module's OWN type, never hand-written JSON: the effect
	// unmarshals into this struct, so a literal that drifted from it would stage
	// a payload the product never produces and fail somewhere unrelated.
	proposal, err := json.Marshal(deals.CloseDateCorrection{
		DealID:            ids.From[ids.DealKind](deal),
		ExpectedCloseDate: "2026-12-01",
		Basis:             "the date has passed and the deal is still open",
	})
	if err != nil {
		t.Fatalf("marshalling the proposal: %v", err)
	}
	id, err := svc.Stage(stageCtx(e), approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: proposal,
		DiffHash:       "h-" + ids.NewV7().String(),
		TargetType:     "deal",
		TargetID:       deal,
		Summary:        "the close date has passed",
	})
	if err != nil {
		t.Fatalf("staging the proposal: %v", err)
	}
	return id
}

// grantDealRepRole gives the owner REAL stored grants.
//
// An auto-apply resolves its authority from the database through
// EffectiveAuthority, not from the permissions a test binds onto a context, so
// a rep with no role_assignment resolves to no grants and every apply refuses.
// That refusal looks exactly like the product working — which is why the admit
// case has to grant a role the same way an installation does, and why the
// refusal cases below are only worth anything beside it.
//
// The grants are the ones a close-date confirmation actually spends: the deal
// it rewrites, and the installation settings the effect reads. Narrower than a
// shipped rep role on purpose — an admin-shaped fixture would pass whatever the
// effect asked for and prove nothing about what an ordinary rep's automatic
// apply can reach.
func grantDealRepRole(t *testing.T, e *Env, user ids.UUID) {
	t.Helper()
	roleKey := "autoapplyrep-" + user.String()[:8]
	e.WsExec(t, `INSERT INTO role (key, name, permissions)
		VALUES ($1, 'Auto-apply Rep', $2::jsonb)`,
		roleKey,
		`{"objects":{"deal":{"read":true,"update":true},`+
			`"installation_settings":{"read":true}},"row_scope":"all"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, $1 FROM role r WHERE r.key = $2`,
		user, roleKey)
}

// turnAutoApplyOn records the rep's own standing answer, through the real
// writer rather than an INSERT: the row a hand-written fixture produces is one
// the product may never write, and a test seeded that way proves nothing about
// the path a rep's click takes.
func turnAutoApplyOn(t *testing.T, svc *approvals.Service, e *Env, rep ids.UUID) {
	t.Helper()
	repCtx := e.As(rep, []ids.UUID{e.Team1}, RepPerms)
	if err := svc.SetAutoApply(repCtx, deals.CloseDateCorrectionKind, true); err != nil {
		t.Fatalf("turning auto-apply on: %v", err)
	}
}

// sweepCtx is what the job binds: the workspace, and no acting principal of its
// own. The pass binds the OWNER per row, so a principal here would only mask
// which authority each apply actually ran under.
func sweepCtx(e *Env) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func statusOf(t *testing.T, approvalID ids.ApprovalID) (string, bool) {
	t.Helper()
	owner := OwnerConn(t)
	var status string
	var bySystem bool
	if err := owner.QueryRow(context.Background(),
		`SELECT status, decided_by_system FROM approval WHERE id = $1`,
		approvalID).Scan(&status, &bySystem); err != nil {
		t.Fatalf("reading the decided row: %v", err)
	}
	return status, bySystem
}

// The admit case. A proposal whose owner has this kind on automatic applies
// without anybody being asked, and the row says the SYSTEM decided it — which
// is what lets the day's "Done for you" lane report it honestly rather than
// putting a rep's name on a click they never made.
func TestAProposalAppliesWhenItsOwnerSaidSo(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)
	turnAutoApplyOn(t, svc, e, e.Rep1)

	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied %d proposals, want the one its owner said to apply", applied)
	}
	status, bySystem := statusOf(t, approvalID)
	if status != "approved" {
		t.Errorf("status = %q, want approved", status)
	}
	if !bySystem {
		t.Error("the row does not say the system decided it, so the receipt would name a person who was never asked")
	}
}

// A rep who has said nothing is a rep who wants to be asked. 'manual' is the
// default, and the absence of a policy row must read as it — otherwise the
// first proposal of a kind would behave unlike every one after it.
func TestAProposalWaitsWhenNobodyOptedIn(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)

	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied %d proposals, want none — nobody opted in", applied)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Errorf("status = %q, want the proposal still waiting for a person", status)
	}
}

// An unowned record is nobody's standing answer. Applying it would be applying
// under an authority nobody currently holds, which is the one thing the owner
// resolution exists to prevent.
func TestAProposalOnAnUnownedRecordDoesNotApply(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Nobody's deal", pipeline, open, nil)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)
	turnAutoApplyOn(t, svc, e, e.Rep1)

	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied %d proposals, want none — the record has no owner", applied)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Errorf("status = %q, want the proposal still waiting", status)
	}
}

// An owner who has left cannot authorize anything. The policy row survives them
// — it is their old answer — so the refusal has to come from resolving their
// authority at APPLY time, not from the absence of a preference.
func TestAProposalWhoseOwnerHasLeftDoesNotApply(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)
	turnAutoApplyOn(t, svc, e, e.Rep1)

	// Grants and policy are both in place, so a suspension is the ONE thing
	// standing between this proposal and applying.
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep1); err != nil {
		t.Fatalf("suspending the owner: %v", err)
	}

	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied %d proposals, want none — the owner is no longer live", applied)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Errorf("status = %q, want the proposal left for a person to answer", status)
	}
}

// A kind outside the reversible set never applies, whatever a stored row says.
// The set is the product's promise that an automatic change can be put back, so
// a row naming a kind outside it is inert rather than honoured.
func TestAnIneligibleKindNeverApplies(t *testing.T) {
	e := Setup(t)
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	svc := approvals.NewService(e.DB())

	if err := svc.SetAutoApply(repCtx, "send_email", true); err == nil {
		t.Fatal("an ineligible kind was accepted as an auto-apply setting, so the product stored a promise it will not keep")
	}
	if mode, err := svc.AutoApplyMode(repCtx, "send_email"); err != nil || mode != approvals.ModeManual {
		t.Errorf("mode = %q (err %v), want manual for a kind that never applies", mode, err)
	}
}

// The premise the whole feature rests on: an automatic change can be put back.
//
// Auto-apply is allowed to happen without asking BECAUSE the rep can undo it,
// so a change the restore path cannot reverse is not a change this may make.
// Nothing new reverses it — the audit row the apply wrote goes back through the
// same record-history restore a person's Undo button uses, which is the point
// of computing reversibility rather than storing a flag beside the approval.
//
// An org rename rather than a close date, and deliberately: a confirmed close
// date currently cannot be undone at all, because its audit image records a
// timestamp against a date column and every restore reads that as superseded.
// That is a defect in the close-date effect rather than in this path, filed
// separately — proving the premise here needs a kind whose image round-trips.
func TestAnAutomaticChangeCanBePutBack(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	svc := approvals.NewService(e.DB())
	org := e.SeedOrg(t, "Weber GmbH", &e.Rep1)
	grantOrgRepRole(t, e, e.Rep1)
	// A promotion only overrides a name the DOMAIN produced — a name a person
	// typed outranks a signature, and the store refuses to touch it. The seed
	// leaves another source, so the fixture states the precondition the
	// promotion is actually about rather than silently proving nothing.
	e.WsExec(t, `UPDATE organization SET name_source = 'domain' WHERE id = $1`, org)

	proposal, err := json.Marshal(map[string]any{
		"organization_id":   org,
		"current_name":      "Weber GmbH",
		"proposed_name":     "Weber Fahrzeugtechnik GmbH",
		"proposed_name_key": "weber fahrzeugtechnik gmbh",
	})
	if err != nil {
		t.Fatalf("marshalling the proposal: %v", err)
	}
	approvalID, err := svc.Stage(stageCtx(e), approvals.StageInput{
		Kind:           "org_name_promotion",
		ProposedChange: proposal,
		DiffHash:       "h-" + ids.NewV7().String(),
		TargetType:     "organization",
		TargetID:       org,
		Summary:        "the signature spells the company differently",
	})
	if err != nil {
		t.Fatalf("staging the proposal: %v", err)
	}
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if err := svc.SetAutoApply(repCtx, "org_name_promotion", true); err != nil {
		t.Fatalf("turning auto-apply on: %v", err)
	}

	if applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool); err != nil || applied != 1 {
		t.Fatalf("applied %d (err %v), want the proposal applied", applied, err)
	}
	if status, bySystem := statusOf(t, approvalID); status != "approved" || !bySystem {
		t.Fatalf("status %q bySystem %v, want an approved row marked as the system's", status, bySystem)
	}

	// The audit row the APPLY wrote, found the way the history screen finds it.
	// The write is recorded against a MACHINE and carries the owner it acted
	// for. Which machine is the effect's own business — an org rename stamps
	// its provenance as the signature it read, not as the pass that released
	// it — but no automatic write may be recorded as a person having typed it,
	// because the receipts lane's whole claim is that nobody was asked.
	var auditID ids.UUID
	var actor string
	var onBehalfOf *ids.UUID
	if err := owner.QueryRow(context.Background(), `
		SELECT id, actor_id, on_behalf_of FROM audit_log
		 WHERE entity_type = 'organization' AND entity_id = $1 AND action = 'update'
		 ORDER BY occurred_at DESC LIMIT 1`, org).Scan(&auditID, &actor, &onBehalfOf); err != nil {
		t.Fatalf("finding the audit row the apply wrote: %v", err)
	}
	if strings.HasPrefix(actor, "human:") {
		t.Errorf("the change is recorded against %q — a person is named for a write nobody was asked about", actor)
	}
	if onBehalfOf == nil || *onBehalfOf != e.Rep1 {
		t.Errorf("on_behalf_of = %v, want the owner whose policy authorized it", onBehalfOf)
	}

	var version int64
	if err := owner.QueryRow(context.Background(),
		`SELECT version FROM organization WHERE id = $1`, org).Scan(&version); err != nil {
		t.Fatalf("reading the record version: %v", err)
	}

	// A PERSON undoes it, through the record-history restore a rep's Undo
	// button uses. The route is human-only, which is the other half of the
	// bargain: the machine may apply without asking, and only somebody who can
	// see the record may put it back.
	// The undoing rep's authority is RESOLVED, not declared. The machine got
	// its grants from role_assignment through EffectiveAuthority, so a
	// hand-written Permissions here would put the person on a different footing
	// and the test would pass even if grantOrgRepRole granted the wrong thing.
	// Same call, same source, both sides.
	rbac, seat, err := identity.NewService(e.Pool).EffectiveAuthority(
		principal.WithWorkspaceID(context.Background(), e.WS), e.WS, e.Rep1)
	if err != nil {
		t.Fatalf("resolving the rep's own authority: %v", err)
	}
	undoCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	undoCtx = principal.WithActor(undoCtx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          principal.HumanIDPrefix + e.Rep1.String(),
		UserID:      e.Rep1,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	undoCtx = principal.WithCorrelationID(undoCtx, ids.NewV7())
	seam := compose.NewRestoreSeam(e.Pool, compose.NewDispatcher(
		compose.NewProvider(e.Pool), nil, e.Pool))
	if _, err := seam.Restore(undoCtx, "organization", org, auditID, version); err != nil {
		t.Fatalf("undoing what the product applied on its own: %v", err)
	}
	var name string
	if err := owner.QueryRow(context.Background(),
		`SELECT display_name FROM organization WHERE id = $1`, org).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Weber GmbH" {
		t.Errorf("after the undo the record reads %q, want the name it had before the apply", name)
	}
}

// grantOrgRepRole grants what an org-name promotion spends, the same way an
// installation grants a role — see grantDealRepRole for why a bound context's
// permissions are not enough.
func grantOrgRepRole(t *testing.T, e *Env, user ids.UUID) {
	t.Helper()
	roleKey := "autoapplyorg-" + user.String()[:8]
	e.WsExec(t, `INSERT INTO role (key, name, permissions)
		VALUES ($1, 'Auto-apply Org Rep', $2::jsonb)`,
		roleKey,
		`{"objects":{"organization":{"read":true,"update":true},`+
			`"installation_settings":{"read":true}},"row_scope":"all"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, $1 FROM role r WHERE r.key = $2`,
		user, roleKey)
}

// A proposal that can NEVER apply must not park the ones behind it.
//
// The batch is ordered oldest first, so a row that fails every time sits at the
// head of every tick until it expires. A close-date proposal pins the deal
// version it was staged against, and anybody editing that deal afterwards makes
// its apply fail permanently — an ordinary edit, not a race. Ending the pass
// there would let one edited deal hold every other rep's automation, so the
// sweep steps over it and keeps going.
//
// "Stranded" is about the WRITE, not the decision: the decision commits and the
// effect then fails, exactly as it does when a person clicks approve on a stale
// pin. What this holds is that the failure stays with its own row.
func TestOneUnapplyableProposalDoesNotParkTheRest(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	grantDealRepRole(t, e, e.Rep1)
	turnAutoApplyOn(t, svc, e, e.Rep1)

	// Staged first, so it leads the batch.
	stuck := e.SeedDeal(t, "Edited since", pipeline, open, &e.Rep1)
	stuckApproval := stageCloseDateCorrection(t, svc, e, stuck)
	// The pin the real close-date sweep stages with, and then the edit that
	// strands it: the deal moves on and the staged version no longer matches.
	if _, err := owner.Exec(context.Background(),
		`UPDATE approval SET target_version = (SELECT version FROM deal WHERE id = $2)
		  WHERE id = $1`, stuckApproval, stuck); err != nil {
		t.Fatalf("pinning the staged version: %v", err)
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE deal SET version = version + 1 WHERE id = $1`, stuck); err != nil {
		t.Fatalf("editing the deal the proposal pinned: %v", err)
	}

	behind := e.SeedDeal(t, "Behind it", pipeline, open, &e.Rep1)
	behindApproval := stageCloseDateCorrection(t, svc, e, behind)

	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("one stranded proposal ended the whole pass: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied %d, want the one behind the stranded proposal", applied)
	}
	if status, _ := statusOf(t, behindApproval); status != "approved" {
		t.Errorf("the proposal behind the stranded one is %q, want approved", status)
	}
	// The stranded row does not vanish quietly. Its decision commits and its
	// effect then fails, which is the same shape a human's click produces on a
	// stale pin — and effect_failure is what makes it findable rather than a
	// row that silently never took.
	var failure *string
	if err := owner.QueryRow(context.Background(),
		`SELECT effect_failure FROM approval WHERE id = $1`, stuckApproval).Scan(&failure); err != nil {
		t.Fatal(err)
	}
	if failure == nil {
		t.Error("the stranded proposal records no failure, so nothing would ever surface it to a person")
	}
}
