// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import "testing"

func TestAnActorIsNamedByTheOwnerThisPageNamesItWith(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, actorType, actorID, member, key string
	}{
		{"a job", "system", "system:auto_apply", "", "magic.by.auto_apply"},
		{"an unknown agent", "agent", "agent:test", "", "magic.by.agent"},
		{"a named member", "human", "human:x", "Dana Weiss", byMember},
		{"a member the reader cannot name", "human", "human:x", "", byColleague},
	} {
		t.Run(test.name, func(t *testing.T) {
			label := ActorLabel(test.actorType, test.actorID, test.member)
			if label.Key != test.key {
				t.Fatalf("labelled %q, want %q", label.Key, test.key)
			}
			if test.key == byMember && (label.Values == nil || (*label.Values)["name"] != test.member) {
				t.Fatalf("a named member carries %+v", label.Values)
			}
		})
	}
}
