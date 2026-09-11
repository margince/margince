// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// What a message carried is written by one module and read back by another, so
// the two spellings of the snapshot must stay one spelling.
//
// comms stages the set into comms_outbound.attachments as JSON; activities
// reads that column back to tell the timeline what a message went out with.
// Nothing in Go connects the two — compose converts field by field on the way
// in (commsFiles) and the way out is a JSON unmarshal, which reports no error
// for a key it does not recognise. Rename a key on one side and the timeline
// quietly says every message carried nothing, which is exactly the state this
// arc was opened to fix.
//
// So both directions fail here: a field on one side and not the other, and a
// field whose stored key differs. The write side is the authority on the key —
// it is the one the column already holds.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
)

// TestTheStagedFileSnapshotIsReadBackInTheSpellingItWasWrittenIn holds
// activities.OutboundFile against comms.OutboundFile, which is the shape of the
// column itself.
func TestTheStagedFileSnapshotIsReadBackInTheSpellingItWasWrittenIn(t *testing.T) {
	written := storedKeys(reflect.TypeOf(comms.OutboundFile{}))
	read := storedKeys(reflect.TypeOf(activities.OutboundFile{}))

	for field, key := range written {
		switch readKey, present := read[field]; {
		case !present:
			t.Errorf("comms.OutboundFile stages %s and activities.OutboundFile has no such field, "+
				"so the timeline cannot report it", field)
		case readKey != key:
			t.Errorf("%s is stored under %q and read back from %q — the timeline reads a key "+
				"the column does not hold", field, key, readKey)
		}
	}
	for field := range read {
		if _, present := written[field]; !present {
			t.Errorf("activities.OutboundFile reads %s out of the snapshot and nothing stages it, "+
				"so the timeline reports a field that is always absent", field)
		}
	}
}

// storedKeys answers each exported field's JSON key, which is what the column
// is keyed by. A field with no tag is keyed by its own name, as encoding/json
// does it — recorded rather than skipped, because an untagged field on one
// side is precisely how the two drift.
func storedKeys(t reflect.Type) map[string]string {
	out := map[string]string{}
	for field := range t.NumField() {
		f := t.Field(field)
		if !f.IsExported() {
			continue
		}
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if key == "" {
			key = f.Name
		}
		out[f.Name] = key
	}
	return out
}
