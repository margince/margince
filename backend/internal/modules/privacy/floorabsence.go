// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Whether this installation knows what the law requires it to keep.
//
// The floor fails OPEN, which is the right way round and the reason the
// absence has to be said out loud. A purge handed no clause shields every row
// (StatutoryFloor.column), so an installation with no applicable pack does not
// destroy correspondence it should have kept — it keeps everything and reports
// the reason as the statute. Both halves of that are wrong to leave silent:
// mail is held that nothing requires holding, and an owner asking why is told
// a law applied when none was established.

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// FloorPosture is what this installation can say about its statutory floor.
type FloorPosture struct {
	// Known reports that some compiled-in pack declares a commercial
	// correspondence floor. False means no pack does, which is a fact about
	// the BUILD rather than about the law: the periods exist, this binary
	// simply carries none of them.
	Known bool
	// Class and Keep describe the strictest floor when one is known, and are
	// empty otherwise. A reader is never shown a period beside a false Known.
	Class string
	Keep  jurisdiction.Period
	// FromYearEnd says the period counts from the end of the calendar year.
	FromYearEnd bool
	// Packs names every compiled-in jurisdiction, floor or no floor. An
	// installation with packs that all decline to declare a floor is a
	// different situation from one with no packs at all — the first has been
	// answered, the second has not been asked.
	Packs []string
}

// StatutoryFloorPosture reports what this installation knows about the floor,
// so an operator is TOLD rather than left to find out.
//
// The distinction the surface turns on is "asked and answered" versus "never
// asked". A pack that declares no correspondence class has made a statement —
// extensions/vn does exactly that, with its reasoning written out — and an
// installation carrying it is in a different position from one built with no
// packs at all, even though both have no floor to apply.
func StatutoryFloorPosture() FloorPosture {
	posture := FloorPosture{}
	for _, pack := range jurisdiction.Applicable() {
		posture.Packs = append(posture.Packs, string(pack.Code()))
	}
	floor := statutoryCorrespondenceFloor(time.Now())
	if floor.Name == "" {
		return posture
	}
	posture.Known = true
	posture.Class = string(floor.Name)
	posture.Keep = floor.Keep
	posture.FromYearEnd = floor.Anchor == jurisdiction.AnchorCalendarYearEnd
	return posture
}
