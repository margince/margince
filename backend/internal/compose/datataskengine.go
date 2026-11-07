// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

const introductionValidationMigration = "core/1792842046"

// dataTaskTitleBound keeps the task title inside the notice subject's cap with
// room for the words in front of it.
const dataTaskTitleBound = 150

func newDataTaskEngine(pool *pgxpool.Pool, release string, now func() time.Time) (*datatask.Engine, error) {
	return datatask.New(pool, dataTaskCatalog(pool, now), release, now, dataTaskFailureNotice(pool))
}

func dataTaskFailureNotice(pool *pgxpool.Pool) func(context.Context, pgx.Tx, datatask.Progress) error {
	return func(ctx context.Context, tx pgx.Tx, p datatask.Progress) error {
		ctx = datatask.Acting(ctx, p.TaskID)
		recipients, err := identity.DataTaskOperatorsTx(ctx, tx)
		if err != nil {
			return err
		}
		store := notices.NewStore(InstallationDB(pool))
		for _, recipient := range recipients {
			subject, body := dataTaskFailureWords(p)
			_, err := store.CreateTx(ctx, tx, notices.NewNotice{
				Recipient: recipient, Kind: "data_task_failed", Subject: subject, Body: body,
				DedupeKey: fmt.Sprintf("%s:%s:%d", p.RunID, p.Step, p.Version),
				Target:    notices.Target{Type: notices.TargetDataTask, ID: p.RunID},
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// dataTaskFailureWords names the correction that stopped and why, so an
// operator knows which of several dozen tasks to look at before opening any.
func dataTaskFailureWords(p datatask.Progress) (subject, body string) {
	title := p.Title
	if title == "" {
		title = p.TaskID
	}
	subject = "Background correction stopped: " + textcut.Runes(title, dataTaskTitleBound)
	reason := "Its last failure could not be classified."
	if detail, ok := dataTaskFailure(p); ok {
		reason = "Last failure: " + strings.TrimSuffix(detail.Sentence, ".") + ". " + detail.Remedy
	}
	body = fmt.Sprintf("It stopped at step %s after %d failed attempts in a row and will not run again on its own. %s "+
		"Open System health to retry it once the cause is fixed.", p.Step, p.ConsecutiveFailures, reason)
	return subject, body
}

func dataTaskCatalog(pool *pgxpool.Pool, now func() time.Time) []datatask.Task {
	return []datatask.Task{
		heldMailDataTask(now), subjectMailDataTask(now), noticeSourceDataTask(now), homeRepairDataTask(pool, now), mailRecipientsDataTask(), machineAddressDataTask(), pastActivityClocksDataTask(), taskActivityClocksDataTask(),
		unaskedCommitmentsDataTask(), retiredStageCardsDataTask(), colleagueSenderDataTask(), deliveryReportsDataTask(pool, now), liftedAgentCallsDataTask(), mailDealsDataTask(pool), mailBodyCompletenessDataTask(),
		machineSenderDataTask(), signatureTitleHistoryDataTask(pool), manualSignalNoEvidenceDataTask(), companyIndustryDataTask(), contactEmployersDataTask(pool), unknownEmploymentDataTask(pool), leadScoresFollowOriginDataTask(now), phaseHistoryLinksDataTask(),
		membersOnlyMailDataTask(), signatureWindowDataTask(), unsplitAddressDataTask(), dedupeScanDataTask(),
		leadMailDataTask(pool), documentLinksDataTask(), requestProofDataTask(), ownDomainFoldDataTask(), participantEvidenceDataTask(), polymorphicKeysDataTask(), initialsNamesDataTask(), seatEmploymentDataTask(), meetingNoonStartsDataTask(), meetingHistoryPrecisionDataTask(), guessedMailboxSeatsDataTask(), matchedMeetingGuestsDataTask(pool),
		meetingGuestAnswersDataTask(pool),
		calendarDescriptionsDataTask(),
		signalDayCountsDataTask(),
		meetingDuplicatesDataTask(pool),
		meetingPairsDataTask(pool),
		meetingSeriesSlotsDataTask(pool),
		meetingOrganizerHostDataTask(pool),
		linkedInIdentityDataTask(),
		leadContactPairsDataTask(pool),
		addressDomainFoldDataTask(),
		geocodePointsDataTask(),
		signaturePhonesDataTask(),
		lossMappingDataTask(pool),
		retiredThreadSplitDataTask(),
		mergeDroppedPrimaryDataTask(),
		heldContactsDataTask(pool),
		technicalAddressDataTask(pool),
		gmailDedupeScanDataTask(),
		retiredReviewsDataTask(pool),
		auditRedactionDataTask(),
		linkedAuditRedactionDataTask(),
	}
}
