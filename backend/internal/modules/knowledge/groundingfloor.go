// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// FloorMeasurement is what was measured for one embedding model: whether any
// cosine separates a covered question from an uncovered one, and where.
//
// A floor is a fact about a model, not a tunable. Cosine is not calibrated
// across models, and a provider that changes a model behind the same name moves
// every number here, so the date and the model beside the value are what let
// the next reader tell a stale entry from a current one.
type FloorMeasurement struct {
	// Model is the name the provider serves, as it appears in the embed
	// identity once the provider prefix and the width are removed.
	Model string
	// Floor is the cosine a passage must reach. Meaningful only when Separable.
	Floor float64
	// Separable is false when the covered and uncovered bands overlap, so no
	// floor exists for the model.
	Separable  bool
	MeasuredOn time.Time
	// Owner re-measures this entry when the binding or the model behind it
	// changes.
	Owner string
}

// measuredFloors is the registry. A binding absent from it has no floor: adding
// an embedding binding means measuring it and adding its row here.
var measuredFloors = []FloorMeasurement{
	{
		// Covered 0.72–0.84, uncovered 0.45–0.58 on a one-document corpus; the
		// bands do not touch and 0.65 splits them.
		Model: "gemini-embedding-001", Floor: 0.65, Separable: true,
		MeasuredOn: time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC), Owner: "ai-models",
	},
	{
		// A covered question scored 0.670 and an unrelated one 0.672 against
		// the same passage.
		Model: "mistralai/mistral-embed-2312", Separable: false,
		MeasuredOn: time.Date(2026, time.August, 28, 0, 0, 0, 0, time.UTC), Owner: "ai-models",
	},
}

// modelOfIdentity extracts the model from "<provider>/<model>@<dims>": the
// provider and the width both vary across installations serving one model.
func modelOfIdentity(identity string) string {
	model, _, _ := strings.Cut(identity, "@")
	_, model, _ = strings.Cut(model, "/")
	return model
}

// MeasurementFor returns the entry for the model an embed identity names.
func MeasurementFor(identity string) (FloorMeasurement, bool) {
	model := modelOfIdentity(identity)
	for _, m := range measuredFloors {
		if m.Model == model {
			return m, true
		}
	}
	return FloorMeasurement{}, false
}

// BindingFloor is the grounding floor an embed identity carries. A binding with
// no measured value, or whose bands overlap, has no floor: it returns 0, which
// removes nothing, and the writer that reads the passages decides coverage.
func BindingFloor(identity string) float64 {
	if m, ok := MeasurementFor(identity); ok && m.Separable {
		return m.Floor
	}
	return 0
}

// EffectiveFloor is the corpus's own override when it set one, else the
// binding's measured floor. The override is deliberately not clamped to the
// binding's floor: it is the workspace's corpus.
func EffectiveFloor(override *float64, identity string) float64 {
	if override != nil {
		return *override
	}
	return BindingFloor(identity)
}

// WarnIfUngated says at startup what an operator bound to this identity would
// otherwise learn from a wrong answer.
func WarnIfUngated(log *slog.Logger, identity string) {
	if identity == "" {
		return
	}
	m, ok := MeasurementFor(identity)
	switch {
	case !ok:
		log.Warn(fmt.Sprintf("embedding binding %s has no measured grounding floor, so a grounding floor cannot be enforced for corpora bound to it; measure it and add it to measuredFloors", identity))
	case !m.Separable:
		log.Warn(fmt.Sprintf("no floor separates covered from uncovered questions under %s (measured %s), so a grounding floor cannot be enforced for corpora bound to it",
			identity, m.MeasuredOn.Format(time.DateOnly)))
	}
}
