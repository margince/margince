// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/extcapability"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// Two tasks of unit "u": one interactive and one background. The registry is
// process-wide and the tests here run in parallel, so they are registered once.
const (
	askTask     = "ext_u_ask"
	nightlyTask = "ext_u_nightly"
)

func init() {
	err := ai.RegisterExtensionTasks([]ai.ExtensionTask{
		{Task: askTask, Unit: "u", DisplayName: "Ask", Summary: "Answers a question.", Ladder: []ai.Tier{ai.TierCheapCloud}, Sites: []string{"ask"}},
		{Task: nightlyTask, Unit: "u", DisplayName: "Nightly", Summary: "Summarises the day.", Ladder: []ai.Tier{ai.TierCheapCloud}, Background: true, Sites: []string{"nightly"}},
	})
	if err != nil {
		panic(err)
	}
}

func laneFor(l extcapability.Lane) func(ai.Task) extcapability.Lane {
	return func(ai.Task) extcapability.Lane { return l }
}

// subjectLane records the subject the call ran under.
type subjectLane struct{ subject ai.Subject }

func (l *subjectLane) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	l.subject, _ = ai.SubjectOf(ctx)
	return model.Response{Text: "ok"}, nil
}

func TestATaskTheUnitDidNotDeclareIsRefusedAndNeverReachesTheLane(t *testing.T) {
	t.Parallel()
	for name, task := range map[string]string{
		"unknown":          "ext_u_missing",
		"another units":    "ext_other_ask",
		"a core task":      string(ai.TaskSummarize),
		"the removed task": "extension_call",
		"empty":            "",
	} {
		l := &lane{}
		models := extcapability.Models{Lane: laneFor(l), Unit: "u", Gate: allow, Trace: func(context.Context, ai.Task) error { return nil }}
		if _, err := models.Complete(context.Background(), task, oneMessage); !errors.Is(err, extension.ErrNotDeclared) {
			t.Errorf("%s: err = %v, want ErrNotDeclared", name, err)
		}
		if l.calls != 0 {
			t.Errorf("%s: the lane was reached", name)
		}
	}
}

func TestAJobTickOrBusDeliveryMayCallABackgroundTaskOnly(t *testing.T) {
	t.Parallel()
	l := &lane{}
	models := extcapability.Models{
		Lane: laneFor(l), Unit: "u", Unattended: true, Gate: allow,
		Trace: func(context.Context, ai.Task) error { return nil },
	}
	if _, err := models.Complete(context.Background(), askTask, oneMessage); !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("an unattended call to an interactive task: err = %v, want ErrForbidden", err)
	}
	if l.calls != 0 {
		t.Fatal("the lane was reached by an unattended call to an interactive task")
	}
	if text, err := models.Complete(context.Background(), nightlyTask, oneMessage); err != nil || text != "ok" {
		t.Fatalf("an unattended call to a background task = %q, %v, want the lane's answer", text, err)
	}
	models.Unattended = false
	if _, err := models.Complete(context.Background(), askTask, oneMessage); err != nil {
		t.Fatalf("an attended call to an interactive task: %v", err)
	}
}

func TestTheTraceNamesTheTaskTheCallRanUnder(t *testing.T) {
	t.Parallel()
	var traced ai.Task
	models := extcapability.Models{
		Lane: laneFor(&lane{}), Unit: "u", Gate: allow,
		Trace: func(_ context.Context, task ai.Task) error { traced = task; return nil },
	}
	if _, err := models.Complete(context.Background(), askTask, oneMessage); err != nil || traced != askTask {
		t.Fatalf("traced %q, err %v, want %q", traced, err, askTask)
	}
}

func TestACallNamesItsRecordOnTheCallLogOnlyWhenItNamesOne(t *testing.T) {
	t.Parallel()
	contact, other := ids.NewV7(), ids.NewV7()
	ref := func(id ids.UUID) extension.EntityRef { return extension.EntityRef{Type: "contact", ID: id.String()} }
	for name, tc := range map[string]struct {
		about []extension.EntityRef
		want  ids.UUID
	}{
		"one record":  {[]extension.EntityRef{ref(contact)}, contact},
		"two records": {[]extension.EntityRef{ref(contact), ref(other)}, ids.Nil},
		"no record":   {nil, ids.Nil},
	} {
		l := &subjectLane{}
		models := extcapability.Models{Lane: laneFor(l), Unit: "u", Gate: allow, Trace: func(context.Context, ai.Task) error { return nil }}
		req := extension.CompleteRequest{Messages: oneMessage.Messages, About: tc.about, NoContactData: "none"}
		if tc.about != nil {
			req.NoContactData = ""
		}
		if _, err := models.Complete(context.Background(), askTask, req); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if l.subject.Ref.ID != tc.want || (tc.want != ids.Nil && l.subject.Ref.Type != "contact") {
			t.Errorf("%s: call ran under subject %+v, want %s", name, l.subject.Ref, tc.want)
		}
	}
}
