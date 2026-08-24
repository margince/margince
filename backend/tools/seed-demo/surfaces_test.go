// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "testing"

// A project's key is minted by the server from its name (projects/keymint.go),
// and a caller that sends one is refused outright — "a project's key is assigned
// by the server from its name and cannot be set or changed", 422 read_only. The
// refusal is deliberate: the key is what a person types in a subject line to
// file mail under a project, so a caller-chosen one becomes a matcher nobody
// audited.
//
// This seeder sent one. The whole run died on the first project, after the
// companies, people and paper were already written, which is the shape of
// failure this test exists to keep out: the phases AFTER projects — quotas,
// consent, lifecycle, relationship types and the owner assignment that runs
// last — never happened at all.
func TestProjectCreateBodySendsNoKey(t *testing.T) {
	body := projectCreateBody("org-1", "Acme — Rollout", "2026-01-01")

	if _, sent := body["key"]; sent {
		t.Error("the create body carries a key; the server mints it and refuses a caller's own (422 read_only)")
	}
	if body["name"] != "Acme — Rollout" {
		t.Errorf("name = %v, want the name the key will be minted from", body["name"])
	}
	if body["organization_id"] != "org-1" {
		t.Errorf("organization_id = %v, want org-1", body["organization_id"])
	}
}

// Convergence has to survive the key going away with it. The seeder used to ask
// "does a project with MY key exist" — a question it can no longer ask, because
// it no longer knows the key. A second run that cannot recognise its own work
// creates every project twice.
func TestProjectIndexKeyDistinguishesNameAndOrganization(t *testing.T) {
	same := projectIndexKey("org-1", "Acme — Rollout")
	if same != projectIndexKey("org-1", "Acme — Rollout") {
		t.Error("the same project indexes differently on two calls, so a second run would duplicate it")
	}
	if same == projectIndexKey("org-2", "Acme — Rollout") {
		t.Error("two organizations' identically named projects share an index entry, so the second is skipped")
	}
	if same == projectIndexKey("org-1", "Acme — Einführung") {
		t.Error("two differently named projects in one organization share an index entry")
	}
}

// Two domains can resolve to ONE organization — an alias, or two dataset entries
// merged into the same company — and one organization gives one derived name. The
// old per-domain key could not collide, so a snapshot taken before the loop was
// enough; this index can, and a snapshot cannot see what the loop itself created.
func TestProjectIndexClaimsANameWithinTheSameRun(t *testing.T) {
	index := map[string]bool{}
	first := projectIndexKey("org-1", "Acme — Rollout")

	if index[first] {
		t.Fatal("an empty index reports a project as present")
	}
	index[first] = true

	if !index[projectIndexKey("org-1", "Acme — Rollout")] {
		t.Error("the second plan entry for one organization does not see the first entry's claim, so the project is created twice")
	}
}
