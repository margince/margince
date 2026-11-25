// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"strings"
	"testing"
	"time"
)

func validJobDeclaration() JobDeclaration {
	return JobDeclaration{
		Unit:              "crm-hello",
		Job:               "refresh_quotes",
		Queue:             "default",
		Cadence:           6 * time.Hour,
		DispatcherTimeout: time.Minute,
		Timeout:           5 * time.Minute,
		MaxAttempts:       3,
		Tier:              TierAutoExecute,
		RequestedScope:    ScopeRead,
	}
}

// TestJobKindsAreTheUnitNamespaceAndTheJobName: the two kinds a scheduled job
// compiles to are derived, never declared twice — and the hyphen a unit name
// may carry becomes the underscore a River kind can hold, exactly as it does
// for a SQL identifier.
func TestJobKindsAreTheUnitNamespaceAndTheJobName(t *testing.T) {
	d := validJobDeclaration()
	if got, want := d.DispatcherKind(), "ext_crm_hello_refresh_quotes"; got != want {
		t.Errorf("dispatcher kind: got %q, want %q", got, want)
	}
	if got, want := d.ChildKind(), "ext_crm_hello_refresh_quotes_ws"; got != want {
		t.Errorf("child kind: got %q, want %q", got, want)
	}
}

// TestJobKindsAreEmptyForAnInvalidUnit: a kind derived from a namespace that is
// not one is not a kind. Answering the empty string rather than a
// plausible-looking string is what makes Validate the thing that reports it,
// instead of an insert failing later under a name nobody declared.
func TestJobKindsAreEmptyForAnInvalidUnit(t *testing.T) {
	d := validJobDeclaration()
	d.Unit = "Not A Unit"
	if got := d.DispatcherKind(); got != "" {
		t.Errorf("dispatcher kind for an invalid unit: got %q, want the empty string", got)
	}
	if got := d.ChildKind(); got != "" {
		t.Errorf("child kind for an invalid unit: got %q, want the empty string", got)
	}
}

func TestJobDeclarationValidateAcceptsAWellFormedJob(t *testing.T) {
	if err := validJobDeclaration().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// TestJobDeclarationValidateRefusesEveryAbsenceThatWouldReachRiverAsADefault:
// each field below is one River default the declaration exists to remove — a
// silent one-minute timeout, a 25-rung attempt ladder, a tick that never fires
// or never stops — so an omission has to be a refusal rather than a zero.
func TestJobDeclarationValidateRefusesEveryAbsenceThatWouldReachRiverAsADefault(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*JobDeclaration)
		want   string
	}{
		{"bad unit", func(d *JobDeclaration) { d.Unit = "Bad Unit" }, "not a valid unit name"},
		{"bad job name", func(d *JobDeclaration) { d.Job = "Refresh-Quotes" }, "not a valid job name"},
		{"no queue", func(d *JobDeclaration) { d.Queue = " " }, "declares no queue"},
		{"no cadence", func(d *JobDeclaration) { d.Cadence = 0 }, "declares cadence"},
		{"negative cadence", func(d *JobDeclaration) { d.Cadence = -time.Second }, "declares cadence"},
		{"cadence and a time of day", func(d *JobDeclaration) { d.At = &TimeOfDay{Minute: 450} }, "both a cadence and a time of day"},
		{"a minute past the day", func(d *JobDeclaration) { d.Cadence, d.At = 0, &TimeOfDay{Minute: minutesPerDay} }, "not a minute of the day"},
		{"a day that is no weekday", func(d *JobDeclaration) { d.Cadence, d.At = 0, &TimeOfDay{Days: []time.Weekday{7}} }, "not a weekday"},
		{"a day named twice", func(d *JobDeclaration) {
			d.Cadence, d.At = 0, &TimeOfDay{Days: []time.Weekday{time.Monday, time.Monday}}
		}, "named twice"},
		{"no dispatcher timeout", func(d *JobDeclaration) { d.DispatcherTimeout = 0 }, "no dispatcher timeout"},
		{"no workspace timeout", func(d *JobDeclaration) { d.Timeout = 0 }, "no workspace timeout"},
		{"no attempt cap", func(d *JobDeclaration) { d.MaxAttempts = 0 }, "max_attempts"},
		{"unknown tier", func(d *JobDeclaration) { d.Tier = "whenever" }, "is not one an extension may request"},
		{"unknown scope", func(d *JobDeclaration) { d.RequestedScope = "everything" }, "Passport scope vocabulary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := validJobDeclaration()
			tc.mutate(&d)
			err := d.Validate()
			if err == nil {
				t.Fatalf("Validate accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate(%s) = %v, want a message mentioning %q", tc.name, err, tc.want)
			}
		})
	}
}

// TestJobValidateChecksTheVerbGrammarAndNothingElse: a Jobs entry is behavior,
// and every other rule about a job is a rule about its DECLARATION, which is
// the contract's.
func TestJobValidateChecksTheVerbGrammarAndNothingElse(t *testing.T) {
	if err := (Job{Name: "refresh_quotes"}).Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// No handler is not a grammar failure: it is how the seam spells "declare
	// it, run nothing".
	if err := (Job{Name: "refresh_quotes", Handle: nil}).Validate(); err != nil {
		t.Fatalf("Validate of a handler-less job: %v", err)
	}
	for _, bad := range []string{"", "RefreshQuotes", "refresh-quotes", "_refresh", "refresh__quotes", "9lives"} {
		if err := (Job{Name: bad}).Validate(); err == nil {
			t.Errorf("Validate accepted job name %q", bad)
		}
	}
}

func TestAJobMayRunAtALocalClockTimeInsteadOfAnInterval(t *testing.T) {
	d := validJobDeclaration()
	d.Cadence, d.At = 0, &TimeOfDay{Minute: 7*60 + 30, Days: []time.Weekday{time.Monday, time.Friday}}
	if err := d.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestTheLongestGapIsTheLongestWaitBetweenTwoRuns(t *testing.T) {
	weekdays := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	for name, tc := range map[string]struct {
		days []time.Weekday
		want time.Duration
	}{
		"every day":     {nil, 24 * time.Hour},
		"weekdays":      {weekdays, 72 * time.Hour},
		"sundays":       {[]time.Weekday{time.Sunday}, 7 * 24 * time.Hour},
		"sat and sun":   {[]time.Weekday{time.Saturday, time.Sunday}, 6 * 24 * time.Hour},
		"mon and thurs": {[]time.Weekday{time.Monday, time.Thursday}, 4 * 24 * time.Hour},
	} {
		if got := (TimeOfDay{Days: tc.days}).LongestGap(); got != tc.want {
			t.Errorf("%s: LongestGap = %s, want %s", name, got, tc.want)
		}
	}
}
