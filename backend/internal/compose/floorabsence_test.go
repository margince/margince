// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the installation says at boot about the floor it applies.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

func announced(posture privacy.FloorPosture) string {
	var out bytes.Buffer
	announceStatutoryFloor(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})), posture)
	return out.String()
}

// The four cases an operator must tell apart each say which they are.
func TestTheBootLineTellsTheFourFloorCasesApart(t *testing.T) {
	sixYears := jurisdiction.RetentionClass{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}}
	for _, c := range []struct {
		name    string
		posture privacy.FloorPosture
		want    []string
	}{
		{
			name:    "not set",
			posture: privacy.FloorPosture{Known: true, Class: "commercial_correspondence", Keep: sixYears.Keep, Packs: []string{"de", "vn"}},
			want:    []string{"level=INFO", "no jurisdiction selected", "strictest compiled-in"},
		},
		{
			name:    "set, no floor",
			posture: privacy.FloorPosture{Selected: "vn", Packs: []string{"de", "vn"}},
			want:    []string{"level=WARN", "selected jurisdiction's pack declares none", "jurisdiction=vn"},
		},
		{
			name:    "set, with floor",
			posture: privacy.FloorPosture{Selected: "de", Known: true, Class: "commercial_correspondence", Keep: sixYears.Keep},
			want:    []string{"level=INFO", "in force", "jurisdiction=de"},
		},
		{
			name: "carried",
			posture: privacy.StatutoryFloorPosture(privacy.FloorPolicy{
				Selected: "vn",
				Carried:  &privacy.CarriedFloor{Class: sixYears, Before: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
			}),
			want: []string{"carried retention floor", "binds_until=2032-03-01"},
		},
		{
			name:    "nothing compiled in",
			posture: privacy.FloorPosture{},
			want:    []string{"level=WARN", "no jurisdiction packs compiled in"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			line := announced(c.posture)
			for _, want := range c.want {
				if !strings.Contains(line, want) {
					t.Errorf("boot line %q does not contain %q", line, want)
				}
			}
		})
	}
}

// A unit-test wiring has no logger and no pool. The reporting path must not
// panic there.
func TestAnnouncingTheFloorWithNoLoggerIsSilentRatherThanFatal(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("announcing the floor with no logger panicked: %v", r)
		}
	}()
	announceStatutoryFloor(nil, privacy.FloorPosture{})
	announceStatutoryFloorFrom(nil, nil)
}

// The posture never shows a period beside a false Known.
func TestAPolicyWithNoFloorCarriesNoPeriod(t *testing.T) {
	posture := privacy.StatutoryFloorPosture(privacy.FloorPolicy{Selected: "vn"})
	if posture.Known || posture.Class != "" || posture.Keep.String() != "P0D" {
		t.Fatalf("a policy with no floor reports %+v, want no class and no period", posture)
	}
}
