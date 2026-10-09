// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"go/ast"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// specFor answers the declaration a test names, failing rather than returning
// a zero Spec: a zero Cadence and a zero Registration are both meaningful
// values here, so a silently missing kind would make a test pass for the
// wrong reason.
func specFor(t *testing.T, kind string) jobs.Spec {
	t.Helper()
	spec, ok := jobs.SpecFor(kind)
	if !ok {
		t.Fatalf("api/jobs.yaml does not declare %q", kind)
	}
	return spec
}

// TestASettingCadenceRunsAtTheSettingsValue pins that a {setting: …} cadence
// schedules from the book's value for that setting. Seven minutes is a value no
// default carries, so a resolver falling back to a default cannot produce it.
func TestASettingCadenceRunsAtTheSettingsValue(t *testing.T) {
	spec := specFor(t, CloseDateSweepArgs{}.Kind())
	book := newScheduleBook(map[string]time.Duration{spec.Cadence.Setting: 7 * time.Minute})
	got := periodicFor(JobRunnerConfig{Schedules: book}, CloseDateSweepArgs{})
	if len(got) != 1 {
		t.Fatalf("got %d periodic jobs, want 1", len(got))
	}
	// River keeps a job's schedule to itself, so the claim is held through
	// the book: the job handed out is the one the book built over its live
	// interval, and that interval is asked below.
	if got[0] != book.kinds[spec.Kind].job {
		t.Fatal("the job handed to River is not the one the book scheduled, so the book's interval says nothing about it")
	}
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if next := book.intervals[spec.Cadence.Setting].Next(start); next != start.Add(7*time.Minute) {
		t.Errorf("the next run after %s is %s, want seven minutes later", start, next)
	}
}

// TestTheReconcileIntervalMatchesTheStatedStalenessBound holds the graph
// projection's staleness promise to the cadence that makes it true.
func TestTheReconcileIntervalMatchesTheStatedStalenessBound(t *testing.T) {
	// The migration states that window counts may be up to 24h over-inclusive.
	// That promise is only true if this pass runs daily; loosening the declared
	// cadence without amending the migration would make the comment a lie. The
	// declaration is what River is handed, so it is what this asserts.
	spec := specFor(t, GraphEdgeReconcileArgs{}.Kind())
	if spec.Cadence.Fixed != 24*time.Hour {
		t.Errorf("declared reconcile cadence is %v, but the projection's stated bound is 24h", spec.Cadence.Fixed)
	}
}

// TestPeriodicForUsesTheDeclaredLiteralCadence proves the resolver is wired to
// the periodic entry, not merely callable.
func TestPeriodicForUsesTheDeclaredLiteralCadence(t *testing.T) {
	got := periodicFor(JobRunnerConfig{}, IdempotencyRetentionArgs{})
	if len(got) != 1 {
		t.Fatalf("got %d periodic jobs, want 1", len(got))
	}
}

func TestPeriodicForRegistersNothingWhenItsDependencyIsAbsent(t *testing.T) {
	got := periodicFor(JobRunnerConfig{}, GmailSyncArgs{})
	if len(got) != 0 {
		t.Errorf("got %d periodic jobs, want 0 — gmail_sync declares registers_nothing without GmailRegistry, and a row nothing can work must never be queued", len(got))
	}
}

// TestPeriodicForNeedsEveryFieldOfADeclaredConjunction covers the one kind
// whose registration names two fields. Gmail's push-watch maintenance needs
// the OAuth app AND a Pub/Sub topic, and the first alone is what the nested
// guard it replaces made easy to get wrong.
func TestPeriodicForNeedsEveryFieldOfADeclaredConjunction(t *testing.T) {
	registry := &capture.Registry{}
	watch := GmailWatchConfig{Topic: "projects/p/topics/t"}

	if got := periodicFor(JobRunnerConfig{GmailRegistry: registry}, GmailWatchArgs{}); len(got) != 0 {
		t.Errorf("with a registry but no topic: got %d periodic jobs, want 0 — every field of a conjunction has to be supplied", len(got))
	}
	if got := periodicFor(JobRunnerConfig{GmailWatch: watch}, GmailWatchArgs{}); len(got) != 0 {
		t.Errorf("with a topic but no registry: got %d periodic jobs, want 0 — every field of a conjunction has to be supplied", len(got))
	}
	if got := periodicFor(JobRunnerConfig{GmailRegistry: registry, GmailWatch: watch}, GmailWatchArgs{}); len(got) != 1 {
		t.Errorf("with both: got %d periodic jobs, want 1", len(got))
	}
}

