// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The child sets Put back judges against the live rows. Each set is projected to
// the fields a restore sends. An entry's image and the rows under it then
// compare as equal whichever writer made them.
const (
	ChildSetEmails  = auditKeyEmails
	ChildSetPhones  = auditKeyPhones
	ChildSetDomains = auditKeyDomains
)

// liveChildSetSQL names, per key, the rows of the set and the fields a restore
// sends for each. The statements are compile-time literals.
var liveChildSetSQL = map[string]string{
	ChildSetEmails: `SELECT jsonb_build_object('email', email, 'email_type', email_type, 'is_primary', is_primary, 'position', position)
		FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`,
	ChildSetPhones: `SELECT jsonb_build_object('phone', phone, 'phone_type', phone_type, 'is_primary', is_primary, 'position', position)
		FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`,
	ChildSetDomains: `SELECT jsonb_build_object('domain', domain, 'is_primary', is_primary)
		FROM company_domain WHERE company_id = $1 AND archived_at IS NULL`,
}

// childSetFields are the keys of a row that a restore sends.
//
//nolint:goconst // wire field names read as data; a shared constant would tie them to unrelated uses of the same word
var childSetFields = map[string][]string{
	ChildSetEmails:  {"email", "email_type", "is_primary", "position"},
	ChildSetPhones:  {"phone", "phone_type", "is_primary", "position"},
	ChildSetDomains: {"domain", "is_primary"},
}

// nullImage is an image that states no rows.
const nullImage = "null"

// IsChildSet reports whether key names a set Put back reads from the live rows.
func IsChildSet(key string) bool { _, ok := liveChildSetSQL[key]; return ok }

// LiveChildSet is the record's current rows for one set, in canonical form.
func LiveChildSet(ctx context.Context, tx pgx.Tx, key string, id ids.UUID) (json.RawMessage, error) {
	query, ok := liveChildSetSQL[key]
	if !ok {
		return nil, fmt.Errorf("contacts: %q is not a child set", key)
	}
	rows, err := tx.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the live %s: %w", key, err)
	}
	defer rows.Close()
	var live []json.RawMessage
	for rows.Next() {
		var row json.RawMessage
		if err := rows.Scan(&row); err != nil {
			return nil, err
		}
		live = append(live, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return canonicalChildSet(key, live)
}

// ChildSetImage is an audit image's value for one set, in the same canonical
// form as LiveChildSet. A null image is the empty set.
func ChildSetImage(key string, image json.RawMessage) (json.RawMessage, error) {
	var rows []json.RawMessage
	if len(image) > 0 && string(image) != nullImage {
		if err := json.Unmarshal(image, &rows); err != nil {
			return nil, fmt.Errorf("contacts: the %s image is not a list: %w", key, err)
		}
	}
	return canonicalChildSet(key, rows)
}

// canonicalChildSet keeps the restorable fields of each row and orders the rows,
// because a child table is read back in no promised order.
func canonicalChildSet(key string, rows []json.RawMessage) (json.RawMessage, error) {
	kept := make([]string, 0, len(rows))
	for _, raw := range rows {
		var row map[string]json.RawMessage
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, fmt.Errorf("contacts: a %s row is not an object: %w", key, err)
		}
		picked := make(map[string]json.RawMessage, len(childSetFields[key]))
		for _, field := range childSetFields[key] {
			picked[field] = row[field]
		}
		text, err := json.Marshal(picked)
		if err != nil {
			return nil, err
		}
		kept = append(kept, string(text))
	}
	slices.Sort(kept)
	out := make([]json.RawMessage, len(kept))
	for i, text := range kept {
		out[i] = json.RawMessage(text)
	}
	return json.Marshal(out)
}

// ChildSetsThatMoved judges the emails, phones and domains an image states
// against the record's live rows. It hands back the rest of the image for the
// judgement by the trail. A writer that never audited a set still counts, and a
// completed reversal leaves the rows equal to the older entry's image.
func ChildSetsThatMoved(ctx context.Context, tx pgx.Tx, id ids.UUID, image []byte) (moved []string, rest []byte, err error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(image, &keys); err != nil {
		return nil, nil, fmt.Errorf("contacts: the judged image is not a JSON object: %w", err)
	}
	for key, stated := range keys {
		if !IsChildSet(key) {
			continue
		}
		delete(keys, key)
		want, err := ChildSetImage(key, stated)
		if err != nil {
			moved = append(moved, key)
			continue
		}
		live, err := LiveChildSet(ctx, tx, key, id)
		if err != nil {
			return nil, nil, err
		}
		if string(live) != string(want) {
			moved = append(moved, key)
		}
	}
	sort.Strings(moved)
	rest, err = json.Marshal(keys)
	return moved, rest, err
}
