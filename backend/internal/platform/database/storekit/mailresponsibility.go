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
	owners, strengths := map[string]string{}, map[string]int{}
	// Keyed by the folded address, so a Unicode and a punycode spelling of one
	// mailbox are one claim, and two seats holding them collide.
	claim := func(seat, label string, strength int) {
		address := mailidentity.SeatAddressKey(label, "")
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
		switch record.source {
		case loginSource, provedMailboxSource:
			claim(record.seat, record.label, 2)
		case "user":
			claim(record.seat, record.label, 1)
		}
	}
	out := map[string][]string{}
	for address, seat := range owners {
		if seat != "" {
			// Every spelling a header may carry: Postgres cannot fold IDNA, so
			// the SQL side matches stored addresses against each.
			out[seat] = append(out[seat], mailidentity.AddressSpellings(address)...)
		}
	}
	return out, nil
}

// MailSeatIdentities reads live mailbox identities for restoring factual
// recipient evidence. It keeps a withdrawn mailbox's label, because stored
// mail that mailbox received was the seat's when it arrived.
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

const (
	loginSource            = "login"
	provedMailboxSource    = "proved_mailbox"
	withdrawnMailboxSource = "withdrawn_mailbox"
)

type mailSeatIdentity struct{ seat, label, source string }

// decidesNew is the lifecycle of a withdrawn mailbox: its label is no evidence
// for a decision made now. Only a disconnect withdraws; a mailbox in error or
// awaiting reauth keeps its label as the seat's own. UnambiguousMailSeatIdentities claims no such source,
// and MailSeatIdentities reads it for stored history.
func (r mailSeatIdentity) decidesNew() bool { return r.source != withdrawnMailboxSource }

func readMailSeatIdentities(ctx context.Context, tx pgx.Tx) ([]mailSeatIdentity, error) {
	rows, err := tx.Query(ctx, `SELECT u.id::text, u.email, 'login' FROM app_user u WHERE `+workforce.LiveMemberSQL("u")+`
 UNION SELECT i.user_id::text, i.value, i.source FROM capture_owner_identity i
 JOIN app_user u ON u.id = i.user_id WHERE i.kind = 'address' AND `+workforce.LiveMemberSQL("u")+`
 UNION SELECT c.user_id::text, c.account_label, CASE WHEN `+provedMailboxClause("c")+` THEN '`+provedMailboxSource+`'
   WHEN c.status = 'disconnected' THEN '`+withdrawnMailboxSource+`' ELSE '' END FROM capture_connection c
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

// directRecipientMatch is one parsed To participant naming one of the given
// addresses. The per-mail test and the set of a seat's mail share it.
const directRecipientMatch = `recipient.role = 'to'
 AND lower(recipient.address) = ANY(ADDRESS_LIST)`

// DirectMailRecipientSQL matches parsed To evidence, never editable body text.
func DirectMailRecipientSQL(activity, addresses string) string {
	clause := `EXISTS (SELECT 1 FROM activity_participant recipient
 WHERE recipient.activity_id = a.id AND ` + directRecipientMatch + `)`
	return strings.ReplaceAll(strings.ReplaceAll(clause, "a.", activity+"."), "ADDRESS_LIST", addresses)
}

// DirectMailActivitiesSQL is every activity whose parsed To line names one
// seat, built once as an array. A seat's own reads start from it, over the
// address index, rather than testing each mail in the workspace.
func DirectMailActivitiesSQL(identities, seat string) string {
	return "ARRAY(SELECT recipient.activity_id FROM activity_participant recipient WHERE " +
		strings.ReplaceAll(directRecipientMatch, "ADDRESS_LIST", MailSeatAddressesSQL(identities, seat)) + ")"
}

// DirectMailSeatsSQL lists every seat the mail's parsed To line names.
func DirectMailSeatsSQL(activity, identities string) string {
	return "ARRAY(SELECT mail_seat.key::uuid FROM jsonb_each(" + identities + ") mail_seat WHERE " +
		DirectMailRecipientSQL(activity, "ARRAY(SELECT jsonb_array_elements_text(mail_seat.value))") + ")"
}

// SoleDirectMailSeatSQL decides who answers an inbound mail: the seat in its
// To line when it is the only seat there, else NULL. Two addressed seats make
// no automatic duty for either, and no shared one.
func SoleDirectMailSeatSQL(activity, identities string) string {
	return "(SELECT CASE WHEN cardinality(direct.seats) = 1 THEN direct.seats[1] END FROM (SELECT " +
		DirectMailSeatsSQL(activity, identities) + " AS seats) direct)"
}

// MailSeatAddressesSQL selects one seat's unambiguous addresses from a bound identity map.
func MailSeatAddressesSQL(identities, seat string) string {
	return "ARRAY(SELECT jsonb_array_elements_text(COALESCE(" + identities + " -> (" + seat + ")::text, '[]'::jsonb)))"
}
