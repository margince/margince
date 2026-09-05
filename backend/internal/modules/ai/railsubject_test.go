// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func acmeRef() ids.Ref {
	return ids.Ref{Type: "organization", ID: ids.NewV7()}
}

// The subject is a fact about the calls under a context, so it must survive
// the trip and be absent — not zero-valued — where no site declared one.
func TestASubjectIsReadBackAsItWasDeclared(t *testing.T) {
	ref := acmeRef()
	got, ok := SubjectOf(WithSubject(context.Background(), ref, "Acme"))
	if !ok || got.Ref != ref || got.Label != "Acme" {
		t.Fatalf("SubjectOf = %+v, %v; want %v named Acme", got, ok, ref)
	}
	if _, ok := SubjectOf(context.Background()); ok {
		t.Error("a context nobody named a subject on reported one")
	}
	if _, ok := SubjectOf(WithSubject(context.Background(), ids.Ref{Type: "organization"}, "Acme")); ok {
		t.Error("a subject with no id is not a record, and must not be reported as one")
	}
}

// The wire admits 120 characters of label, and the cut has to land between
// characters: a company name is not ASCII, and a byte cut through an umlaut
// reaches the reader as a replacement glyph.
func TestALongLabelIsCutBetweenCharacters(t *testing.T) {
	long := strings.Repeat("ü", railSubjectLabelBound+7)
	got, _ := SubjectOf(WithSubject(context.Background(), acmeRef(), long))
	if !utf8.ValidString(got.Label) {
		t.Fatal("the cut label is not valid UTF-8")
	}
	if n := utf8.RuneCountInString(got.Label); n != railSubjectLabelBound {
		t.Errorf("the label was cut to %d characters, want %d", n, railSubjectLabelBound)
	}
	short := "Acme"
	if got, _ := SubjectOf(WithSubject(context.Background(), acmeRef(), short)); got.Label != short {
		t.Errorf("a label within the bound came back as %q", got.Label)
	}
}

// Both announcements stamp the subject, because the projection overwrites
// every column from the newest event: a settle that dropped it would strip
// the name from the row the instant the work finished.
func TestASubjectStampsTheAnnouncementOrLeavesItUntouched(t *testing.T) {
	ref := acmeRef()
	var named crmcontracts.InternalEventAiTaskStateChanged
	Subject{Ref: ref, Label: "Acme"}.stamp(&named)
	if named.SubjectType == nil || *named.SubjectType != "organization" {
		t.Errorf("subject_type = %v, want organization", named.SubjectType)
	}
	if named.SubjectId == nil || ids.UUID(*named.SubjectId) != ref.ID {
		t.Errorf("subject_id = %v, want %s", named.SubjectId, ref.ID)
	}
	if named.SubjectLabel == nil || *named.SubjectLabel != "Acme" {
		t.Errorf("subject_label = %v, want Acme", named.SubjectLabel)
	}

	// A record with no name is still a record: the rail draws its unnamed
	// sentence, and the projection still knows what the work was about.
	var unnamed crmcontracts.InternalEventAiTaskStateChanged
	Subject{Ref: ref}.stamp(&unnamed)
	if unnamed.SubjectId == nil || unnamed.SubjectLabel != nil {
		t.Errorf("an unnamed record stamped id=%v label=%v; want the id and no label", unnamed.SubjectId, unnamed.SubjectLabel)
	}

	var none crmcontracts.InternalEventAiTaskStateChanged
	Subject{}.stamp(&none)
	if none.SubjectType != nil || none.SubjectId != nil || none.SubjectLabel != nil {
		t.Error("no subject wrote subject fields onto the announcement")
	}
}

// The PRODUCTION path, end to end inside the router: the site names the
// subject on the context, and both the start the rail hears first and the
// terminal call the flush settles from carry it. Asserted on both halves
// because they are read at different moments by different code, and a name
// on one alone renders as a rail that forgets who it was talking about.
func TestServingACallCarriesItsSubjectToTheStartAndTheSettle(t *testing.T) {
	starter := &countingStarter{}
	r := assembleRouter(
		map[Tier]model.Client{TierCheapCloud: stubClient{resp: model.Response{Text: "answer"}}},
		nil, ProfileCloudFrontier, stubMeter{}, unlimitedBudget{}, starter,
		map[Tier]routeMeta{TierCheapCloud: {provider: "openai", model: "gpt-cheap"}},
		false, nil,
	)
	ref := acmeRef()
	ctx := WithSubject(principal.WithCorrelationID(wsCtx(), ids.NewV7()), ref, "Acme")

	if _, _, err := r.serveCompletion(ctx, TaskSummarize, []Tier{TierCheapCloud}, model.Request{}); err != nil {
		t.Fatalf("serving the call: %v", err)
	}

	want := Subject{Ref: ref, Label: "Acme"}
	if len(starter.starts) != 1 || starter.starts[0].Subject != want {
		t.Errorf("the start carried subject %+v, want %+v", starter.starts, want)
	}
	var terminal *Call
	for i := range starter.recorded {
		if starter.recorded[i].IsTerminal {
			terminal = &starter.recorded[i]
		}
	}
	if terminal == nil {
		t.Fatal("no terminal call was recorded")
	}
	if terminal.Subject != want {
		t.Errorf("the terminal call carried subject %+v, want %+v — the settle would announce an unnamed occurrence", terminal.Subject, want)
	}
}
