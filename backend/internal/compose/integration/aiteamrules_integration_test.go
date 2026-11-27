// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func teamRulesStore(e *Env) *ai.AdminStore {
	return ai.NewAdminStore(e.DB(), settings.New(e.Pool, settings.NewRegistry(ai.Definitions()...)), nil, nil, nil)
}

func teamRulesAuditRows(e *Env, t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'ai_team_rules'`)
}

func TestTeamRulesAreWrittenByAdminOnlyAuditedOnceAndGuardedAgainstAStaleSave(t *testing.T) {
	e := Setup(t)
	store := teamRulesStore(e)
	admin := e.Admin()
	// The seeded rep grant: read, no update.
	rep := e.As(e.Rep1, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_team_rules": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})

	empty, err := store.ReadTeamRules(rep)
	if err != nil || empty.Text != "" || empty.UpdatedAt != nil {
		t.Fatalf("a rep reads the rules, and none are saved yet: %+v %v", empty, err)
	}
	if _, err := store.ReplaceTeamRules(rep, crmcontracts.AiTeamRulesChange{Text: "Never quote a discount."}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep's save must be refused, got %v", err)
	}
	if rows := teamRulesAuditRows(e, t); rows != 0 {
		t.Fatalf("a refused save wrote %d audit rows", rows)
	}

	first, err := store.ReplaceTeamRules(admin, crmcontracts.AiTeamRulesChange{Text: "  Never quote a discount.  "})
	if err != nil || first.Text != "Never quote a discount." || first.UpdatedAt == nil {
		t.Fatalf("admin save: %+v %v", first, err)
	}
	if rows := teamRulesAuditRows(e, t); rows != 1 {
		t.Fatalf("one save wrote %d audit rows, want 1", rows)
	}

	if _, err := store.ReplaceTeamRules(admin, crmcontracts.AiTeamRulesChange{Text: "Another text."}); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a save by an editor who read no saved version must be refused as stale, got %v", err)
	}
	same, err := store.ReplaceTeamRules(admin, crmcontracts.AiTeamRulesChange{Text: first.Text, ExpectedUpdatedAt: first.UpdatedAt})
	if err != nil || teamRulesAuditRows(e, t) != 1 || !same.UpdatedAt.Equal(*first.UpdatedAt) {
		t.Fatalf("resaving the same text must change nothing: %+v %v", same, err)
	}
	second, err := store.ReplaceTeamRules(admin, crmcontracts.AiTeamRulesChange{Text: "Say Sie, never du.", ExpectedUpdatedAt: first.UpdatedAt})
	if err != nil || second.Text != "Say Sie, never du." {
		t.Fatalf("a save of the version the editor read: %+v %v", second, err)
	}
	if rows := teamRulesAuditRows(e, t); rows != 2 {
		t.Fatalf("two changes wrote %d audit rows, want 2", rows)
	}

	tooLong := crmcontracts.AiTeamRulesChange{Text: strings.Repeat("ä", ai.MaxTeamRulesRunes+1), ExpectedUpdatedAt: second.UpdatedAt}
	var invalid settings.InvalidValue
	if _, err := store.ReplaceTeamRules(admin, tooLong); !errors.As(err, &invalid) {
		t.Fatalf("%d characters must be refused as an invalid value, got %v", ai.MaxTeamRulesRunes+1, err)
	}
	tooLong.Text = strings.Repeat("ä", ai.MaxTeamRulesRunes)
	if _, err := store.ReplaceTeamRules(admin, tooLong); err != nil {
		t.Fatalf("%d characters is the bound and must be saved: %v", ai.MaxTeamRulesRunes, err)
	}
}

func TestADraftLoadsTheTeamRulesWithoutHoldingTheirGrant(t *testing.T) {
	e := Setup(t)
	if _, err := teamRulesStore(e).ReplaceTeamRules(e.Admin(), crmcontracts.AiTeamRulesChange{Text: "Never quote a discount."}); err != nil {
		t.Fatal(err)
	}
	voices := ai.NewVoiceStore(e.DB())
	for name, ctx := range map[string]context.Context{"agent": e.AgentCtx(), "system": e.AutomationCtx(e.Rep1)} {
		if loaded, _ := draftvoice.Load(ctx, voices, nil); loaded.TeamRules != "Never quote a discount." {
			t.Errorf("%s draft loaded team rules %q, want the saved text", name, loaded.TeamRules)
		}
	}
	if _, err := teamRulesStore(e).ReadTeamRules(e.AgentCtx()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the agent holds no read grant, so the answering read must refuse it: %v", err)
	}
}