// Every kind that declares off_at_zero places no schedule while its setting
// holds zero, and one once it holds a positive value. Both directions per
// kind, because the negative half alone cannot tell a kind that honours zero
// from one that never schedules at all. The subjects are the declarations.
func TestEveryOffAtZeroKindPlacesNoScheduleAtZero(t *testing.T) {
	declaring := 0
	for kind, spec := range jobs.Declared() {
		if !spec.Cadence.OffAtZero {
			continue
		}
		declaring++
		t.Run(kind, func(t *testing.T) {
			off := newScheduleBook(map[string]time.Duration{spec.Cadence.Setting: 0})
			if _, running := off.schedule(spec, nil, nil); running {
				t.Errorf("%s at zero still placed a schedule; an admin who switched it off is still swept", spec.Cadence.Setting)
			}
			on := newScheduleBook(map[string]time.Duration{spec.Cadence.Setting: time.Hour})
			if _, running := on.schedule(spec, nil, nil); !running {
				t.Errorf("%s at an hour placed no schedule, so the half above proves nothing", spec.Cadence.Setting)
			}
		})
	}
	if declaring == 0 {
		t.Fatal("no kind declares off_at_zero — the specs moved, or this test has stopped reading the field it derives its subjects from")
	}
}

// TestOffAtZeroMatchesWhatTheSettingAdmits holds each declaration to its
// setting: a cadence that may be switched off reads a setting that admits
// zero, and one that may not reads a setting that refuses it. Otherwise an
// admin could save a zero the schedule then runs as a busy loop, or be refused
// the off the contract promises.
func TestOffAtZeroMatchesWhatTheSettingAdmits(t *testing.T) {
	definitions := map[string]settings.Definition{}
	for _, def := range settingsDefinitions() {
		definitions[def.Key()] = def
	}
	for kind, spec := range jobs.Declared() {
		if spec.Cadence.Setting == "" {
			continue
		}
		def, registered := definitions[spec.Cadence.Setting]
		if !registered {
			t.Errorf("%s schedules from %s, which no module registers", kind, spec.Cadence.Setting)
			continue
		}
		admitsZero := def.ValidateJSON(json.RawMessage("0")) == nil
		if admitsZero != spec.Cadence.OffAtZero {
			t.Errorf("%s: off_at_zero is %t but %s admits zero: %t", kind, spec.Cadence.OffAtZero, spec.Cadence.Setting, admitsZero)
		}
	}
}

// TestEveryScheduleSettingDefaultsToWholeSeconds is what scheduleValue's panic
// leans on: every setting a cadence reads defaults to a positive whole number
// of seconds, so DefaultSchedules builds and nothing a default schedules runs
// in a loop.
func TestEveryScheduleSettingDefaultsToWholeSeconds(t *testing.T) {
	keys := scheduleSettings()
	if len(keys) == 0 {
		t.Fatal("no cadence reads a setting — this test has stopped reading the contract")
	}
	for _, key := range keys {
		raw, err := settingDefault(key)
		if err != nil {
			t.Fatal(err)
		}
		var seconds int
		if err := json.Unmarshal(raw, &seconds); err != nil || seconds <= 0 {
			t.Errorf("%s defaults to %s, want a positive whole number of seconds", key, raw)
		}
	}
}

// TestPeriodicForPlacesNoScheduleForAnOnDemandDispatcher covers the dispatcher
// a human's confirm enqueues. Its registration posture is registers_anyway, so
// nothing but the cadence can be the reason there is no entry.
func TestPeriodicForPlacesNoScheduleForAnOnDemandDispatcher(t *testing.T) {
	if got := periodicFor(JobRunnerConfig{}, EmbedReindexArgs{}); len(got) != 0 {
		t.Errorf("got %d periodic jobs, want 0 — embed_reindex declares cadence: on_demand and no clock ever enqueues it", len(got))
	}
}

