// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// A reply this site refuses must be re-asked, not silently floored.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// reAskingLane is a lane that CAN re-ask, which in production is routerBrain.
// It records the reading it was handed rather than acting on it: the retry
// policy belongs to ai.Router.CompleteStructured and is tested there, and a
// fake that re-implemented it here would prove only that the fake works.
type reAskingLane struct {
	answer string
	read   ai.Validator
	plain  int
}

func (l *reAskingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.plain++
	return model.Response{Text: l.answer}, nil
}

func (l *reAskingLane) CompleteValidated(
	_ context.Context, _ model.Request, validate ai.Validator,
) (model.Response, error) {
	l.read = validate
	return model.Response{Text: l.answer}, nil
}

// The site hands its own reading down to the lane, so a reply it would refuse
// buys a re-ask that names the refusal.
//
// Without this the site called the plain lane, refused the reply itself, and
// degraded to the deterministic floor with the model never told what was
// wrong — which fails identically for every obedient model on every request,
// with nothing red anywhere. The reply surface never had the defect because it
// reached for this lane; these two did not.
func TestARefusedReplyIsReAskedRatherThanFloored(t *testing.T) {
	lane := &reAskingLane{answer: `{"subject":"Next steps","body":"Hi Sarah,\n\nShall we pick this up?"}`}

	if _, _, err := Write(context.Background(), lane, sampleInput(), draftvoice.Context{}); err != nil {
		t.Fatal(err)
	}
	if lane.plain > 0 {
		t.Errorf("the plain lane was used %d time(s), so a refused reply buys no re-ask", lane.plain)
	}
	if lane.read == nil {
		t.Fatal("no reading reached the lane, so a refused reply buys no re-ask")
	}

	// The reading must be this site's OWN, not a looser schema check beside
	// it: an empty subject is schema-valid JSON and is exactly what a model
	// returned on the shipped defect.
	if lane.read(`{"subject":"","body":""}`) == nil {
		t.Error("the reading accepts an empty subject and body, which this site refuses")
	}
	if err := lane.read(`{"subject":"Next steps","body":"Hi Sarah,\n\nShall we pick this up?"}`); err != nil {
		t.Errorf("the reading refuses a reply this site accepts: %v", err)
	}
}
