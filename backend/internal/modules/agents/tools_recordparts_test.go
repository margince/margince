// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// change_record_parts' actions: which seam method each reaches, with which
// arguments, and what each refuses. The writes behind the seam are proven over
// a real database in compose's record-part door suites.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingParts keeps the one seam call it was asked, spelled as method and
// arguments, and answers that spelling.
type recordingParts struct{ call string }

func (p *recordingParts) record(method string, args ...any) (json.RawMessage, error) {
	p.call = fmt.Sprintln(append([]any{method}, args...)...)
	return json.Marshal(p.call)
}

func (p *recordingParts) AddOfferLine(_ context.Context, offer ids.UUID, line json.RawMessage) (json.RawMessage, error) {
	return p.record("AddOfferLine", offer, string(line))
}

func (p *recordingParts) UpdateOfferLine(_ context.Context, offer, line ids.UUID, fields json.RawMessage) (json.RawMessage, error) {
	return p.record("UpdateOfferLine", offer, line, string(fields))
}

func (p *recordingParts) RemoveOfferLine(_ context.Context, offer, line ids.UUID) (json.RawMessage, error) {
	return p.record("RemoveOfferLine", offer, line)
}

func (p *recordingParts) CreateCompanyFact(_ context.Context, company ids.UUID, fact json.RawMessage) (json.RawMessage, error) {
	return p.record("CreateCompanyFact", company, string(fact))
}

func (p *recordingParts) UpdateCompanyFact(_ context.Context, company ids.UUID, key, value string, ifVersion *int64) (json.RawMessage, error) {
	return p.record("UpdateCompanyFact", company, key, value, versionOf(ifVersion))
}

func (p *recordingParts) ConfirmCompanyFact(_ context.Context, company ids.UUID, key string, ifVersion *int64) (json.RawMessage, error) {
	return p.record("ConfirmCompanyFact", company, key, versionOf(ifVersion))
}

func (p *recordingParts) DeleteCompanyFact(_ context.Context, company ids.UUID, key string, ifVersion *int64) (json.RawMessage, error) {
	return p.record("DeleteCompanyFact", company, key, versionOf(ifVersion))
}

func (p *recordingParts) UpdateCompanyProfileField(_ context.Context, company ids.UUID, field, value string, ifVersion *int64) (json.RawMessage, error) {
	return p.record("UpdateCompanyProfileField", company, field, value, versionOf(ifVersion))
}

func (p *recordingParts) ConfirmCompanyProfileField(_ context.Context, company ids.UUID, field string, ifVersion *int64) (json.RawMessage, error) {
	return p.record("ConfirmCompanyProfileField", company, field, versionOf(ifVersion))
}

func (p *recordingParts) SetProjectCompany(_ context.Context, project, company ids.UUID, role *string) (json.RawMessage, error) {
	named := "<default>"
	if role != nil {
		named = *role
	}
	return p.record("SetProjectCompany", project, company, named)
}

func (p *recordingParts) RemoveProjectCompany(_ context.Context, project, company ids.UUID) (json.RawMessage, error) {
	return p.record("RemoveProjectCompany", project, company)
}

func (p *recordingParts) SetProjectStakeholder(_ context.Context, project, contact ids.UUID, role string) (json.RawMessage, error) {
	return p.record("SetProjectStakeholder", project, contact, role)
}

func (p *recordingParts) RemoveProjectStakeholder(_ context.Context, project, contact ids.UUID) (json.RawMessage, error) {
	return p.record("RemoveProjectStakeholder", project, contact)
}

func versionOf(v *int64) string {
	if v == nil {
		return "<unpinned>"
	}
	return fmt.Sprint(*v)
}

const (
	partsOffer   = "019ff000-0000-7000-8000-0000000000a1"
	partsLine    = "019ff000-0000-7000-8000-0000000000a2"
	partsCompany = "019ff000-0000-7000-8000-0000000000a3"
	partsProject = "019ff000-0000-7000-8000-0000000000a4"
	partsContact = "019ff000-0000-7000-8000-0000000000a5"
)

// fullPartArgs names every argument an action can need, so one action's call
// is these narrowed to what it takes.
var fullPartArgs = map[string]any{
	partOfferID: partsOffer, partLineItemID: partsLine, partLine: map[string]any{"quantity": 2},
	partCompanyID: partsCompany, partProjectID: partsProject, partContactID: partsContact,
	partFactKey: "employee_range:11-50", partCategory: "company", partField: "employee_range",
	partValue: "11-50", partRole: "sponsor",
}

// argsFor is the call an action makes: its own needs, and nothing else.
func argsFor(t *testing.T, action string, extra map[string]any) json.RawMessage {
	t.Helper()
	args := map[string]any{actionArg: action}
	for _, need := range recordPartActions[action] {
		args[need] = fullPartArgs[need]
	}
	maps.Copy(args, extra)
	out, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding %s's arguments: %v", action, err)
	}
	return out
}

