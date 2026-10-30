// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lossPipeline is a pipeline with one open and one lost stage.
type lossPipeline struct {
	pipeline   ids.PipelineID
	open, lost ids.StageID
}

func seedLossPipeline(t *testing.T, e *configEnv) lossPipeline {
	t.Helper()
	ctx := e.as()
	pipeline, err := e.store.CreatePipeline(ctx, CreatePipelineInput{Name: "Sales " + ids.NewV7().String()})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	out := lossPipeline{pipeline: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))}
	zero := 0
	for position, stage := range []struct {
		name     string
		semantic StageSemantic
		into     *ids.StageID
	}{{"Discovery", SemanticOpen, &out.open}, {"Lost", SemanticLost, &out.lost}} {
		made, err := e.store.CreateStage(ctx, CreateStageInput{
			PipelineID: out.pipeline, Name: stage.name, Position: position,
			Semantic: string(stage.semantic), WinProbability: &zero,
		})
		if err != nil {
			t.Fatalf("seeding stage %s: %v", stage.name, err)
		}
		*stage.into = ids.From[ids.StageKind](ids.UUID(made.Id))
	}
	return out
}

func (p lossPipeline) deal(t *testing.T, e *configEnv, name string) ids.DealID {
	t.Helper()
	owner := ids.From[ids.UserKind](e.admin)
	made, err := e.store.CreateDeal(e.asLossAdmin(), CreateDealInput{
		Name: name, PipelineID: p.pipeline, StageID: p.open, OwnerID: &owner, OwnerExact: true,
	})
	if err != nil {
		t.Fatalf("seeding deal %s: %v", name, err)
	}
	return ids.From[ids.DealKind](ids.UUID(made.Id))
}

// asLossAdmin may write deals and the loss reason list.
func (e *configEnv) asLossAdmin() context.Context {
	return e.withObjects(map[string]principal.ObjectGrant{
		"deal":         {Create: true, Read: true, Update: true},
		"pipeline":     {Create: true, Read: true, Update: true},
		"custom_field": {Create: true, Read: true, Update: true},
	})
}

func (e *configEnv) withObjects(objects map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll, Objects: objects},
	})
}

func refusedAs[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

// lostRows answers the code each move to lost recorded on the deal's history,
// oldest first.
func lostRows(t *testing.T, e *configEnv, deal ids.DealID) []*string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `SELECT lost_reason_code_at_change FROM deal_stage_history
		WHERE deal_id = $1 AND semantic_at_change = 'lost' ORDER BY changed_at, id`, deal)
	if err != nil {
		t.Fatalf("reading stage history: %v", err)
	}
	codes, err := pgx.CollectRows(rows, pgx.RowTo[*string])
	if err != nil {
		t.Fatalf("reading stage history: %v", err)
	}
	return codes
}

func TestALossRecordsItsCodeOnTheDealTheMoveAndTheEvent(t *testing.T) {
	e := setupConfigEnv(t)
	p := seedLossPipeline(t, e)
	deal := p.deal(t, e, "Fleet renewal")

	got, err := e.store.AdvanceDeal(e.asLossAdmin(), deal, AdvanceDealInput{ToStageID: p.lost, LostReasonCode: new("price")})
	if err != nil {
		t.Fatalf("losing with a code and no note: %v", err)
	}
	if got.LostReasonCode == nil || *got.LostReasonCode != "price" || got.LostReasonBasis == nil || string(*got.LostReasonBasis) != lossBasisRecorded {
		t.Fatalf("the deal reads code %v basis %v, want price recorded", got.LostReasonCode, got.LostReasonBasis)
	}
	if got.LostReason != nil {
		t.Errorf("a loss with no note stored the note %q", *got.LostReason)
	}
	if codes := lostRows(t, e, deal); len(codes) != 1 || codes[0] == nil || *codes[0] != "price" {
		t.Errorf("the move recorded %v, want one loss coded price", codes)
	}
	var published string
	if err := e.owner.QueryRow(context.Background(), `SELECT envelope->'payload'->>'lost_reason_code' FROM event_outbox
		WHERE envelope->>'type' = 'deal.stage_changed' AND envelope->'entity'->>'id' = $1::text`, deal).Scan(&published); err != nil {
		t.Fatalf("reading the stage change event: %v", err)
	}
	if published != "price" {
		t.Errorf("deal.stage_changed carries lost_reason_code %q, want price", published)
	}
}

