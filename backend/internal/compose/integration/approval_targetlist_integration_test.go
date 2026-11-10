// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The inbox filtered to the records one task touched. An agent asks it before
// it reports the work finished. It answers what those records left waiting,
// and still hides what the caller cannot decide.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestApprovalListFilteredToSeveralTargetsAnswersExactlyThose(t *testing.T) {
	e := Setup(t)
	svc := approvals.NewService(e.DB())
	first := e.SeedCompany(t, "First Account", &e.Rep1)
	second := e.SeedCompany(t, "Second Account", &e.Rep1)
	untouched := e.SeedCompany(t, "Untouched Account", &e.Rep1)
	onFirst := stageFor(t, svc, e, "deepread", "company", first)
	onSecond := stageFor(t, svc, e, "deepread", "company", second)
	onUntouched := stageFor(t, svc, e, "deepread", "company", untouched)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, siteReadPerms)

	got := listIDs(rep, t, svc, approvals.ListInput{Targets: []ids.Ref{
		{Type: "company", ID: first}, {Type: "company", ID: second},
		// The type is half of the reference: the untouched account's id under
		// another type names a different record.
		{Type: "contact", ID: untouched},
	}})
	if !got[onFirst] || !got[onSecond] {
		t.Errorf("the read is missing a staging on a named record: got %v", got)
	}
	if got[onUntouched] {
		t.Error("the read returned a staging on a record it did not name")
	}
	if len(got) != 2 {
		t.Errorf("returned %d approvals, want the 2 on the named records", len(got))
	}
}

// A named record the caller cannot see adds nothing. One hidden record must not
// empty or refuse the whole read.
func TestApprovalListFilteredToSeveralTargetsHidesAnOutOfScopeOne(t *testing.T) {
	e := Setup(t)
	svc := approvals.NewService(e.DB())
	mine := e.SeedCompany(t, "Mine", &e.Rep1)
	theirs := e.SeedCompany(t, "Other Team Account", &e.Rep3)
	e.MakeCapturePrivate(t, "company", theirs, e.Rep3)
	onMine := stageFor(t, svc, e, "deepread", "company", mine)
	onTheirs := stageFor(t, svc, e, "deepread", "company", theirs)
	stageFor(t, svc, e, "deepread", "company", e.SeedCompany(t, "Unnamed", &e.Rep1))
	targets := []ids.Ref{{Type: "company", ID: mine}, {Type: "company", ID: theirs}}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, siteReadPerms)
	got := listIDs(rep, t, svc, approvals.ListInput{Targets: targets})
	if !got[onMine] || got[onTheirs] || len(got) != 1 {
		t.Errorf("rep read %v, want only %s", got, onMine)
	}

	// The positive control: the owner sees the hidden one, so its absence above
	// is the visibility rule and not a broken filter.
	owner := e.As(e.Rep3, []ids.UUID{e.Team2}, siteReadPerms)
	if got := listIDs(owner, t, svc, approvals.ListInput{Targets: targets}); !got[onTheirs] {
		t.Errorf("the owning team's read is missing %s", onTheirs)
	}
}

// The tool door reaches the same filter. A record with nothing staged answers
// an empty list.
func TestListApprovalsWithTargetsAnswersOnlyTheNamedRecords(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "finishing agent", "read", "write", "enrich"))
	touched, onTouched := q.stageAConfirmFirstCall(t, invoke, "Touched Account")
	onOther := stageEnrichOn(t, q, invoke, "other.example.com")

	read := func(targets string) map[string]bool {
		t.Helper()
		out, err := invoke("list_approvals", `{"targets":`+targets+`}`)
		if err != nil {
			t.Fatalf("list_approvals with targets %s → %v", targets, err)
		}
		got := map[string]bool{}
		for _, item := range answered[struct {
			Approvals []queueItem `json:"approvals"`
		}](t, out).Approvals {
			got[item.StagedActionID] = true
		}
		return got
	}

	got := read(`[{"type":"company","id":"` + touched + `"}]`)
	if !got[onTouched.String()] || got[onOther.String()] || len(got) != 1 {
		t.Errorf("the touched record answered %v, want only %s", got, onTouched)
	}
	if got := read(`[{"type":"company","id":"` + ids.NewV7().String() + `"}]`); len(got) != 0 {
		t.Errorf("a record with nothing staged answered %v, want an empty list", got)
	}
}

// stageEnrichOn stages an enrich on a second company, under its own domain
// because two companies cannot share one.
func stageEnrichOn(t *testing.T, q *queueEnv, invoke func(tool, args string) (string, error), domain string) ids.UUID {
	t.Helper()
	var company struct {
		ID string `json:"id"`
	}
	if status := q.Call(t, "POST", "/v1/companies", AnyMap{"source": "manual", "display_name": domain,
		"domains": []AnyMap{{"domain": domain, "is_primary": true}}}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	_, err := invoke("enrich", `{"company_id":"`+company.ID+`"}`)
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("enrich → %v, want a staged approval", err)
	}
	return staged.ApprovalID.UUID
}
