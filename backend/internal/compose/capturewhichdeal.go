// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one "Which deal is this about?" question the capture deal rule raises
// when several open deals fit a mail (capture.AskWhichDeal).
//
// It goes only to the colleague the mailbox confirms is responsible: the seat
// that sent it, or the one seat capture bound to its To. Copied seats, several
// seats, an address the sender typed, a mailbox owner or a record owner are no
// evidence, so with none the mail stays on its company and contacts and
// nobody is asked.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// whichDealActor is the system identity the question task is written under.
const whichDealActor = "system:capture-which-deal"

var (
	whichDealSubject = langcopy.Phrase{
		En: "Which deal is this about?",
		De: "Um welchen Deal geht es hier?",
		Vi: "Thư này thuộc về thương vụ nào?",
	}
	whichDealBody = langcopy.Phrase{
		En: "Several open deals fit this mail, so Margince filed it on none of them. Open the mail and use Relink to put it on the right deal.",
		De: "Mehrere offene Deals passen zu dieser Mail, deshalb hat Margince sie keinem zugeordnet. Öffnen Sie die Mail und ordnen Sie sie mit Neu verknüpfen dem richtigen Deal zu.",
		Vi: "Nhiều thương vụ đang mở phù hợp với thư này, nên Margince chưa gắn thư vào thương vụ nào. Hãy mở thư và dùng Liên kết lại để gắn vào đúng thương vụ.",
	}
)

// whichDealAsker writes the question as a task through the activities store.
type whichDealAsker struct {
	tasks *activities.Store
}

// Ask raises the question at most once per thread. The task is keyed on the
// thread, archived tasks included, so a question a rep dismissed is not asked
// again and a second mail in the thread adds no second question.
func (a whichDealAsker) Ask(ctx context.Context, tx pgx.Tx, q capture.WhichDeal) error {
	seat, err := a.respondent(ctx, tx, q.ActivityID.UUID)
	if err != nil || seat.IsZero() {
		return err
	}
	var thread string
	var shared bool
	err = tx.QueryRow(ctx, `
		SELECT coalesce(nullif(thread_key, ''), id::text), audience = 'workspace'
		  FROM activity WHERE id = $1 AND restricted_at IS NULL AND archived_at IS NULL`, q.ActivityID).Scan(&thread, &shared)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("compose: reading the mail a deal question is about: %w", err)
	}
	lang, err := seatLanguage(ctx, tx, seat)
	if err != nil {
		return err
	}
	// One question per thread for the asked seat. The thread key is the
	// sender's References root, so it is keyed on the seat as well: a sender
	// reusing another mailbox's root cannot use up this seat's question.
	sum := sha256.Sum256([]byte(seat.String() + "\x00" + thread))
	key, subject, body := hex.EncodeToString(sum[:]), whichDealSubject.In(lang), whichDealBody.In(lang)
	source, mail, assignee := provenance.WhichDealSource, q.ActivityID.UUID, ids.From[ids.UserKind](seat)
	in := activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindTask), Subject: &subject, Body: &body,
		SourceSystem: &source, SourceID: &key, Source: source, SourceActivityID: &mail,
		AssigneeID: &assignee, Origin: activities.OriginSystemRemediation,
	}
	for _, record := range []struct {
		kind agentRecordType
		id   ids.UUID
	}{{recordTypeCompany, q.CompanyID}, {recordTypeProject, q.ProjectID}} {
		if !record.id.IsZero() {
			in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: string(record.kind), EntityID: record.id})
		}
	}
	in.AdmitInvitedAssignee()
	if !shared {
		in.VisibleOnlyTo(seat)
	}
	writer := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: whichDealActor, OnBehalfOf: seat})
	_, _, err = a.tasks.LogActivityTx(writer, tx, in)
	return err
}

// respondent is the one colleague the mail names as responsible, or zero.
//
// Only rows the mailbox confirms count, never an address the sender typed:
// a sender could otherwise name any colleague and hand them the question. The
// seat must have imported the mail into their own mailbox, and either be the
// sender of mail the provider attests as their sent mail, or hold a To row
// capture bound to their seat from an address. A seat row without an address only says whose mailbox the
// copy is in, which cannot tell To from Cc, so on its own it asks nobody.
func (a whichDealAsker) respondent(ctx context.Context, tx pgx.Tx, mail ids.UUID) (ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.user_id
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id AND p.user_id IS NOT NULL
		  JOIN capture_import ci ON ci.activity_id = a.id AND ci.user_id = p.user_id
		 WHERE a.id = $1 AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND CASE WHEN a.direction = 'outbound' THEN a.counterparty_outbound_attested AND p.role = 'from'
		            ELSE p.role = 'to' AND p.address IS NOT NULL END
		 ORDER BY p.user_id
		 LIMIT 2`, mail)
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: reading who a deal question is for: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(seats) != 1 {
		return ids.Nil, err
	}
	seat, nobody := seats[0], ids.Nil
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, mail, &nobody, seat)
	if err != nil || !may {
		return ids.Nil, err
	}
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](seat))
	if err != nil || state == activities.SeatCannotHoldWork {
		return ids.Nil, err
	}
	return seat, nil
}

// seatLanguage is the seat's own UI language, else the installation's.
func seatLanguage(ctx context.Context, tx pgx.Tx, seat ids.UUID) (textlang.Lang, error) {
	var locale *string
	if err := tx.QueryRow(ctx, `SELECT locale FROM app_user WHERE id = $1`, seat).Scan(&locale); err != nil {
		return textlang.English, fmt.Errorf("compose: reading the asked colleague's language: %w", err)
	}
	if locale != nil && textlang.Known(*locale) {
		return textlang.Lang(*locale), nil
	}
	base, err := identity.LanguageOf(ctx, tx)
	if err != nil || !textlang.Known(base) {
		return textlang.English, err
	}
	return textlang.Lang(base), nil
}
