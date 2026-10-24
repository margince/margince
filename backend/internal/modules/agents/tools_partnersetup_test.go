// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type recordingPartnerSetUp struct {
	checkErr error
	setUps   []PartnerSetUpFields
}

func (p *recordingPartnerSetUp) Check(context.Context, ids.UUID) error { return p.checkErr }

func (p *recordingPartnerSetUp) SetUp(_ context.Context, _ ids.UUID, f PartnerSetUpFields) error {
	p.setUps = append(p.setUps, f)
	return nil
}

func setUpPartnerTool(partners PartnerSetUp, companyID ids.UUID) setUpPartner {
	return setUpPartner{
		p:        stubRecordProvider{rec: stagedRecord(datasource.EntityCompany, companyID, true)},
		partners: partners,
	}
}

func TestSetUpPartnerRefusesAMarginTierBeforeStagingAnything(t *testing.T) {
	companyID := ids.NewV7()
	partners := &recordingPartnerSetUp{}
	tool := setUpPartnerTool(partners, companyID)
	args := json.RawMessage(fmt.Sprintf(`{"company_id":%q,"partner_role":"hosting","margin_tier":"tier3_25"}`, companyID))

	_, stageErr := tool.StageInfo(context.Background(), args)
	_, handleErr := tool.Handle(context.Background(), args)

	for door, err := range map[string]error{"StageInfo": stageErr, "Handle": handleErr} {
		bad, ok := errors.AsType[*BadArgsError](err)
		if !ok || bad.Field != "margin_tier" || !strings.Contains(bad.Cause.Error(), "a human sets it on the company's Partner tab") {
			t.Errorf("%s err = %v, want the margin-tier refusal naming the Partner tab", door, err)
		}
	}
	if len(partners.setUps) != 0 {
		t.Errorf("a call naming a tier wrote %d programs", len(partners.setUps))
	}
}

func TestSetUpPartnerStagesOnlyWhatTheStoreWouldAccept(t *testing.T) {
	companyID := ids.NewV7()
	args := json.RawMessage(fmt.Sprintf(`{"company_id":%q,"partner_role":"hosting"}`, companyID))

	info, err := setUpPartnerTool(&recordingPartnerSetUp{}, companyID).StageInfo(context.Background(), args)
	if err != nil {
		t.Fatalf("StageInfo err = %v", err)
	}
	if info.TargetType != "company" || info.TargetID != companyID || info.TargetVersion == nil || *info.TargetVersion != 11 {
		t.Errorf("staged %+v, want the company pinned at the version read", info)
	}
	if !strings.Contains(info.Summary, "hosting") {
		t.Errorf("summary %q does not name the role", info.Summary)
	}

	refused := &recordingPartnerSetUp{checkErr: apperrors.ErrConflict}
	if _, err := setUpPartnerTool(refused, companyID).StageInfo(context.Background(), args); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a refusing check staged anyway: err = %v", err)
	}
}

func TestSetUpPartnerRefusesWordsOutsideTheContractsVocabularies(t *testing.T) {
	companyID := ids.NewV7()
	for field, body := range map[string]string{
		"partner_role":       `"partner_role":"reseller"`,
		"relationship_stage": `"partner_role":"hosting","relationship_stage":"won"`,
		"next_step_due_at":   `"partner_role":"hosting","next_step_due_at":"soon"`,
	} {
		args := json.RawMessage(fmt.Sprintf(`{"company_id":%q,%s}`, companyID, body))
		_, err := setUpPartnerTool(&recordingPartnerSetUp{}, companyID).Handle(context.Background(), args)
		if bad, ok := errors.AsType[*BadArgsError](err); !ok || bad.Field != field {
			t.Errorf("%s: err = %v, want a refusal naming the field", field, err)
		}
	}
}

func TestSetUpPartnerHandsTheStoreTheTermsAndNoTier(t *testing.T) {
	companyID := ids.NewV7()
	partners := &recordingPartnerSetUp{}
	args := json.RawMessage(fmt.Sprintf(
		`{"company_id":%q,"partner_role":"strategic","relationship_stage":"contacted","next_step":"call","next_step_due_at":"2026-11-30"}`, companyID))

	if _, err := setUpPartnerTool(partners, companyID).Handle(context.Background(), args); err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if len(partners.setUps) != 1 {
		t.Fatalf("store called %d times, want 1", len(partners.setUps))
	}
	got := partners.setUps[0]
	if got.Role != "strategic" || *got.Stage != "contacted" || *got.NextStep != "call" || got.NextStepDueAt.Format("2006-01-02") != "2026-11-30" {
		t.Errorf("store got %+v", got)
	}
}
