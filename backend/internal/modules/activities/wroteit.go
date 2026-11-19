// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// WroteItOurSide answers who held the pen on one row.
//
// The row's voice names the side when it can. A colleague writing from one of
// our own domains arrives inbound, and direction alone calls that mail theirs.
//
// A machine and an unresolved sender name no side, so those read the transport.
// known is false when neither can answer.
func WroteItOurSide(activity crmcontracts.Activity) (ours, known bool) {
	// Direction first, because having one is what makes a row correspondence. A
	// note or a task is written down rather than sent, whoever authored it, so
	// no side spoke.
	if activity.Direction == nil {
		return false, false
	}
	if voice := activity.Voice; voice != nil && voice.Author != nil && voiceNamesSender(activity.Kind) {
		switch voice.Author.Kind {
		case crmcontracts.ActivityAuthorKindColleague:
			return true, true
		case crmcontracts.ActivityAuthorKindContactRecord:
			return false, true
		}
	}
	switch *activity.Direction {
	case crmcontracts.ActivityDirectionOutbound:
		return true, true
	case crmcontracts.ActivityDirectionInbound:
		return false, true
	}
	return false, false
}

// voiceNamesSender says the voice of this kind names who sent it. On a logged
// call the voice names the seat that wrote the record down, and only the
// direction says who rang.
func voiceNamesSender(kind crmcontracts.ActivityKind) bool {
	switch kind {
	case crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage, crmcontracts.ActivityKindMeeting:
		return true
	}
	return false
}

// WriterSide spells WroteItOurSide as a direction word: "outbound" for our
// side, "inbound" for theirs, empty when no side spoke. A prompt that reads
// direction as authorship then keeps its words and gets the author.
func WriterSide(activity crmcontracts.Activity) string {
	ours, known := WroteItOurSide(activity)
	switch {
	case !known:
		return ""
	case ours:
		return string(crmcontracts.ActivityDirectionOutbound)
	}
	return string(crmcontracts.ActivityDirectionInbound)
}

// WriterSideSQL is WriterSide over the stored columns of activity alias, for a
// reader that never projects the row. It follows authorOf: the seat capture
// recorded, then the From party (the organizer on a meeting), then the
// importer's seat. A sending system and an unresolved sender read direction.
func WriterSideSQL(alias string) string {
	return fmt.Sprintf(writerSideSQL, alias,
		partyAuthorSideSQL(alias, "from", machineSentSQL(alias)), partyExistsSQL(alias, "from"),
		partyAuthorSideSQL(alias, "organizer", "false"), partyExistsSQL(alias, "organizer"),
		storekit.KnownMeetingHostSQL(alias))
}

const writerSideSQL = `(CASE
	WHEN %[1]s.direction IS NULL THEN NULL
	WHEN %[1]s.kind IN ('email','message') THEN CASE
		WHEN %[1]s.invitation_sender IS NOT NULL THEN 'outbound'
		WHEN %[3]s THEN coalesce(%[2]s, %[1]s.direction)
		WHEN %[1]s.source_author_id IS NOT NULL THEN 'outbound'
		ELSE %[1]s.direction END
	WHEN %[1]s.kind = 'meeting' THEN CASE
		WHEN %[5]s THEN coalesce(%[4]s, %[1]s.direction)
		WHEN %[1]s.invitation_sender IS NOT NULL OR (%[6]s) IS NOT NULL
		  OR %[1]s.source_author_id IS NOT NULL THEN 'outbound'
		ELSE %[1]s.direction END
	ELSE %[1]s.direction END)`

// partyExistsSQL says the voice read a party in role, as readEmailPartiesOf
// reads one; a received mail's bare sender counts as its From.
func partyExistsSQL(alias, role string) string {
	exists := `EXISTS (` + partySQL(alias, role) + `)`
	if role != "from" {
		return exists
	}
	return `(` + exists + ` OR (` + alias + `.direction = 'inbound' AND coalesce(` + alias + `.counterparty_email, '') <> ''))`
}

// partyAuthorSideSQL is partyAuthor's side for the row's parties in role, or
// NULL when a machine, an unresolved address or no party wrote it.
func partyAuthorSideSQL(alias, role, machine string) string {
	party := partySQL(alias, role)
	return `(CASE WHEN EXISTS (` + party + ` AND ws.user_id IS NOT NULL) THEN 'outbound'
		WHEN NOT ` + machine + ` AND EXISTS (` + party + ` AND EXISTS (SELECT 1 FROM contact wc
		      WHERE wc.id = ws.contact_id AND wc.archived_at IS NULL)) THEN 'inbound' END)`
}

// partySQL is one stated party of the row in role, under the filters
// readEmailPartiesOf applies.
func partySQL(alias, role string) string {
	return `SELECT 1 FROM activity_participant ws WHERE ws.activity_id = ` + alias + `.id AND ws.role = '` + role +
		`' AND NOT ` + auth.ParticipantUnstatedSQL("ws") + ` AND ` + auth.ParticipantStandsSQL("ws", alias)
}

// machineSentSQL is machineSent over the stored columns: the automatic kind
// relstrength.Evidence names a sending system.
func machineSentSQL(alias string) string {
	return `(` + alias + `.sender_is_machine AND ` + alias + `.kind = 'email' AND ` +
		relstrength.OriginIsEngagementSQL(alias) + ` AND coalesce(` + alias + `.calendar_method, '') = '')`
}
