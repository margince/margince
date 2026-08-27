// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The two questions this flag used to answer at once, held apart.
//
// Hashing is a property of the KEY: does source_id embed a person's provider
// account. Payload attribution is a property of the COUNTERPARTY: does the
// record name its human by an account rather than an address. A record can be
// either, both or neither, and the four combinations below are what a single
// field could not represent — which is how one connector ended up with its
// direct messages hashed and its mentions not, on identical key semantics.
func TestHashingFollowsTheKeyAndAttributionFollowsTheCounterparty(t *testing.T) {
	const notificationID = "dispact:88421"
	for name, tc := range map[string]struct {
		keyNamesAPerson bool
		channelProvider string
		wantHashed      bool
		wantAttributed  bool
	}{
		// The reported case, both halves. One connector, one key shape, two
		// ways of naming a counterparty — and the source id is a notification
		// id in both, so neither may be hashed.
		"a notification from a channel account": {
			channelProvider: "dispact",
			wantAttributed:  true,
		},
		"a notification from an address": {},
		// A chat message keyed by an account id: hashed, and the counterparty
		// is named by an account too.
		"a chat message": {
			keyNamesAPerson: true,
			channelProvider: "telegram",
			wantHashed:      true,
			wantAttributed:  true,
		},
		// The combination that proves the two are independent: a key that
		// names a person, on a record whose counterparty has an address.
		"an account-keyed record with an addressable sender": {
			keyNamesAPerson: true,
			wantHashed:      true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec := connector.NormalizedRecord{
				EntityType: datasource.EntityActivity,
				NaturalKey: connector.NaturalKey{
					SourceSystem:         "dispact",
					SourceID:             notificationID,
					SourceIDNamesAPerson: tc.keyNamesAPerson,
				},
			}
			rec.Counterparty.ChannelIdentity.Provider = tc.channelProvider
			if tc.channelProvider != "" {
				rec.Counterparty.ChannelIdentity.ChannelUserID = "u-1"
			}

			entry := (&Sink{}).traceEntry(context.Background(), rec,
				pipelinetrace.StageActivityWrite, TraceCaptured, "")

			stored := traceSourceID(entry.SourceID, entry.SourceIDNamesAPerson)
			hashed := strings.HasPrefix(stored, "sha256:")
			if hashed != tc.wantHashed {
				t.Errorf("the trace stores %q (hashed=%v), want hashed=%v — hashing is the KEY's "+
					"question, and this key %s", stored, hashed, tc.wantHashed,
					map[bool]string{true: "names a person", false: "names a notification"}[tc.keyNamesAPerson])
			}
			if !tc.wantHashed && stored != notificationID {
				t.Errorf("an unhashed trace must keep the provider's own id verbatim, got %q — "+
					"it is what makes a support question answerable (ADR-0082 §1)", stored)
			}
			if got := entry.namesItsHumanByAccount(); got != tc.wantAttributed {
				t.Errorf("namesItsHumanByAccount = %v, want %v — attribution is the COUNTERPARTY's "+
					"question, and this record names its human %s", got, tc.wantAttributed,
					map[bool]string{true: "by an account", false: "by an address"}[tc.channelProvider != ""])
			}
		})
	}
}

// The flag is carried, not re-derived. A trace whose hashing decision came from
// anywhere but the key it is hashing is the bug this replaced, and it would
// pass every case above as long as the two happened to agree.
func TestTheTraceHashesTheKeyItWasGiven(t *testing.T) {
	for _, namesAPerson := range []bool{false, true} {
		rec := connector.NormalizedRecord{
			EntityType: datasource.EntityActivity,
			NaturalKey: connector.NaturalKey{
				SourceSystem: "dispact", SourceID: "k-1", SourceIDNamesAPerson: namesAPerson,
			},
		}
		// The counterparty says the OPPOSITE of the key, every time: this is
		// the disagreement the old inference could not survive.
		if !namesAPerson {
			rec.Counterparty.ChannelIdentity.Provider = "dispact"
			rec.Counterparty.ChannelIdentity.ChannelUserID = "u-1"
		}
		entry := (&Sink{}).traceEntry(context.Background(), rec,
			pipelinetrace.StageActivityWrite, TraceCaptured, "")
		if entry.SourceIDNamesAPerson != namesAPerson {
			t.Errorf("the key said SourceIDNamesAPerson=%v and the trace entry says %v — the trace "+
				"is reading something other than the key", namesAPerson, entry.SourceIDNamesAPerson)
		}
	}
}
