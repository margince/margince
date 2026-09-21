// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The registration posture, held to the wiring.
//
// jobcensus.go measures one MAXIMALLY-configured role, which is the only way to
// see the contract's full extent — and is exactly why it cannot see this. Every
// kind is wired there, so the question api/jobs.yaml answers per kind ("what
// happens when this dependency is absent?") never comes up.
//
// It is a question with two answers that look identical from inside a wiring
// function: registers nothing, so a row nobody could work is never queued; or
// registers anyway, so a row that IS queued reaches a worker that can say what
// is missing instead of being refused at insert with a message about River's
// worker bundle. jobschedule.go's registers() reads the declared answer for the
// SCHEDULE half. Nothing read it for the worker half, and the guards that stand
// in for it are hand-written per group — which is how two kinds came to take a
// posture their own declaration contradicts.
//
// So this withholds one declared dependency at a time, rebuilds the wiring, and
// holds what got registered to what registers() says should have.

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// gatedDependencyFloor guards against a vacuous pass. Twenty-five declared
// paths gate kinds today; the floor sits low enough that retiring a few does
// not drag it along, and high enough that a declaration answering none — which
// would make the walk below iterate zero times — is reported rather than read
// as a clean census.
const gatedDependencyFloor = 20

// everyDeclaredPostureIsHonoured withholds each declared dependency in turn and
// reports every kind whose registration then disagrees with its declaration.
func everyDeclaredPostureIsHonoured() []string {
	full := censusJobConfig()
	paths := slices.Sorted(maps.Keys(gatedDependencyPaths()))
	var findings []string
	for _, path := range paths {
		withheld, known := withholdDependency(full, path)
		if !known {
			findings = append(findings, "api/jobs.yaml gates a kind on JobRunnerConfig."+path+
				", which withholdDependency does not know how to withhold — add it there, or this check stops asking about that dependency and reports nothing either way")
			continue
		}
		reg, _ := wireJobs(nil, slog.New(slog.DiscardHandler), withheld)
		findings = append(findings, posturesDisagreeingWithout(withheld, reg, path)...)
	}
	if len(paths) < gatedDependencyFloor {
		findings = append(findings, fmt.Sprintf(
			"only %d JobRunnerConfig paths gate a declared kind, expected at least %d — the walk above asked about almost nothing and would read clean whatever the wiring did", len(paths), gatedDependencyFloor))
	}
	return findings
}

// gatedDependencyPaths collects the JobRunnerConfig field paths that gate a
// kind, read off the declaration rather than listed beside it.
func gatedDependencyPaths() map[string]struct{} {
	paths := map[string]struct{}{}
	for _, spec := range jobs.Declared() {
		for _, path := range spec.Registration.When {
			paths[path] = struct{}{}
		}
	}
	return paths
}

// posturesDisagreeingWithout reports each kind registered against its
// declaration under a configuration missing one dependency.
func posturesDisagreeingWithout(cfg JobRunnerConfig, reg *jobRegistry, withheld string) []string {
	var findings []string
	for kind, spec := range jobs.Declared() {
		_, wired := reg.wired[kind]
		switch declared := registers(cfg, spec.Registration); {
		case declared && !wired:
			findings = append(findings, kind+
				" is unregistered with JobRunnerConfig."+withheld+" withheld, which its declaration does not allow — a row queued against such a role is refused at insert with a message about River's worker bundle, and neither a rep who pressed the button nor the log they land in learns what is actually missing. Either the guard demands a dependency the declaration does not name, or the kind declares `absent: registers_anyway` and the guard returns before registering it")
		case !declared && wired:
			findings = append(findings, kind+
				" is registered with JobRunnerConfig."+withheld+" withheld, which its declaration calls absent — the schedule half reads the declaration and places no tick, so this worker waits for rows the same build will never enqueue. Either drop the guard's extra reach, or declare `absent: registers_anyway` and make the worker say what is missing")
		}
	}
	// Sorted because Declared() is a map iteration and the reader is comparing
	// one run's findings against the last.
	slices.Sort(findings)
	return findings
}