// TestRegistersHonoursBothAbsentPostures pins the distinction the declaration
// exists to carry. The same missing field decides differently depending on the
// posture, so a resolver that collapsed the two would fail exactly one arm.
func TestRegistersHonoursBothAbsentPostures(t *testing.T) {
	nothing := jobs.Registration{When: []string{"Embedder"}}
	anyway := jobs.Registration{When: []string{"Embedder"}, AbsentRegistersAnyway: true}

	if registers(JobRunnerConfig{}, nothing) {
		t.Error("registers_nothing with the field absent = true, want false")
	}
	if !registers(JobRunnerConfig{}, anyway) {
		t.Error("registers_anyway with the field absent = false, want true — the worker stays so a picked-up row fails loudly instead of rotting queued")
	}
	if !registers(JobRunnerConfig{}, jobs.Registration{}) {
		t.Error("an empty registration = false, want true — a kind that names no dependency registers unconditionally")
	}
}

// assertTableAnswersExactly compares a lookup table's keys with the paths the
// declaration names. Both directions matter: an unanswered path panics the
// boot, and an answer no kind names is a dead entry nobody would notice.
func assertTableAnswersExactly(t *testing.T, table string, answered, declared []string) {
	t.Helper()
	if len(declared) == 0 {
		t.Fatalf("%s: the declaration names no field path at all — this gate would pass vacuously", table)
	}
	for _, path := range declared {
		if !slices.Contains(answered, path) {
			t.Errorf("%s does not answer JobRunnerConfig.%s, which api/jobs.yaml names — the boot would panic on it", table, path)
		}
	}
	for _, path := range answered {
		if !slices.Contains(declared, path) {
			t.Errorf("%s answers JobRunnerConfig.%s, which no declaration names — a dead entry", table, path)
		}
	}
}

// TestEveryDeclaredRegistrationFieldIsAnswered derives the obligation from the
// declaration rather than maintaining it as a list: configDependencies must
// answer exactly the JobRunnerConfig fields api/jobs.yaml gates a kind on.
func TestEveryDeclaredRegistrationFieldIsAnswered(t *testing.T) {
	assertTableAnswersExactly(t, "configDependencies",
		slices.Sorted(maps.Keys(configDependencies(JobRunnerConfig{}))),
		slices.Sorted(maps.Keys(gatedDependencyPaths())))
}

// scheduledDispatcherFloor guards against a vacuous pass below. Twenty-two
// dispatchers carry a clock today; the floor sits at eighteen so retiring a
// few passes does not drag the gate along, while a walker that matched
// nothing — or a rename of periodicFor — still trips it.
const scheduledDispatcherFloor = 18

// periodicSiteKinds reads every periodicFor call in this package's own sources
// and answers the kinds they schedule, counted so a duplicate site is visible.
// Read off the source rather than executed because NewJobRunner needs a live
// pool, and the claim here is about what is wired, not about what runs.
func periodicSiteKinds(t *testing.T) map[string]int {
	t.Helper()
	byType := kindByGoType()
	wired := map[string]int{}
	_, files := parseComposeSources(t)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "periodicFor" || len(call.Args) != 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.CompositeLit)
			if !ok {
				t.Errorf("a periodicFor site passes %T rather than an args literal, so this gate cannot read the kind it schedules", call.Args[1])
				return true
			}
			name, ok := lit.Type.(*ast.Ident)
			if !ok {
				t.Errorf("a periodicFor site passes a composite literal of %T rather than a named args type", lit.Type)
				return true
			}
			kind, declared := byType[name.Name]
			if !declared {
				t.Errorf("a periodicFor site names %s, which no declared kind is typed as", name.Name)
				return true
			}
			wired[kind]++
			return true
		})
	}
	return wired
}

// TestEveryScheduledKindIsWiredExactlyOnce derives the wiring obligation from
// the contract instead of keeping it as a list: a kind whose declaration
// carries a clock must have exactly one periodicFor site, and a site must not
// name a kind with no schedule to place. A pass that lost its site is otherwise
// silent — nothing fails, it simply never ticks — which is the failure mode of
// splitting twenty schedules across five files.
//
// The obligation follows the CADENCE, not the role. A dispatcher always carries
// one; a worker carries one when it is a pass in its own right rather than a
// dispatcher's child (ADR-0103 §1).
func TestEveryScheduledKindIsWiredExactlyOnce(t *testing.T) {
	wired := periodicSiteKinds(t)

	scheduled := 0
	for kind, spec := range jobs.Declared() {
		wantSite := declaresAClock(spec.Cadence) && !spec.Cadence.OnDemand
		if !wantSite {
			if wired[kind] > 0 {
				t.Errorf("%s has %d periodicFor site(s) but declares no schedule to place", kind, wired[kind])
			}
			continue
		}
		scheduled++
		if wired[kind] != 1 {
			t.Errorf("%s has %d periodicFor site(s), want exactly 1 — a declared cadence nothing wires never ticks", kind, wired[kind])
		}
	}
	if scheduled < scheduledDispatcherFloor {
		t.Fatalf("only %d dispatchers declare a cadence, under the floor of %d — this gate is no longer reading the contract",
			scheduled, scheduledDispatcherFloor)
	}
}

