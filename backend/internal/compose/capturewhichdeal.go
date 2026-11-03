// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one "Which deal is this about?" question the capture deal rule raises
// when several open deals fit a mail (capture.AskWhichDeal).
//
// It goes only to the colleague the mailbox confirms is responsible: the seat
// that sent it, or the one seat capture bound to its To. Copied seats, several
// seats, a typed address, a mailbox owner or a record owner are no evidence.
// With none, nobody is asked.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	now   func() time.Time
}

// Ask raises the question at most once per thread. The task is keyed on the
// thread, archived tasks included. A dismissed question is not asked again,
// and a second mail in the thread adds none.
func (a whichDealAsker) Ask(ctx context.Context, tx pgx.Tx, q capture.WhichDeal) error {
	seat, err := a.respondent(ctx, tx, q.ActivityID.UUID)
	if err != nil || seat.IsZero() {
		return err
	}
	// Only current mail asks. An old mail captured today makes no new work,
	// by the same clause every task reader applies to the question.
	var thread string
	var shared bool
	args := []any{q.ActivityID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `
		SELECT coalesce(nullif(a.thread_key, ''), a.id::text), a.audience = 'workspace'
		  FROM activity a WHERE a.id = $1 AND a.restricted_at IS NULL AND a.archived_at IS NULL
		   AND ` + storekit.CurrentSourceMailClause("a.id", a.now(), arg)
	err = tx.QueryRow(ctx, query, args...).Scan(&thread, &shared)
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
	// The thread key is the sender's References root, so the key names the
	// seat too. A sender reusing another mailbox's root cannot use up this
	// seat's question.
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
// Only rows the mailbox confirms count, never an address the sender typed. The
// seat imported the mail. It sent the mail with the provider attesting it, or
// holds a To row capture bound to the seat from an address. A seat row with
// no address only says whose mailbox the copy is in, not To from Cc.
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

// ActivityDealFiler is capture.DealFiler over the activities module, which
// owns activity_link and the relink event. Capture and its repair write
// through it.
type ActivityDealFiler struct{}

// FileOnDealIfUnfiled is activities.FileOnDealIfUnfiledTx.
func (ActivityDealFiler) FileOnDealIfUnfiled(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any,
) (landed, filed bool, err error) {
	outcome, err := activities.FileOnDealIfUnfiledTx(ctx, tx, id, deal, evidence)
	return outcome == activities.DealFilingLanded, outcome == activities.DealFilingAlreadyFiled, err
}

// RetractMachineDeal is activities.RetractMachineDealTx.
func (ActivityDealFiler) RetractMachineDeal(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any,
) (bool, error) {
	return activities.RetractMachineDealTx(ctx, tx, id, deal, evidence)
}
