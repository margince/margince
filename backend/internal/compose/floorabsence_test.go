// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the installation says at boot about the law it is applying.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

// The absence is stated, not left to be discovered. Which branch this build
// takes depends on its compiled-in packs, so the assertion follows the posture
// rather than asserting one — what it holds is that EVERY posture says
// something, and that a build with no floor says so at WARN.
func TestTheInstallationSaysWhetherItKnowsItsStatutoryFloor(t *testing.T) {
	var out bytes.Buffer
	announceStatutoryFloor(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))

	line := out.String()
	if line == "" {
		t.Fatal("the installation said nothing about its statutory floor")
	}
	posture := privacy.StatutoryFloorPosture()
	if posture.Known {
		if !strings.Contains(line, "in force") || !strings.Contains(line, posture.Class) {
			t.Fatalf("a known floor is not named in %q", line)
		}
		return
	}
	// Not a configuration preference: mail is kept that nothing requires
	// keeping, and the reason given for keeping it names a law nobody
	// established.
	if !strings.Contains(line, "level=WARN") {
		t.Fatalf("an absent floor was not reported as a warning: %q", line)
	}
	if !strings.Contains(line, "no statutory retention floor") {
		t.Fatalf("the absence is not stated in %q", line)
	}
}

// A build with no observability is a unit-test wiring, and several route-level
// tests construct one. It must not panic on the reporting path — a wiring gap
// should be learned about where the wiring is, not through a nil dereference
// inside a log line.
//
// The recover is the assertion: without it this test could only fail by
// crashing the run, which reports the wrong thing about the wrong test.
func TestAnnouncingTheFloorWithNoLoggerIsSilentRatherThanFatal(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("announcing the floor with no logger panicked: %v", r)
		}
	}()
	announceStatutoryFloor(nil)
}

// The posture never shows a period beside a false Known: a reader handed
// "6 years" under "no floor known" would reasonably believe the first.
func TestAnUnknownFloorCarriesNoPeriod(t *testing.T) {
	posture := privacy.StatutoryFloorPosture()
	if posture.Known {
		if posture.Class == "" {
			t.Fatal("a known floor names no class")
		}
		return
	}
	if posture.Class != "" || posture.Keep.String() != "P0D" {
		t.Fatalf("an unknown floor carries %q / %s, want neither", posture.Class, posture.Keep)
	}
}
