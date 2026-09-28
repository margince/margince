// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLogicalCallAppendKeepsExactlyOneTerminal proves the invariant every
// flush relies on: after any number of appends, exactly the LAST one
// appended carries IsTerminal — a later attempt always supersedes an
// earlier one, never the reverse.
func TestLogicalCallAppendKeepsExactlyOneTerminal(t *testing.T) {
	lc := newLogicalCall()
	lc.append(Call{Tier: TierPremium, ErrorSentinel: "provider_error"})
	lc.append(Call{Tier: TierCheapCloud, ErrorSentinel: "provider_error"})
	lc.append(Call{Tier: TierLocalSmall})

	terminalCount := 0
	for i, c := range lc.attempts {
		if c.Attempt != i+1 {
			t.Fatalf("attempt %d has Attempt=%d, want %d", i, c.Attempt, i+1)
		}
		if c.LogicalCallID != lc.id {
			t.Fatalf("attempt %d does not carry the logical call's id", i)
		}
		if c.IsTerminal {
			terminalCount++
		}
	}
	if terminalCount != 1 {
		t.Fatalf("want exactly 1 terminal attempt, got %d", terminalCount)
	}
	if !lc.attempts[2].IsTerminal {
		t.Fatal("the last attempt appended must be the terminal one")
	}
	if lc.terminal().Tier != TierLocalSmall {
		t.Fatalf("terminal() = %+v, want the last-appended attempt", lc.terminal())
	}
}

// TestComputeConfigHashIsDeterministicAndSensitiveToEveryField: the same
// four inputs always digest to the same hash (EnsureConfig's ON CONFLICT
// DO NOTHING depends on this to collapse repeats onto one row), and
// changing any ONE input must change the hash — two config snapshots that
// differ only in prompt version, say, must not collide onto the same
// dimension row.
func TestComputeConfigHashIsDeterministicAndSensitiveToEveryField(t *testing.T) {
	base := computeConfigHash("task-hash", "routing-hash", "v1", json.RawMessage(`{}`))
	again := computeConfigHash("task-hash", "routing-hash", "v1", json.RawMessage(`{}`))
	if base != again {
		t.Fatalf("identical inputs produced different hashes: %q vs %q", base, again)
	}
	variants := []string{
		computeConfigHash("other-task-hash", "routing-hash", "v1", json.RawMessage(`{}`)),
		computeConfigHash("task-hash", "other-routing-hash", "v1", json.RawMessage(`{}`)),
		computeConfigHash("task-hash", "routing-hash", "v2", json.RawMessage(`{}`)),
		computeConfigHash("task-hash", "routing-hash", "v1", json.RawMessage(`{"k":"v"}`)),
	}
	for i, v := range variants {
		if v == base {
			t.Fatalf("variant %d collided with the base hash — a changed field must change the digest", i)
		}
	}
}

// TestNewConfigSnapshotUsesTheGeneratedTaskContractHash pins the task-
// contract half of the spec §4 config key to the generated constant
// (tasks_gen.go) rather than a hand-maintained copy that could drift from
// the contract it is meant to fingerprint.
func TestNewConfigSnapshotUsesTheGeneratedTaskContractHash(t *testing.T) {
	snap := newConfigSnapshot("routing-hash", 1024)
	if snap.TaskContractHash != TaskContractHash {
		t.Fatalf("TaskContractHash = %q, want the generated constant %q", snap.TaskContractHash, TaskContractHash)
	}
	if snap.RoutingConfigHash != "routing-hash" {
		t.Fatalf("RoutingConfigHash = %q, want the passed-in digest", snap.RoutingConfigHash)
	}
	if snap.Hash == "" {
		t.Fatal("newConfigSnapshot must compute Hash, not leave it zero")
	}
	if snap.Hash != computeConfigHash(snap.TaskContractHash, snap.RoutingConfigHash, snap.PromptVersion, snap.ProviderParams) {
		t.Fatal("Hash does not match computeConfigHash over the snapshot's own fields")
	}
}

// TestNewConfigSnapshotCarriesTheConfiguredEmbedDimensionAndChangesHash
// proves the config snapshot's provider_params names the configured embed
// width (Task 5), and — since ProviderParams feeds computeConfigHash — that
// two snapshots differing only in that width land on two DIFFERENT
// ai_call_config rows rather than silently colliding onto one.
func TestNewConfigSnapshotCarriesTheConfiguredEmbedDimensionAndChangesHash(t *testing.T) {
	snap := newConfigSnapshot("routing-hash", 768)
	if string(snap.ProviderParams) != `{"embed_dimensions":768}` {
		t.Fatalf("ProviderParams = %s, want {\"embed_dimensions\":768}", snap.ProviderParams)
	}
	other := newConfigSnapshot("routing-hash", 1536)
	if other.Hash == snap.Hash {
		t.Fatal("two snapshots with different configured embed dimensions must not collide onto the same Hash")
	}
}

// The wire leaves attempt_reason a free string, so its description is the
// only place a client learns the vocabulary; every reason a row can carry
// must be named there.
func TestTheAttemptReasonDescriptionNamesEveryReason(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Components struct {
			Schemas struct {
				Attempt struct {
					Properties struct {
						Reason struct {
							Description string `yaml:"description"`
						} `yaml:"attempt_reason"`
					} `yaml:"properties"`
				} `yaml:"AiCallAttempt"` //nolint:tagliatelle // a schema name in crm.yaml, PascalCase as the contract spells it
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	description := contract.Components.Schemas.Attempt.Properties.Reason.Description
	if description == "" || len(attemptReasons) == 0 {
		t.Fatalf("nothing to compare: description %q, %d reasons", description, len(attemptReasons))
	}
	for _, reason := range attemptReasons {
		if !strings.Contains(description, reason) {
			t.Errorf("AiCallAttempt.attempt_reason does not name %q", reason)
		}
	}
}
