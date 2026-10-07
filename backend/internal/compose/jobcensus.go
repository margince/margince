// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The job census: api/jobs.yaml held to the wiring, in both directions.
//
// Every other gate on this contract holds one END of it. The generated union
// stops an undeclared kind compiling; MustBeTotal refuses a boot that got one
// in anyway; Govern makes the declared timeout the one River applies. None of
// them can see the other direction — a kind declared and never wired, a
// {derived: …} timeout whose Go constant moved out from under it, an args
// field somebody added without declaring what it carries. Those are the drifts
// a declaration nobody checks accumulates, and this is where both halves are
// laid beside each other.
//
// It builds a REAL runner assembly to do it, minus the River client, because
// which kinds are wired is deployment-dependent. The configuration it wires
// through, and the guard that keeps that configuration from falling behind the
// declaration, are jobcensusconfig.go's — this file is what the assembly is
// then held to.

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// declaredJobKindFloor guards against a vacuous pass. The contract declares well
// above it; the floor sits low enough that retiring a few passes does not
// drag it along, and high enough that a Declared() answering nothing — which
// would make every check below iterate zero times — is reported rather than
// read as a clean census.
const declaredJobKindFloor = 45

// JobCensus is one fully-wired job assembly, kept so the declaration can be
// held to it. It holds no database handle and works nothing: NewJobRunner
// builds the same wiring with a River client on the end, and this is that
// wiring without one.
type JobCensus struct {
	wired map[string]wiredWorker
}

// NewJobCensus assembles every worker a maximally-configured role would run.
//
// The error is the guard against a census that is quietly measuring less than
// it claims: the configuration below has to supply every dependency the
// declaration names, or a kind gated on the one it missed would be reported as
// unwired when it is merely unconfigured here.
func NewJobCensus() (*JobCensus, error) {
	cfg := censusJobConfig()
	if err := everyDeclaredDependencySupplied(cfg); err != nil {
		return nil, err
	}
	// The periodic entries are the schedule half, which jobschedule_test.go
	// owns kind by kind; the census is about what is REGISTERED.
	reg, _ := wireJobs(nil, slog.New(slog.DiscardHandler), cfg)
	return &JobCensus{wired: reg.wired}, nil
}

// Validate reports every disagreement between the contract and the wiring at
// once. One message per finding, each naming the kind, because the reader is
// someone who has just changed one end and needs to know which.
func (c *JobCensus) Validate() error {
	findings := slices.Concat(
		bothGeneratedHalvesCameFromOneContract(),
		c.everyDeclaredKindIsWiredAndBack(),
		c.everyKindIsWorkedByItsDeclaredArgsType(),
		c.everyDerivedTimeoutStillEqualsItsConstant(),
		c.everyArgsFieldIsDeclaredAndBack(),
		c.everyFanOutChildCarriesItsUnitKey(),
		c.everyArgsOwnedKindCarriesItsOwnInsertOpts(),
		c.noArgsTypeAnswersToASecondKind(),
		c.everyDeclaredQueueIsBuiltWithItsDeclaredBound(),
		everyDeclaredPostureIsHonoured(),
	)
	if len(findings) == 0 {
		return nil
	}
	return errors.New("job census: api/jobs.yaml and the wiring disagree:\n  " + strings.Join(findings, "\n  "))
}

// NilAfterLoggingWaivers is every declared fault waiver, keyed by the WORKER
// TYPE that carries it rather than by the kind that declares it.
//
// The join is the point. api/jobs.yaml is keyed by kind, and the gate that
// reads worker bodies off the source knows only the receiver a Work method
// hangs off; the registration is the one place those two meet. Deriving the
// mapping here is what stops it being a second list beside the first.
func (c *JobCensus) NilAfterLoggingWaivers() map[string]string {
	waivers := map[string]string{}
	for kind, spec := range jobs.Declared() {
		entry, wired := c.wired[kind]
		if !wired || spec.Fault.NilAfterLogging == "" {
			continue
		}
		waivers[goTypeName(reflect.TypeOf(entry.worker))] = spec.Fault.NilAfterLogging
	}
	return waivers
}

