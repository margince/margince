// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// MailSeatIdentitiesSQL binds private identities for matching readable mail only.
// Callers receive an expression, never aliases to return as another seat's profile.
func MailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	identities, err := UnambiguousMailSeatIdentities(ctx, tx)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(identities)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d::jsonb", bind(encoded)), nil
}

type mailSeatSnapshotKey struct{}

type mailSeatSnapshot struct {
	tx         pgx.Tx
	actorID    string
	identities map[string][]string
}

// WithMailSeatIdentitySnapshot shares one identity read inside this caller's read transaction.
func WithMailSeatIdentitySnapshot(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	identities, err := UnambiguousMailSeatIdentities(ctx, tx)
	if err != nil {
		return ctx, err
	}
	actor, _ := principal.Actor(ctx)
	return context.WithValue(ctx, mailSeatSnapshotKey{}, mailSeatSnapshot{tx: tx, actorID: actor.ID, identities: identities}), nil
}

// PreparedMailSeatIdentitiesSQL refuses sorting projections without their prepared read snapshot.
func PreparedMailSeatIdentitiesSQL(ctx context.Context, bind func(any) int) (string, error) {
	snapshot, ok := ctx.Value(mailSeatSnapshotKey{}).(mailSeatSnapshot)
	actor, _ := principal.Actor(ctx)
	if !ok || snapshot.actorID != actor.ID {
		return "", errors.New("mail identities were not loaded for this reader's transaction")
	}
	return SQLf("$%d::jsonb", bind(snapshot.identities)), nil
}

// UnambiguousMailSeatIdentities gives proven holders precedence over self-declared aliases.
func UnambiguousMailSeatIdentities(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
	actor, _ := principal.Actor(ctx)
	if snapshot, ok := ctx.Value(mailSeatSnapshotKey{}).(mailSeatSnapshot); ok && snapshot.tx == tx && snapshot.actorID == actor.ID {
		return snapshot.identities, nil
	}
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return nil, err
	}
	return unambiguousIdentities(records, mailIdentityStrength), nil
}

// mailIdentityStrength ranks the sources mail recipient matching trusts: a
// login or proved mailbox outranks an alias the seat declared itself.
var mailIdentityStrength = map[string]int{"login": 2, provedMailboxSource: 2, "user": 1}

// provenIdentityStrength keeps only identities the system proved: the login,
// a proved mailbox, and an address a receiving server delivered to. A seat's
// own declared alias is left out, so it cannot claim somebody else's event.
var provenIdentityStrength = map[string]int{"login": 2, provedMailboxSource: 2, "delivered_to": 2}

// ProvenMailSeatIdentitiesSQL binds each seat's proven, unambiguous addresses,
// for evidence that names a seat as taking part, such as a calendar attendee.
func ProvenMailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(unambiguousIdentities(records, provenIdentityStrength))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d::jsonb", bind(encoded)), nil
}

// unambiguousIdentities gives each address to the one seat holding it at the
// highest strength; a tie between seats gives it to nobody.
func unambiguousIdentities(records []mailSeatIdentity, strength map[string]int) map[string][]string {
	owners, strengths := map[string]string{}, map[string]int{}
	claim := func(seat, label string, strength int) {
		address := mailidentity.ReaderAddress(label, "")
		if address == "" {
			return
		}
		if strength > strengths[address] {
			owners[address], strengths[address] = seat, strength
		} else if strength == strengths[address] && owners[address] != seat {
			owners[address] = ""
		}
	}
	for _, record := range records {
		if level, trusted := strength[record.source]; trusted {
			claim(record.seat, record.label, level)
		}
	}
	out := map[string][]string{}
	for address, seat := range owners {
		if seat != "" {
			out[seat] = append(out[seat], address)
		}
	}
	return out
}

// MailSeatIdentities reads live mailbox identities for restoring factual recipient evidence.
func MailSeatIdentities(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, record := range records {
		if address := mailidentity.ReaderAddress(record.label, record.source); address != "" {
			out[record.seat] = append(out[record.seat], address)
		}
	}
	return out, nil
}

const provedMailboxSource = "proved_mailbox"

type mailSeatIdentity struct{ seat, label, source string }

func readMailSeatIdentities(ctx context.Context, tx pgx.Tx) ([]mailSeatIdentity, error) {
	rows, err := tx.Query(ctx, `SELECT u.id::text, u.email, 'login' FROM app_user u WHERE `+workforce.LiveMemberSQL("u")+`
 UNION SELECT i.user_id::text, i.value, i.source FROM capture_owner_identity i
 JOIN app_user u ON u.id = i.user_id WHERE i.kind = 'address' AND `+workforce.LiveMemberSQL("u")+`
 UNION SELECT c.user_id::text, c.account_label, CASE WHEN `+provedMailboxClause("c")+` THEN '`+provedMailboxSource+`' ELSE '' END FROM capture_connection c
 JOIN app_user u ON u.id = c.user_id WHERE c.archived_at IS NULL AND `+workforce.LiveMemberSQL("u")+` AND coalesce(c.account_label, '') <> ''`)
	if err != nil {
		return nil, fmt.Errorf("reading mail identities: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (mailSeatIdentity, error) {
		var value mailSeatIdentity
		err := row.Scan(&value.seat, &value.label, &value.source)
		return value, err
	})
}

// DirectMailRecipientSQL matches parsed To evidence, never editable body text.
func DirectMailRecipientSQL(activity, addresses string) string {
	clause := `EXISTS (SELECT 1 FROM activity_participant recipient
 WHERE recipient.activity_id = a.id AND recipient.role = 'to'
 AND lower(recipient.address) = ANY(ADDRESS_LIST))`
	return strings.ReplaceAll(strings.ReplaceAll(clause, "a.", activity+"."), "ADDRESS_LIST", addresses)
}

// MailSeatAddressesSQL selects one seat's unambiguous addresses from a bound identity map.
func MailSeatAddressesSQL(identities, seat string) string {
	return "ARRAY(SELECT jsonb_array_elements_text(COALESCE(" + identities + " -> (" + seat + ")::text, '[]'::jsonb)))"
}
