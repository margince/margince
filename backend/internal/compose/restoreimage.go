// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a restore may SEND, out of what the trail recorded.
//
// An audit image is a snapshot of columns; an update request is a shape the
// contract declares. They do not match, and the gap is where a reversal either
// becomes dishonest or becomes a refusal. This file is that translation and
// nothing else: which keys survive it, which fold, which travel as clears, and
// whether the entry recorded a change at all.
//
// The evaluator (undoability.go) decides what to do with the answers.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/shared/ports/datasource"
)

// derivedColumns never travel in a restore even when the image carries them.
// They are the write path's own output, not a person's decision, and replaying
// one would state a stamp nobody made.
var derivedColumns = map[string]bool{
	"updated_at": true, "created_at": true, "id": true, "version": true,
}

// addressColumns maps the address_* columns an audit image carries onto the
// keys of the structured `address` object the update shapes declare. The image
// is per COLUMN, because that is what the record holds; the request is one
// nested object, because that is how a person edits an address.
//
// Folding is what makes an address change reversible at all. Without it every
// key filters out as unspellable and the whole entry refuses — and an edit that
// touched an address would be permanently un-undoable, which is not a limit the
// product should have for the sake of a name mismatch.
var addressColumns = map[string]string{
	"address_line1":       "line1",
	"address_line2":       "line2",
	"address_city":        "city",
	"address_region":      "region",
	"address_postal_code": "postal_code",
	"address_country":     "country",
}

// addressField is the key the folded object travels under. Every update shape
// that accepts an address spells it this way.
const addressField = "address"

// objectFieldsClearedByAnEmptyObject are fields the record holds in their own
// table or document rather than in a column, and whose update shape takes a
// whole object. A null image means "there was none", and the way to say that is
// an EMPTY object: it is supplied, so the module replaces the contents with
// nothing, where a bare null would decode to "not supplied" and change nothing.
//
// This is the same reason the address folds. The difference is only that an
// address arrives as columns and these arrive already whole.
var objectFieldsClearedByAnEmptyObject = map[string]bool{
	"social": true,
}

// namedByTheShapeButNotWrittenByThePatch: keys a record type's update REQUEST
// declares that its update path does not write. The generated shape is the
// contract's, and the module's mapper is narrower than it — a key in the gap is
// accepted, ignored, and answers success, which is the silent-drop failure this
// whole refusal set exists to prevent.
//
// deals is the whole of it today. fx_rate_to_base and fx_rate_date are DERIVED
// from the amount and currency, so a restore that puts those two back re-derives
// them; replaying a stored rate would state a conversion nobody performed.
// status and lost_reason belong to the advance-and-close path, and a field patch
// is not how a deal's lifecycle moves.
//
// Held by TestARestoreLandsEveryFieldItSends
// (backend/internal/compose/recordrestoreshape_integration_test.go), which sets
// every field a record type's shape declares, restores, and reports any that did
// not land — so a key that joins the gap later is named rather than dropped.
var namedByTheShapeButNotWrittenByThePatch = map[string]map[string]bool{
	"deal": {
		"fx_rate_to_base": true, "fx_rate_date": true,
		"status": true, "lost_reason": true,
	},
}