// withholdDependency answers the configuration with ONE declared dependency
// taken away, and whether it knew how. An unknown path is reported rather than
// skipped, which is what keeps the walk above from failing short.
func withholdDependency(cfg JobRunnerConfig, path string) (JobRunnerConfig, bool) {
	take, known := dependencyWithholders()[path]
	if !known {
		return cfg, false
	}
	take(&cfg)
	return cfg, true
}

// dependencyWithholders is the inverse of configDependencies, keyed by the same
// field paths and hand-written for the same reason: a path is not always a
// field — GmailRegistry.OffersGraph is a registry that HOLDS a connector — so
// renaming one below fails the build here rather than quietly withholding
// nothing. The other direction, a declared path with no entry, is a fitness
// test.
func dependencyWithholders() map[string]func(*JobRunnerConfig) {
	return map[string]func(*JobRunnerConfig){
		"AccountScanBrain":       func(c *JobRunnerConfig) { c.AccountScanBrain = nil },
		"AgentScheduler.Service": func(c *JobRunnerConfig) { c.AgentScheduler.Service = nil },
		"Blobstore":              func(c *JobRunnerConfig) { c.Blobstore = nil },
		"ChannelVault":           func(c *JobRunnerConfig) { c.ChannelVault = nil },
		"ClassifyBrain":          func(c *JobRunnerConfig) { c.ClassifyBrain = nil },
		"DeepReadBrain":          func(c *JobRunnerConfig) { c.DeepReadBrain = nil },
		"DocumentExtractBrain":   func(c *JobRunnerConfig) { c.DocumentExtractBrain = nil },
		"Embedder":               func(c *JobRunnerConfig) { c.Embedder = nil },
		"EnrichBrain":            func(c *JobRunnerConfig) { c.EnrichBrain = nil },
		"Geocoder":               func(c *JobRunnerConfig) { c.Geocoder = nil },
		"GmailRegistry":          func(c *JobRunnerConfig) { c.GmailRegistry = nil },
		// The registry still there and the connector gone, because that is the
		// deployment this path describes: Gmail credentials configured and
		// Microsoft ones not.
		"GmailRegistry.OffersGraph":  func(c *JobRunnerConfig) { c.GmailRegistry = capture.NewRegistry(nil, nil, nil, nil) },
		"GmailWatch.Topic":           func(c *JobRunnerConfig) { c.GmailWatch.Topic = "" },
		"GraphWatch.NotificationURL": func(c *JobRunnerConfig) { c.GraphWatch.NotificationURL = "" },
		"OwedBrain":                  func(c *JobRunnerConfig) { c.OwedBrain = nil },
		"ProviderRuns.Registry":      func(c *JobRunnerConfig) { c.ProviderRuns.Registry = nil },
		"ProviderRuns.Vault":         func(c *JobRunnerConfig) { c.ProviderRuns.Vault = nil },
		"SendDelivery":               func(c *JobRunnerConfig) { c.SendDelivery = nil },
		"SendRegistry":               func(c *JobRunnerConfig) { c.SendRegistry = nil },
		"StageEvidenceBrain":         func(c *JobRunnerConfig) { c.StageEvidenceBrain = nil },
		"TechnicalEnricher":          func(c *JobRunnerConfig) { c.TechnicalEnricher = nil },
		"TranscriptProposeBrain":     func(c *JobRunnerConfig) { c.TranscriptProposeBrain = nil },
		"VatChecker":                 func(c *JobRunnerConfig) { c.VatChecker = nil },
		"VoiceBrain":                 func(c *JobRunnerConfig) { c.VoiceBrain = nil },
		"WebhookRetry.Deliverer":     func(c *JobRunnerConfig) { c.WebhookRetry.Deliverer = nil },
	}
}