// declaresAClock reports whether the contract gives this kind a schedule of its
// own — a fixed interval, or a setting that carries one. A kind with neither
// is reached by its dispatcher, and has no periodicFor site to own.
func declaresAClock(c jobs.Cadence) bool {
	return c.Fixed != 0 || c.Setting != "" || c.OnDemand
}

// TestThePeriodicInsertYieldsToAnArgsOwnedCap is what makes periodicInsertOpts'
// claim about River's resolution order checkable.
//
// River reads the EXPLICIT InsertOpts before the args type's own, so the
// periodic insert leaving MaxAttempts at zero is the whole reason an
// opts_owner: args kind runs at the number api/jobs.yaml publishes for it.
// Supply one unconditionally and every declared cap is silently outranked —
// TestArgsOwnedAttemptCapsMatchTheirDeclaration would still pass, because it
// reads the args method rather than what the scheduler inserts with, and the
// drift would surface only as a pass retrying far more than the file says.
func TestThePeriodicInsertYieldsToAnArgsOwnedCap(t *testing.T) {
	t.Parallel()
	// AgentSchedulerArgs owns its InsertOpts and declares one attempt; the
	// periodic insert must add nothing, or the one becomes three.
	if got := periodicInsertOpts(AgentSchedulerArgs{}).MaxAttempts; got != 0 {
		t.Errorf("the periodic insert supplied MaxAttempts %d for a kind whose args own the cap: River reads "+
			"the explicit opts first, so this outranks the %d api/jobs.yaml publishes for agent_scheduler",
			got, specFor(t, AgentSchedulerArgs{}.Kind()).MaxAttempts)
	}
}

// TestThePeriodicInsertCapsAPassNobodyElseCaps is the other half: a scheduled
// kind whose args own no InsertOpts has no other place a cap could come from,
// and River's default of 25 is a ladder reaching days that nobody chose.
func TestThePeriodicInsertCapsAPassNobodyElseCaps(t *testing.T) {
	t.Parallel()
	// CloseDateSweepArgs is opts_owner: caller — api/jobs.yaml refuses it a
	// max_attempts, so this insert is the only door the number comes through.
	if _, owned := any(CloseDateSweepArgs{}).(river.JobArgsWithInsertOpts); owned {
		t.Fatal("CloseDateSweepArgs now owns its own InsertOpts, so it no longer stands for the caller-owned " +
			"passes this test is about — name one that does not")
	}
	if got := periodicInsertOpts(CloseDateSweepArgs{}).MaxAttempts; got != periodicPassMaxAttempts {
		t.Errorf("the periodic insert gave a caller-owned pass MaxAttempts %d, not periodicPassMaxAttempts "+
			"(%d): nothing else caps it, so anything but this leaves it on River's %d-rung default",
			got, periodicPassMaxAttempts, river.MaxAttemptsDefault)
	}
}

// The journal export needs an object store on both halves of its wiring.
//
// The worker is withheld by the composer and the schedule by the kind's own
// declaration.
//
// A store missing from one and present to the other would place a schedule
// nothing serves, or a worker nothing calls.
func TestTheSuppressionJournalNeedsAnObjectStoreForBothHalves(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.DiscardHandler)

	if got := periodicFor(JobRunnerConfig{}, SuppressionJournalArgs{}); len(got) != 0 {
		t.Errorf("with no object store: got %d periodic jobs, want 0", len(got))
	}
	if got := suppressionJournalExporterFor(nil, nil, log); got != nil {
		t.Error("with no object store: got a worker, want nil — there is nowhere to write the journal")
	}

	blob := blobstore.NewMemory()
	if got := periodicFor(JobRunnerConfig{Blobstore: blob}, SuppressionJournalArgs{}); len(got) != 1 {
		t.Errorf("with an object store: got %d periodic jobs, want 1", len(got))
	}
	if got := suppressionJournalExporterFor(nil, blob, log); got == nil {
		t.Error("with an object store: got no worker, want one")
	}
}
