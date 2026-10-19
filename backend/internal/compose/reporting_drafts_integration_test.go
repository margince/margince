// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// draftsFixture is one company-wide report, an editor, and a rep who reads it
// but may not edit it.
type draftsFixture struct {
	*reportingBusinessFixture
	report crmcontracts.ReportingReport
	reader context.Context
}

func reportWithReader(t *testing.T) draftsFixture {
	t.Helper()
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{
		Name: "Weekly review", Audience: "workspace", Selection: f.selection(),
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID = f.env.Rep3
	actor.ID = "human:" + f.env.Rep3.String()
	actor.TeamIDs = []ids.UUID{f.env.Team2}
	grants := maps.Clone(actor.Permissions.Objects)
	delete(grants, "team_oversight")
	actor.Permissions.Objects = grants
	return draftsFixture{reportingBusinessFixture: f, report: report, reader: principal.WithActor(f.human, actor)}
}

// shorter is the fixture's selection without its last chart, so a revision
// that holds it can be told from one that does not.
func (f draftsFixture) shorter() crmcontracts.ReportingSelection {
	selection := f.selection()
	selection.Blocks = selection.Blocks[:len(selection.Blocks)-1]
	return selection
}

func blocksOf(t *testing.T, report crmcontracts.ReportingReport, err error) []crmcontracts.ReportingBlockKind {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return report.Selection.Blocks
}

// A draft is the editors' until they publish it. A reader keeps the published
// revision, is not told the draft exists, and cannot evaluate it by number.
func TestADraftStaysWithItsEditorsUntilPublished(t *testing.T) {
	f := reportWithReader(t)
	id := ids.UUID(f.report.Id)
	drafted, err := f.service.SaveReportDraft(f.human, id, f.report.Version,
		crmcontracts.ReportingReportDraftInput{Selection: f.shorter()})
	if err != nil {
		t.Fatalf("saving a draft: %v", err)
	}
	if drafted.DraftRevision == nil || *drafted.DraftRevision != 2 || drafted.Revision != 1 {
		t.Fatalf("after a draft the report is at revision %d with draft %v, want 1 and 2", drafted.Revision, drafted.DraftRevision)
	}

	seen, err := f.service.GetReport(f.reader, id)
	if got := blocksOf(t, seen, err); !slices.Equal(got, f.selection().Blocks) || seen.DraftRevision != nil {
		t.Errorf("the reader sees %v with draft %v, want the published charts and no draft", got, seen.DraftRevision)
	}
	if _, err := f.service.EvaluateReport(f.reader, id, nil); err != nil {
		t.Fatalf("the reader cannot evaluate the published revision, so the draft check below proves nothing: %v", err)
	}
	draft := int64(2)
	if _, err := f.service.EvaluateReport(f.reader, id, &draft); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a reader evaluating the draft's number answered %v, want not found", err)
	}
	if _, err := f.service.EvaluateReport(f.human, id, &draft); err != nil {
		t.Errorf("the editor cannot preview their draft: %v", err)
	}
	if _, err := f.service.UpdateReport(f.human, id, drafted.Version, crmcontracts.ReportingReportInput{
		Name: "Weekly review", Audience: "workspace", Selection: f.selection(),
	}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a direct publish over an open draft answered %v, want a refusal", err)
	}

	published, err := f.service.PublishReportDraft(f.human, id, drafted.Version)
	if err != nil {
		t.Fatalf("publishing the draft: %v", err)
	}
	if published.Revision != 2 || published.DraftRevision != nil {
		t.Errorf("after publishing the report is at %d with draft %v, want 2 and none", published.Revision, published.DraftRevision)
	}
	seen, err = f.service.GetReport(f.reader, id)
	if got := blocksOf(t, seen, err); !slices.Equal(got, f.shorter().Blocks) {
		t.Errorf("after publishing the reader sees %v, want the draft's charts", got)
	}
}

// Discarding a draft removes it and changes nothing a reader sees.
func TestDiscardingADraftLeavesThePublishedRevision(t *testing.T) {
	f := reportWithReader(t)
	id := ids.UUID(f.report.Id)
	drafted, err := f.service.SaveReportDraft(f.human, id, f.report.Version,
		crmcontracts.ReportingReportDraftInput{Selection: f.shorter()})
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := f.service.DiscardReportDraft(f.human, id, drafted.Version)
	if err != nil {
		t.Fatalf("discarding: %v", err)
	}
	if discarded.DraftRevision != nil || discarded.Revision != 1 {
		t.Errorf("after discarding the report is at %d with draft %v, want 1 and none", discarded.Revision, discarded.DraftRevision)
	}
	revisions, err := f.service.ListReportRevisions(f.human, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions.Data) != 1 {
		t.Errorf("after discarding the report lists %d revisions, want the published one alone", len(revisions.Data))
	}
}

// Restoring an earlier revision publishes a copy of it as the newest one. The
// earlier revision stays as it was, so an edition made from it still reads it.
func TestRestoringAnEarlierRevisionPublishesACopyOfIt(t *testing.T) {
	f := reportWithReader(t)
	id := ids.UUID(f.report.Id)
	revised, err := f.service.UpdateReport(f.human, id, f.report.Version, crmcontracts.ReportingReportInput{
		Name: "Weekly review", Audience: "workspace", Selection: f.shorter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := f.service.RestoreReportRevision(f.human, id, revised.Version, 1)
	if err != nil {
		t.Fatalf("restoring revision 1: %v", err)
	}
	if restored.Revision != 3 || !slices.Equal(restored.Selection.Blocks, f.selection().Blocks) {
		t.Errorf("the restore published revision %d with %v, want 3 with the first revision's charts",
			restored.Revision, restored.Selection.Blocks)
	}

	revisions, err := f.service.ListReportRevisions(f.human, id)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, r := range revisions.Data {
		states = append(states, string(r.State))
		if r.CreatedBy.String() == "" {
			t.Errorf("revision %d names no author", r.Revision)
		}
	}
	if want := []string{"published", "earlier", "earlier"}; !slices.Equal(states, want) {
		t.Errorf("the revisions read %v newest first, want %v", states, want)
	}
	first := int64(1)
	if _, err := f.service.EvaluateReport(f.human, id, &first); err != nil {
		t.Errorf("the first revision no longer evaluates after the restore: %v", err)
	}
	if _, err := f.service.RestoreReportRevision(f.human, id, restored.Version, 3); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("restoring the current revision answered %v, want a refusal", err)
	}
}

// The revision history is the editors'. A reader who may not edit the report
// is refused it.
func TestOnlyAnEditorListsAReportsRevisions(t *testing.T) {
	f := reportWithReader(t)
	if _, err := f.service.ListReportRevisions(f.reader, ids.UUID(f.report.Id)); err == nil {
		t.Error("a reader who may not edit the report listed its revisions")
	}
}