// bothGeneratedHalvesCameFromOneContract catches a half-regenerated pair. One
// run of gen-jobs writes both tables, so the fingerprint each carries is the
// same one — unless somebody regenerated with one output path and committed
// only that half. The compiler believes the result either way: a Spec table
// and a type union describing two different revisions of the file both
// type-check, and every check below would then be comparing one revision's
// declaration against another's wiring.
//
// It says nothing about api/jobs.yaml as it stands on disk. A pair regenerated
// TOGETHER from a stale contract agrees with itself and passes here; `make
// drift` is what compares the tables against the file, and this arm is what
// makes the census's own reading coherent before it starts.
func bothGeneratedHalvesCameFromOneContract() []string {
	if jobs.JobContractHash == jobContractHash {
		return nil
	}
	return []string{fmt.Sprintf(
		"the generated halves came from different revisions of api/jobs.yaml (platform/jobs carries %s, compose carries %s) — run `make gen`",
		jobs.JobContractHash, jobContractHash)}
}

// everyDeclaredKindIsWiredAndBack is the totality both ways. A declared kind
// nothing registers is a pass that silently never runs — periodicFor may still
// place its tick, and the rows would sit available with no worker to claim
// them. A registered kind nothing declares cannot reach here (the generated
// union refuses it at compile time and MustBeTotal at boot), and is checked
// anyway because both those gates read the same generated file: a hand edit
// that widened the union would be believed by the compiler.
func (c *JobCensus) everyDeclaredKindIsWiredAndBack() []string {
	var findings []string
	declared := 0
	for kind := range jobs.Declared() {
		declared++
		if _, wired := c.wired[kind]; !wired {
			findings = append(findings,
				kind+" is declared but this build registers no worker for it — a role that ticks its dispatcher would queue rows nothing can claim; wire it, or retire the declaration")
		}
	}
	if declared < declaredJobKindFloor {
		findings = append(findings, fmt.Sprintf(
			"the contract declares only %d kinds, expected at least %d — every check below iterates the declaration, so a census over an empty one reports nothing and reads clean", declared, declaredJobKindFloor))
	}
	for _, kind := range slices.Sorted(maps.Keys(c.wired)) {
		if _, ok := jobs.SpecFor(kind); !ok {
			findings = append(findings,
				kind+" is registered but api/jobs.yaml declares no such kind — it would run at River's silent one-minute default; declare it and run `make gen`")
		}
	}
	return findings
}

// everyKindIsWorkedByItsDeclaredArgsType is the build-time half of the pairing
// the runner refuses to boot without. The finding is the same one and the
// wording is misfiledKinds'; what this arm adds is the floor, because a walk
// that inspected nothing would report nothing and read exactly like a fleet
// whose every kind is filed correctly.
func (c *JobCensus) everyKindIsWorkedByItsDeclaredArgsType() []string {
	findings := misfiledKinds(c.wired)
	if len(c.wired) < declaredJobKindFloor {
		findings = append(findings, fmt.Sprintf(
			"only %d registrations were paired against their declared go_type, expected at least %d — the census wired almost nothing and this check would pass by asking nobody", len(c.wired), declaredJobKindFloor))
	}
	return findings
}

// derivedTimeoutConstants is the join the compiler cannot make on its own: Go
// has no reflection over a constant's NAME, so a {derived: …} declaration can
// only be tied to the constant it names by a table somebody writes. This is
// that table, and it is the last hand-written list in this contract's chain —
// which is why the check below refuses both a name it cannot resolve and an
// entry no declaration uses. Neither can rot unnoticed.
//
// Every constant here is READ by something other than this table. A duration
// nothing else consumes belongs in api/jobs.yaml as a literal: derived from a
// constant nobody reads, the check would compare the declaration against a
// private copy of itself, and the number would live in two places to no end.
func derivedTimeoutConstants() map[string]time.Duration {
	return map[string]time.Duration{
		"agentSchedulerPassTimeout":   agentSchedulerPassTimeout,
		"deepReadTimeout":             deepReadTimeout,
		"privacyRetentionPassTimeout": privacyRetentionPassTimeout,
		"telegramPollJobTimeout":      telegramPollJobTimeout,
		"voiceBuildTimeout":           voiceBuildTimeout,
		"webhookRetrySweepTimeout":    webhookRetrySweepTimeout,
	}
}