// Reopening clears the deal's reason, and losing it again records the second
// answer beside the first rather than over it. The note the first loss carried
// survives in the audit log.
func TestReopeningAndLosingAgainKeepsBothLosses(t *testing.T) {
	e := setupConfigEnv(t)
	p := seedLossPipeline(t, e)
	deal := p.deal(t, e, "Depot tooling")
	ctx := e.asLossAdmin()

	if _, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.lost, LostReasonCode: new("price"), LostReason: new("Too dear this year")}); err != nil {
		t.Fatalf("first loss: %v", err)
	}
	reopened, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.open})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if reopened.LostReasonCode != nil || reopened.LostReasonBasis != nil || reopened.LostReason != nil {
		t.Fatalf("a reopened deal still carries code %v basis %v note %v", reopened.LostReasonCode, reopened.LostReasonBasis, reopened.LostReason)
	}
	if _, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.lost, LostReasonCode: new("competitor")}); err != nil {
		t.Fatalf("second loss: %v", err)
	}
	codes := lostRows(t, e, deal)
	if len(codes) != 2 || codes[0] == nil || *codes[0] != "price" || codes[1] == nil || *codes[1] != "competitor" {
		t.Fatalf("history holds %v, want price then competitor", codes)
	}
	var kept int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
		WHERE entity_id = $1 AND before->>'lost_reason' = 'Too dear this year'`, deal).Scan(&kept); err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	if kept != 1 {
		t.Errorf("the reopen's before-image kept the first note %d times, want once", kept)
	}
}

func TestEveryLossDoorRefusesWhatTheOneCheckRefuses(t *testing.T) {
	e := setupConfigEnv(t)
	p := seedLossPipeline(t, e)
	ctx := e.asLossAdmin()
	reasons, err := e.store.ListLossReasons(ctx)
	if err != nil {
		t.Fatalf("listing reasons: %v", err)
	}
	var timing ids.UUID
	for _, reason := range reasons {
		if reason.Key == "timing" {
			timing = ids.UUID(reason.Id)
		}
	}
	if _, err := e.store.UpdateLossReason(ctx, timing, UpdateLossReasonInput{Active: new(false)}); err != nil {
		t.Fatalf("retiring timing: %v", err)
	}

	cases := []struct {
		name string
		in   AdvanceDealInput
		want func(error) bool
	}{
		{
			name: "a code the list does not hold",
			in:   AdvanceDealInput{ToStageID: p.lost, LostReasonCode: new("weather")},
			want: refusedAs[*UnknownLossReasonError],
		},
		{
			name: "a retired code",
			in:   AdvanceDealInput{ToStageID: p.lost, LostReasonCode: new("timing")},
			want: refusedAs[*RetiredLossReasonError],
		},
		{
			name: "neither a code nor a note",
			in:   AdvanceDealInput{ToStageID: p.lost, LostReason: new("  ")},
			want: refusedAs[*LostReasonRequiredError],
		},
		{
			name: "a code on a move that is not a loss",
			in:   AdvanceDealInput{ToStageID: p.open, LostReasonCode: new("price")},
			want: refusedAs[*LossReasonNotLostError],
		},
	}
	for _, c := range cases {
		deal := p.deal(t, e, c.name)
		if c.in.ToStageID == p.open {
			if _, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.lost, LostReason: new("set up")}); err != nil {
				t.Fatalf("%s: losing first: %v", c.name, err)
			}
		}
		if _, err := e.store.AdvanceDeal(ctx, deal, c.in); !c.want(err) {
			t.Errorf("%s: answered %v", c.name, err)
		}
	}

	legacy := p.deal(t, e, "Old client")
	got, err := e.store.AdvanceDeal(ctx, legacy, AdvanceDealInput{ToStageID: p.lost, LostReason: new("Went with the incumbent")})
	if err != nil {
		t.Fatalf("a client that sends only the note: %v", err)
	}
	if got.LostReasonCode != nil || got.LostReason == nil {
		t.Errorf("a note-only loss reads code %v note %v, want the note and no code", got.LostReasonCode, got.LostReason)
	}
}

func lossCode(t *testing.T, e *configEnv, deal ids.DealID) (code, basis *string) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT lost_reason_code, lost_reason_basis FROM deal WHERE id = $1`, deal).Scan(&code, &basis); err != nil {
		t.Fatalf("reading the deal's code: %v", err)
	}
	return code, basis
}

// legacyNote finds one note on the legacy list. The lane database is shared by
// the package's tests, so the list holds other tests' notes too.
func legacyNote(t *testing.T, e *configEnv, text string) crmcontracts.LegacyLossReason {
	t.Helper()
	listed, err := e.store.ListLegacyLossReasons(e.asLossAdmin(), MaxLegacyLossReasons)
	if err != nil {
		t.Fatalf("listing legacy notes: %v", err)
	}
	for _, row := range listed.Data {
		if strings.EqualFold(strings.TrimSpace(row.Text), text) {
			return row
		}
	}
	t.Fatalf("the legacy list does not hold %q: %+v", text, listed.Data)
	return crmcontracts.LegacyLossReason{}
}

