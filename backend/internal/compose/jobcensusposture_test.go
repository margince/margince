// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the posture check can and cannot see.
//
// The check itself passes on this tree, which is the state it is meant to hold
// — and a check that passes proves nothing about what it would catch. These
// plant the two divergences it exists for, plus the one way it could fail
// short: a declared dependency nobody taught it to withhold.

import (
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
)

// wiringWithout is the registration a role gets with one dependency missing.
func wiringWithout(t *testing.T, path string) (JobRunnerConfig, *jobRegistry) {
	t.Helper()
	cfg, known := withholdDependency(censusJobConfig(), path)
	if !known {
		t.Fatalf("withholdDependency does not know JobRunnerConfig.%s", path)
	}
	reg, _ := wireJobs(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	return cfg, reg
}

// A kind that declares `absent: registers_anyway` and is not registered is the
// defect this check was written for: the row is refused at insert, and the
// message names River's worker bundle rather than the missing configuration.
func TestAKindMissingWhereItsDeclarationSaysRegisterAnywayIsReported(t *testing.T) {
	t.Parallel()
	cfg, reg := wiringWithout(t, "TechnicalEnricher")
	if _, wired := reg.wired["technical_enrich_company"]; !wired {
		t.Fatal("technical_enrich_company is not registered without an enricher; the fixture below would prove nothing")
	}
	delete(reg.wired, "technical_enrich_company")

	findings := posturesDisagreeingWithout(cfg, reg, "TechnicalEnricher")

	if !mentions(findings, "technical_enrich_company", "is unregistered") {
		t.Errorf("a kind declared registers_anyway went missing and the check said %v", findings)
	}
}

// The other direction: a worker present for a kind its own declaration calls
// absent. The schedule half reads the declaration, so nothing would ever
// enqueue for it.
func TestAKindRegisteredWhereItsDeclarationSaysAbsentIsReported(t *testing.T) {
	t.Parallel()
	cfg, reg := wiringWithout(t, "Geocoder")
	if _, wired := reg.wired["geocode_backfill"]; wired {
		t.Fatal("geocode_backfill registers without a geocoder; its declaration says it should not")
	}
	reg.wired["geocode_backfill"] = wiredWorker{}

	findings := posturesDisagreeingWithout(cfg, reg, "Geocoder")

	if !mentions(findings, "geocode_backfill", "is registered") {
		t.Errorf("a kind declared absent was wired and the check said %v", findings)
	}
}

// The one way this check could fail short. A declaration naming a dependency
// withholdDependency has no entry for would be skipped, and every kind gated on
// it would go unasked about while the census still reported PASS.
func TestADependencyNobodyCanWithholdIsReportedRatherThanSkipped(t *testing.T) {
	t.Parallel()
	if _, known := withholdDependency(censusJobConfig(), "AConfigFieldNobodyAdded"); known {
		t.Fatal("withholdDependency claims to withhold a field that does not exist")
	}
}

// withholdDependency carries the same obligation as configDependencies beside
// it, held the same way: exactly the paths a declaration gates a kind on. A
// missing entry stops the posture walk asking about that dependency at all; a
// dead one is an entry nobody would notice.
func TestEveryGatedDependencyCanBeWithheld(t *testing.T) {
	t.Parallel()
	assertTableAnswersExactly(t, "dependencyWithholders",
		slices.Sorted(maps.Keys(dependencyWithholders())),
		slices.Sorted(maps.Keys(gatedDependencyPaths())))
}

// mentions answers whether some finding names both the kind and the direction.
func mentions(findings []string, kind, direction string) bool {
	for _, finding := range findings {
		if strings.HasPrefix(finding, kind+" ") && strings.Contains(finding, direction) {
			return true
		}
	}
	return false
}