// everyDerivedTimeoutStillEqualsItsConstant keeps a transcribed duration tied
// to the arithmetic it was transcribed from. Four of the six are expressions
// over another module's own limit (privacy.MaxPassDuration and friends), which
// moves when that module's batch bounds do; the other two are spent by code
// that has to agree with the wall clock (a reclaim grace, a long-poll budget).
// The declaration restates the result in every case, and a restatement nothing
// compares is a copy waiting to go stale.
func (c *JobCensus) everyDerivedTimeoutStillEqualsItsConstant() []string {
	constants := derivedTimeoutConstants()
	resolved := map[string]bool{}
	var findings []string
	for kind, spec := range jobs.Declared() {
		name := spec.Timeout.DerivedFrom
		if name == "" {
			continue
		}
		value, known := constants[name]
		if !known {
			findings = append(findings, fmt.Sprintf(
				"%s declares a timeout derived from %s, which derivedTimeoutConstants does not resolve — add it there, or the declaration tracks nothing", kind, name))
			continue
		}
		resolved[name] = true
		if value != spec.Timeout.Fixed {
			findings = append(findings, fmt.Sprintf(
				"%s declares %v derived from %s, which is now %v — River is handed the declared value, so update api/jobs.yaml and run `make gen`", kind, spec.Timeout.Fixed, name, value))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(constants)) {
		if !resolved[name] {
			findings = append(findings,
				name+" is resolved by derivedTimeoutConstants but no declaration derives from it — a dead entry; delete it")
		}
	}
	return findings
}

// everyArgsOwnedKindCarriesItsOwnInsertOpts holds the half of opts_owner: args
// that supplying the queue cannot.
//
// The queue itself is no longer compared: jobs.QueuedAs stamps it from the
// declaration at every insert, so the two can no longer disagree and an arm
// comparing them would pass whatever anyone wrote. What is still worth
// refusing is a kind that declares its args own the options and carries none —
// nothing then owns its uniqueness or its attempt cap, and River takes its own
// defaults for both.
func (c *JobCensus) everyArgsOwnedKindCarriesItsOwnInsertOpts() []string {
	var findings []string
	checked := 0
	for kind, spec := range jobs.Declared() {
		if spec.OptsOwner != jobs.OptsArgs {
			continue
		}
		entry, wired := c.wired[kind]
		if !wired {
			continue // already reported by the totality check.
		}
		if _, owns := entry.args.(river.JobArgsWithInsertOpts); !owns {
			findings = append(findings, fmt.Sprintf(
				"%s declares opts_owner: args but %s has no InsertOpts() — nothing then owns its uniqueness or its attempt cap, and River takes its own defaults", kind, spec.GoType))
			continue
		}
		checked++
	}
	if checked == 0 {
		findings = append(findings,
			"no opts_owner: args kind was checked — telegram_poll is the one kind this check exists for, and it matched nothing")
	}
	return findings
}

// noArgsTypeAnswersToASecondKind closes River's rename door. AddWorker
// registers a work unit under Kind() AND under every value KindAliases()
// returns, so a type that grew an alias would satisfy the closed union, pass
// Govern, compile — and have River working rows under a kind api/jobs.yaml
// never named. The contract cannot declare that alias either: go_type is
// unique per kind, so one args struct is one kind here by construction.
//
// The registration records the aliases too, which is what makes MustBeTotal
// refuse such a boot; this arm is the same finding at build time, where the
// rename is being written rather than deployed.
func (c *JobCensus) noArgsTypeAnswersToASecondKind() []string {
	var findings []string
	inspected := 0
	for _, kind := range slices.Sorted(maps.Keys(c.wired)) {
		inspected++
		aliased, answers := c.wired[kind].args.(river.JobArgsWithKindAliases)
		if !answers {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"%s also answers to %s through KindAliases — River registers a worker under every one of them, and api/jobs.yaml cannot declare a second kind for one args struct; rename by adding the new kind with its own args type and draining the old one",
			kind, strings.Join(aliased.KindAliases(), ", ")))
	}
	if inspected < declaredJobKindFloor {
		findings = append(findings, fmt.Sprintf(
			"only %d registered args types were inspected for kind aliases, expected at least %d — the walk matched almost nothing and this check would pass by asking nobody", inspected, declaredJobKindFloor))
	}
	return findings
}