// applyMappings runs the apply step a row at a time over the one decision a
// test made, as its current version.
func applyMappings(t *testing.T, e *configEnv, decided crmcontracts.LossReasonMapping) int {
	t.Helper()
	ctx := e.asLossAdmin()
	reviewed := map[ids.UUID]int64{ids.UUID(decided.Id): decided.Version}
	changed := 0
	err := e.store.Tx(ctx, func(tx pgx.Tx) error {
		after := ids.Nil
		for {
			scanned, n, err := ApplyLossReasonMappingsTx(ctx, tx, after, 1, reviewed)
			if err != nil {
				return err
			}
			changed += n
			if len(scanned) == 0 {
				return nil
			}
			after = scanned[len(scanned)-1]
		}
	})
	if err != nil {
		t.Fatalf("applying the mappings: %v", err)
	}
	return changed
}

// A reviewed mapping reaches every lost deal whose note matches it and keeps
// the note as it was. It never replaces a code the closer chose, and a second
// run changes nothing.
func TestAReviewedMappingAppliesOnceAndNeverOverwritesAChoice(t *testing.T) {
	e := setupConfigEnv(t)
	p := seedLossPipeline(t, e)
	ctx := e.asLossAdmin()
	lose := func(name, note string, code *string) ids.DealID {
		deal := p.deal(t, e, name)
		if _, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.lost, LostReason: new(note), LostReasonCode: code}); err != nil {
			t.Fatalf("losing %s: %v", name, err)
		}
		return deal
	}
	first := lose("North", "Far too expensive", nil)
	second := lose("South", "  far too EXPENSIVE ", nil)
	chosen := lose("East", "far too expensive", new("competitor"))

	note := legacyNote(t, e, "far too expensive")
	if note.DealCount != 2 {
		t.Fatalf("the note counts %d deals, want the two with no code", note.DealCount)
	}
	decided, err := e.store.DecideLegacyLossReason(ctx, note.TextHash, "price")
	if err != nil {
		t.Fatalf("deciding the note: %v", err)
	}
	if changed := applyMappings(t, e, decided); changed != 2 {
		t.Fatalf("the first run changed %d deals, want 2", changed)
	}
	for _, deal := range []ids.DealID{first, second} {
		if code, basis := lossCode(t, e, deal); code == nil || *code != "price" || basis == nil || *basis != lossBasisMapped {
			t.Errorf("deal %v reads %v/%v, want price mapped", deal, code, basis)
		}
	}
	if code, basis := lossCode(t, e, chosen); code == nil || *code != "competitor" || *basis != lossBasisRecorded {
		t.Errorf("the closer's choice became %v/%v", code, basis)
	}
	var kept string
	if err := e.owner.QueryRow(context.Background(), `SELECT lost_reason FROM deal WHERE id = $1`, second).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != "far too EXPENSIVE" {
		t.Errorf("the note became %q, want it as the closer wrote it", kept)
	}
	if changed := applyMappings(t, e, decided); changed != 0 {
		t.Errorf("a second run changed %d deals, want none", changed)
	}

	corrected, err := e.store.DecideLegacyLossReason(ctx, note.TextHash, "no_budget")
	if err != nil {
		t.Fatalf("correcting the decision: %v", err)
	}
	if changed := applyMappings(t, e, corrected); changed != 2 {
		t.Errorf("a corrected decision moved %d deals, want the 2 it mapped", changed)
	}
}

// A mapping whose decision moved after the review is not applied from the old
// review: the administrator applied what they saw.
func TestAStaleReviewAppliesNothing(t *testing.T) {
	e := setupConfigEnv(t)
	p := seedLossPipeline(t, e)
	ctx := e.asLossAdmin()
	deal := p.deal(t, e, "West")
	if _, err := e.store.AdvanceDeal(ctx, deal, AdvanceDealInput{ToStageID: p.lost, LostReason: new("No money left this quarter")}); err != nil {
		t.Fatal(err)
	}
	decided, err := e.store.DecideLegacyLossReason(ctx, legacyNote(t, e, "no money left this quarter").TextHash, "price")
	if err != nil {
		t.Fatal(err)
	}
	stale := map[ids.UUID]int64{ids.UUID(decided.Id): decided.Version - 1}
	err = e.store.Tx(ctx, func(tx pgx.Tx) error {
		_, changed, err := ApplyLossReasonMappingsTx(ctx, tx, ids.Nil, 100, stale)
		if err == nil && changed != 0 {
			t.Errorf("a stale review changed %d deals", changed)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOnlyASeatThatMayDecideSeesTheLegacyNotes(t *testing.T) {
	e := setupConfigEnv(t)
	reader := e.withObjects(map[string]principal.ObjectGrant{
		"deal": {Read: true}, "custom_field": {Read: true},
	})
	if _, err := e.store.ListLegacyLossReasons(reader, 50); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader of the list saw the legacy notes: %v", err)
	}
	if _, err := e.store.DecideLegacyLossReason(reader, "00", "price"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader of the list decided a mapping: %v", err)
	}
}
