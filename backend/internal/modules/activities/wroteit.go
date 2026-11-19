// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"slices"
	"strings"

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

// senderNamingKinds are the kinds whose voice names who sent them. On a logged
// call the voice names the seat that wrote the record down, and only the
// direction says who rang.
var senderNamingKinds = []crmcontracts.ActivityKind{
	crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage, crmcontracts.ActivityKindMeeting,
}

func voiceNamesSender(kind crmcontracts.ActivityKind) bool {
	return slices.Contains(senderNamingKinds, kind)
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
// recorded, then the first identified From party (the organizer on a meeting),
// then the importer's seat. A sending system and an unresolved sender read
// direction, and so does a row the reader may not open (readable false), whose
// voice the projection withholds. Two answers of the projection depend on the
// reader and stay out: a bare From address it resolves to a contact, and a
// contact outside the reader's scope. Both arise only on a row with a direction
// capture already set from the same From, so the answers agree in practice.
func WriterSideSQL(alias, readable string) string {
	return fmt.Sprintf(writerSideSQL, alias,
		firstPartySideSQL(alias, "from", relstrength.SendingSystemSQL(alias)), fromPartyExistsSQL(alias),
		firstPartySideSQL(alias, "organizer", "false"), `EXISTS (`+partySQL(alias, "organizer")+`)`,
		storekit.KnownMeetingHostSQL(alias), readable, kindListSQL(crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage))
}

const writerSideSQL = `(CASE
	WHEN %[1]s.direction IS NULL THEN NULL
	WHEN NOT (%[7]s) THEN %[1]s.direction
	WHEN %[1]s.kind IN (%[8]s) THEN CASE
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

func kindListSQL(kinds ...crmcontracts.ActivityKind) string {
	quoted := make([]string, len(kinds))
	for i, kind := range kinds {
		quoted[i] = "'" + string(kind) + "'"
	}
	return strings.Join(quoted, ",")
}

// fromPartyExistsSQL says the voice read a From party: a stated one, or the
// received mail's bare sender receivedFromSQL stands in.
func fromPartyExistsSQL(alias string) string {
	return `(EXISTS (` + partySQL(alias, "from") + `) OR ` + receivedFromArmSQL(alias) + `)`
}

// firstPartySideSQL is partyAuthor's side for the first party in role a seat or
// live contact identifies (identifiedFirst), or NULL when a machine, an
// unresolved address or no party wrote it.
func firstPartySideSQL(alias, role, machine string) string {
	return `(SELECT CASE WHEN ws.user_id IS NOT NULL THEN 'outbound' WHEN NOT ` + machine + ` THEN 'inbound' END
		FROM activity_participant ws
		WHERE ` + partyWhereSQL(alias, role) + ` AND ` + auth.ParticipantStandsSQL("ws", alias) + `
		  AND (ws.user_id IS NOT NULL OR EXISTS (SELECT 1 FROM contact wc WHERE wc.id = ws.contact_id AND wc.archived_at IS NULL))
		ORDER BY ws.created_at, ws.id LIMIT 1)`
}

// partySQL is one stated party of the row in role, under the filters
// readEmailPartiesOf applies.
func partySQL(alias, role string) string {
	return `SELECT 1 FROM activity_participant ws WHERE ` + partyWhereSQL(alias, role) + ` AND ` + auth.ParticipantStandsSQL("ws", alias)
}

func partyWhereSQL(alias, role string) string {
	return `ws.activity_id = ` + alias + `.id AND ws.role = '` + role + `' AND NOT ` + auth.ParticipantUnstatedSQL("ws")
}