func TestEachRecordPartActionReachesItsOwnSeamMethod(t *testing.T) {
	pinned := map[string]any{"if_version": 4}
	for _, c := range []struct {
		action string
		extra  map[string]any
		want   string
	}{
		{"add_offer_line", nil, "AddOfferLine " + partsOffer + ` {"quantity":2}`},
		{"update_offer_line", nil, "UpdateOfferLine " + partsOffer + " " + partsLine + ` {"quantity":2}`},
		{"remove_offer_line", nil, "RemoveOfferLine " + partsOffer + " " + partsLine},
		{"add_company_fact", nil, "CreateCompanyFact " + partsCompany + ` {"category":"company","field":"employee_range","value":"11-50"}`},
		{"update_company_fact", pinned, "UpdateCompanyFact " + partsCompany + " employee_range:11-50 11-50 4"},
		{"confirm_company_fact", nil, "ConfirmCompanyFact " + partsCompany + " employee_range:11-50 <unpinned>"},
		{"remove_company_fact", pinned, "DeleteCompanyFact " + partsCompany + " employee_range:11-50 4"},
		{"update_company_profile_field", nil, "UpdateCompanyProfileField " + partsCompany + " employee_range 11-50 <unpinned>"},
		{"confirm_company_profile_field", pinned, "ConfirmCompanyProfileField " + partsCompany + " employee_range 4"},
		{"set_project_company", nil, "SetProjectCompany " + partsProject + " " + partsCompany + " <default>"},
		{"set_project_company", map[string]any{partRole: "vendor"}, "SetProjectCompany " + partsProject + " " + partsCompany + " vendor"},
		{"remove_project_company", nil, "RemoveProjectCompany " + partsProject + " " + partsCompany},
		{"set_project_stakeholder", nil, "SetProjectStakeholder " + partsProject + " " + partsContact + " sponsor"},
		{"remove_project_stakeholder", nil, "RemoveProjectStakeholder " + partsProject + " " + partsContact},
	} {
		parts := &recordingParts{}
		got, err := changeRecordParts{parts: parts}.Handle(context.Background(), argsFor(t, c.action, c.extra))
		if err != nil {
			t.Errorf("%s refused: %v", c.action, err)
			continue
		}
		if parts.call != c.want+"\n" {
			t.Errorf("%s reached\n  %q\nwant\n  %q", c.action, parts.call, c.want+"\n")
		}
		var answered string
		if err := json.Unmarshal(got, &answered); err != nil || answered != parts.call {
			t.Errorf("%s answered %s, not the seam's own answer", c.action, got)
		}
	}
}

// An action missing one argument it needs is refused naming that argument, and
// the seam is never reached.
func TestARecordPartActionMissingAnArgumentNamesIt(t *testing.T) {
	for action, needs := range recordPartActions {
		for _, need := range needs {
			args := map[string]any{}
			if err := json.Unmarshal(argsFor(t, action, nil), &args); err != nil {
				t.Fatalf("decoding %s's arguments: %v", action, err)
			}
			delete(args, need)
			in, err := json.Marshal(args)
			if err != nil {
				t.Fatalf("encoding %s's arguments: %v", action, err)
			}
			parts := &recordingParts{}
			_, err = changeRecordParts{parts: parts}.Handle(context.Background(), in)
			bad, ok := errors.AsType[*BadArgsError](err)
			if !ok || bad.Field != need {
				t.Errorf("%s without %s answered %v, want a refusal naming %s", action, need, err, need)
			}
			if parts.call != "" {
				t.Errorf("%s without %s still reached the seam: %s", action, need, parts.call)
			}
		}
	}
}

func TestARecordPartActionTheToolDoesNotKnowIsRefused(t *testing.T) {
	for _, args := range []string{`{"action":"merge_offer_lines"}`, `{}`} {
		parts := &recordingParts{}
		_, err := changeRecordParts{parts: parts}.Handle(context.Background(), json.RawMessage(args))
		bad, ok := errors.AsType[*BadArgsError](err)
		if !ok || bad.Field != actionArg {
			t.Errorf("%s answered %v, want a refusal naming action", args, err)
		}
		if parts.call != "" {
			t.Errorf("%s reached the seam: %s", args, parts.call)
		}
	}
}

// A seam failure reaches the caller as the seam said it, not as an answer.
func TestARecordPartSeamFailureIsTheToolsAnswer(t *testing.T) {
	_, err := changeRecordParts{parts: seamProbeRecordParts{}}.Handle(context.Background(), argsFor(t, "remove_project_stakeholder", nil))
	if !errors.Is(err, errSeamReached) {
		t.Errorf("a failing seam answered %v, want its own error", err)
	}
}
