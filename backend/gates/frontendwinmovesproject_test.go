// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The won dialog tells the reader that winning also moves the deal's project.
// It decides that from its own copy of the phases StartDeliveryForWonDeal moves
// out of, and of the phase it moves into. A phase on one side only makes the
// dialog promise a move the server skips, or keep quiet about one it makes.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendWinMovesProject = "../frontend/src/features/deals/deal360/winmovesproject.ts"

var (
	tsWinPhases     = regexp.MustCompile(`(?s)MOVED_BY_A_WIN: readonly ProjectPhase\[\] = \[(.*?)\];`)
	tsWinTarget     = regexp.MustCompile(`MOVED_INTO_BY_A_WIN = ["']([a-z_]+)["'] satisfies ProjectPhase;`)
	tsQuotedPhase   = regexp.MustCompile(`["']([a-z_]+)["']`)
	tsPhaseComments = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)
)

func TestTheWonDialogMirrorsThePhasesAWinMoves(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(frontendWinMovesProject)
	if err != nil {
		t.Fatalf("reading the won dialog's phase list: %v", err)
	}
	source := string(raw)
	list := tsWinPhases.FindStringSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares MOVED_BY_A_WIN as a ProjectPhase array literal; "+
			"this gate cannot judge a list it cannot find", frontendWinMovesProject)
	}
	var inTS []string
	for _, m := range tsQuotedPhase.FindAllStringSubmatch(tsPhaseComments.ReplaceAllString(list[1], " "), -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no phases parsed out of MOVED_BY_A_WIN: a gate that reads nothing agrees with everything")
	}
	inGo := projects.PhasesAWinAdvances()
	for _, phase := range inGo {
		if !slices.Contains(inTS, phase) {
			t.Errorf("a win moves a project out of %q, and the won dialog keeps quiet about it", phase)
		}
	}
	for _, phase := range inTS {
		if !slices.Contains(inGo, phase) {
			t.Errorf("the won dialog says a win moves a project out of %q, and the server leaves it there", phase)
		}
	}

	target := tsWinTarget.FindStringSubmatch(source)
	switch {
	case target == nil:
		t.Errorf("%s no longer declares MOVED_INTO_BY_A_WIN as a ProjectPhase literal", frontendWinMovesProject)
	case target[1] != projects.PhaseDelivering:
		t.Errorf("the won dialog names %q as where the project goes, and the server moves it to %q",
			target[1], projects.PhaseDelivering)
	}
	if strings.Count(source, "MOVED_BY_A_WIN") < 2 {
		t.Errorf("%s declares MOVED_BY_A_WIN and never reads it, so the dialog decides from something else", frontendWinMovesProject)
	}
}