// filterImage reduces a before-image to the patch a restore could send, and
// reports the keys it had to leave behind.
//
// Those two answers travel together because the second is not a detail. A
// person's update changed a title and an address; the address arrives in the
// image as address_line1…address_country and the update shape spells only a
// structured `address`, so a filter that quietly kept the title would put half
// the change back and report success. That is the dishonest success this whole
// refusal set exists to prevent, and it is worse than refusing, because the
// person reads the confirmation and stops looking.
//
// Derived columns and the keys a record type's shape names but its mapper never
// writes are dropped SILENTLY and on purpose: neither was a person's decision,
// and neither is missing from the restore in any sense a reader would care about.
func filterImage(entityType string, before json.RawMessage) (map[string]json.RawMessage, []string, error) {
	var image map[string]json.RawMessage
	if len(before) > 0 {
		if err := json.Unmarshal(before, &image); err != nil {
			return nil, nil, fmt.Errorf("compose: undoability: before-image is not a JSON object: %w", err)
		}
	}
	writable, served := agents.UpdatableFields(datasource.EntityType(entityType))
	if !served {
		return nil, nil, nil
	}
	allowed := make(map[string]bool, len(writable))
	for _, field := range writable {
		allowed[field] = true
	}
	patch := make(map[string]json.RawMessage, len(image))
	address := map[string]json.RawMessage{}
	var unspellable []string
	for key, value := range image {
		if derivedColumns[key] || namedByTheShapeButNotWrittenByThePatch[entityType][key] {
			continue
		}
		if nested, isAddress := addressColumns[key]; isAddress && allowed[addressField] {
			address[nested] = value
			continue
		}
		// A null on one of these is "there was none", which an empty object
		// says and a null cannot.
		if objectFieldsClearedByAnEmptyObject[key] && allowed[key] && string(value) == "null" {
			patch[key] = json.RawMessage(`{}`)
			continue
		}
		// A cf_* key is a custom field: the catalog decides whether it is still
		// writable, and that is a live-state question the value check owns. The
		// shape check admits it here so the value check can name it.
		if !allowed[key] && !strings.HasPrefix(key, "cf_") {
			unspellable = append(unspellable, key)
			continue
		}
		patch[key] = value
	}
	if len(address) > 0 {
		// The nested object is SUPPLIED, so the nulls inside it are values the
		// update path can write. That is the whole difference between folding
		// and refusing: a bare address_city null is indistinguishable from an
		// absent field, and one inside a supplied address is not.
		folded, err := json.Marshal(address)
		if err != nil {
			return nil, nil, fmt.Errorf("compose: fold the restored address: %w", err)
		}
		patch[addressField] = folded
	}
	sort.Strings(unspellable)
	return patch, unspellable, nil
}

// sortedFields spells a patch's keys for a refusal's detail, so the person hears
// which field stopped the restore rather than that something did.
func sortedFields(patch map[string]json.RawMessage) []string {
	out := make([]string, 0, len(patch))
	for key := range patch {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// splitNulls separates the patch into the values a restore SENDS and the fields
// it must ask to be CLEARED, and names the ones this record type cannot clear
// at all.
//
// A null cannot travel in the patch. Every field on every update request is an
// optional pointer, so a JSON null decodes to nil and the module reads it as
// "not supplied" — the write succeeds and the field keeps its value. Cleared
// fields therefore travel beside the patch, and a field the record type cannot
// clear is refused rather than sent and silently dropped.
func splitNulls(entityType string, patch map[string]json.RawMessage) (map[string]json.RawMessage, []string, []string) {
	values := make(map[string]json.RawMessage, len(patch))
	var clear, unclearable []string
	for key, value := range patch {
		if string(value) != "null" {
			values[key] = value
			continue
		}
		if canClear(entityType, key) {
			clear = append(clear, key)
			continue
		}
		unclearable = append(unclearable, key)
	}
	sort.Strings(clear)
	sort.Strings(unclearable)
	return values, clear, unclearable
}

// recordsAChange reports whether the entry's images differ on any key. A store
// that assigns a column the value it already holds records before and after as
// the same, and replaying that is a write nobody would notice.
func recordsAChange(before, after json.RawMessage) (bool, error) {
	var was, now map[string]json.RawMessage
	if len(before) > 0 {
		if err := json.Unmarshal(before, &was); err != nil {
			return false, fmt.Errorf("compose: before-image is not a JSON object: %w", err)
		}
	}
	if len(after) > 0 {
		if err := json.Unmarshal(after, &now); err != nil {
			return false, fmt.Errorf("compose: after-image is not a JSON object: %w", err)
		}
	}
	for key, value := range now {
		if derivedColumns[key] {
			continue
		}
		if string(was[key]) != string(value) {
			return true, nil
		}
	}
	return false, nil
}
