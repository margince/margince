// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/companyclockrefold"
	"github.com/margince/margince/backend/internal/compose/companyprofiles"
	"github.com/margince/margince/backend/internal/compose/credentialrepair"
	"github.com/margince/margince/backend/internal/compose/customerlinks"
	"github.com/margince/margince/backend/internal/compose/dealclockrefold"
	"github.com/margince/margince/backend/internal/compose/employmentrepair"
	"github.com/margince/margince/backend/internal/compose/erasedseattraces"
	"github.com/margince/margince/backend/internal/compose/halfvectors"
	"github.com/margince/margince/backend/internal/compose/halvedprobabilities"
	"github.com/margince/margince/backend/internal/compose/importscopes"
	"github.com/margince/margince/backend/internal/compose/initiativeenrolment"
	"github.com/margince/margince/backend/internal/compose/invitationsenders"
	"github.com/margince/margince/backend/internal/compose/meetingrosters"
	"github.com/margince/margince/backend/internal/compose/negativecounts"
	"github.com/margince/margince/backend/internal/compose/offernetdeals"
	"github.com/margince/margince/backend/internal/compose/participantaddresses"
	"github.com/margince/margince/backend/internal/compose/provensenders"
	"github.com/margince/margince/backend/internal/compose/qualifytasks"
	"github.com/margince/margince/backend/internal/compose/readproposals"
	"github.com/margince/margince/backend/internal/compose/recordsources"
	"github.com/margince/margince/backend/internal/compose/retiredstagecards"
	"github.com/margince/margince/backend/internal/compose/retiredthreadsplit"
	"github.com/margince/margince/backend/internal/compose/rolemailboxseats"
	"github.com/margince/margince/backend/internal/compose/strengthdates"
	"github.com/margince/margince/backend/internal/compose/unaskedcommitments"
	"github.com/margince/margince/backend/internal/compose/viabilitytasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const introductionValidationMigration = "core/1792842046"

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
			subject, body, err := dataTaskFailureWords(p)
			if err != nil {
				return err
			}
			_, err = store.CreateTx(ctx, tx, notices.NewNotice{
				Recipient: recipient, Kind: "data_task_failed", Subject: subject, Body: body, SystemText: dataTaskFailureMessage(p),
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
func dataTaskFailureWords(p datatask.Progress) (subject, body string, err error) {
	return dataTaskFailureMessage(p).EnglishWords()
}

func dataTaskFailureMessage(p datatask.Progress) *systemtext.Message {
	title := p.Title
	if title == "" {
		title = p.TaskID
	}
	subject := systemtext.Sentence("datatask.stopped", systemtext.Reference(dataTaskTitleCode(p.TaskID), title))
	reason := systemtext.Sentence("datatask.unclassified")
	if detail, ok := dataTaskFailure(p); ok {
		reason = systemtext.Sentence("datatask.failure", systemtext.Reference("jobs."+detail.Class+".sentence", detail.Sentence), systemtext.Reference("jobs."+detail.Class+".remedy", detail.Remedy))
	}
	body := systemtext.Sequence(systemtext.Sentence("datatask.progress", systemtext.Text(p.Step), systemtext.Count(int64(p.ConsecutiveFailures))), reason, systemtext.Sentence("datatask.retry"))
	return &systemtext.Message{Subject: subject, Body: body}
}

func dataTaskTitleCode(taskID string) string {
	return "datatask.title." + strings.ReplaceAll(taskID, "/", ".")
}

func dataTaskCatalog(pool *pgxpool.Pool, now func() time.Time) []datatask.Task {
	return []datatask.Task{
		heldMailDataTask(now), subjectMailDataTask(now), noticeSourceDataTask(now), homeRepairDataTask(pool, now), mailRecipientsDataTask(), machineAddressDataTask(), pastActivityClocksDataTask(), taskActivityClocksDataTask(),
		unaskedcommitments.DataTask(), retiredstagecards.DataTask(), colleagueSenderDataTask(), deliveryReportsDataTask(pool, now), liftedAgentCallsDataTask(), mailDealsDataTask(pool), mailBodyCompletenessDataTask(),
		machineSenderDataTask(), signatureTitleHistoryDataTask(pool), manualSignalNoEvidenceDataTask(), companyprofiles.IndustryTask(), contactEmployersDataTask(pool), unknownEmploymentDataTask(pool), leadScoresFollowOriginDataTask(now), phaseHistoryLinksDataTask(),
		activities.MeetingNotesDataTask(activities.NewStore(InstallationDB(pool)).WithLeadQualifyTasks(newLeadWorkTasks(InstallationDB(pool), slog.Default()))), activities.CaptureLabelsDataTask(), activities.SearchTextDataTask(), membersOnlyMailDataTask(), signatureWindowDataTask(), unsplitAddressDataTask(), dedupeScanDataTask(),
		leadMailDataTask(pool), documentLinksDataTask(), requestProofDataTask(), ownDomainFoldDataTask(), participantEvidenceDataTask(), polymorphicKeysDataTask(), initialsNamesDataTask(), seatEmploymentDataTask(), meetingNoonStartsDataTask(), meetingHistoryPrecisionDataTask(), guessedMailboxSeatsDataTask(), matchedMeetingGuestsDataTask(pool),
		meetingGuestAnswersDataTask(pool),
		calendarDescriptionsDataTask(),
		calendarMethodsDataTask(pool),
		signalDayCountsDataTask(),
		meetingDuplicatesDataTask(pool),
		meetingPairsDataTask(pool),
		meetingSeriesSlotsDataTask(pool),
		splitSeriesDataTask(pool), activities.ImportedMeetingCopiesDataTask(activities.NewStore(InstallationDB(pool))),
		meetingOrganizerHostDataTask(pool),
		linkedInIdentityDataTask(),
		leadContactPairsDataTask(pool),
		addressDomainFoldDataTask(),
		deals.SeededStageNamesDataTask(),
		geocodePointsDataTask(),
		signaturePhonesDataTask(),
		lossMappingDataTask(pool),
		threadHoldClosesDataTask(pool),
		retiredthreadsplit.DataTask(),
		mergeDroppedPrimaryDataTask(),
		initiativeenrolment.Task(NewInitiativeStore(pool)),
		heldContactsDataTask(pool),
		technicalAddressDataTask(pool),
		gmailDedupeScanDataTask(),
		retiredReviewsDataTask(pool),
		viabilitytasks.DataTask(activities.NewStore(InstallationDB(pool))),
		auditRedactionDataTask(),
		linkedAuditRedactionDataTask(pool),
		markedAuditRedactionDataTask(),
		keyedIdentifiersDataTask(),
		stockLaddersDataTask(ai.NewRoutingStore(NewSettingsStore(pool), StoredConfig(pool)).AdoptStockTaskLaddersTx),
		mailCopiesDataTask(pool),
		answeredTasksDataTask(pool),
		offernetdeals.DataTask(InstallationDB(pool), DealsInstallation(), notices.NewStore(InstallationDB(pool))),
		halvedprobabilities.DataTask(halvedProbabilityStore(pool)),
		falseMeetingCancelsDataTask(now),
		provensenders.DataTask(),
		importscopes.DataTask(),
		invitationsenders.DataTask(),
		meetingrosters.GraphGuestsDataTask(identity.NewService(pool), installationContext(pool)), meetingrosters.BirthRostersDataTask(), meetingrosters.SeatSetsDataTask(),
		credentialrepair.DataTask(capture.NewPendingStore(InstallationDB(pool)).PurgeRawCaptureTx),
		erasedseattraces.DataTask(),
		newslettersDataTask(pool),
		dealclockrefold.DataTask(), dealclockrefold.PersonalMeetingsTask(),
		companyclockrefold.DataTask(), companyclockrefold.PersonalMailTask(), strengthdates.DataTask(), strengthdates.DirectionlessSendersTask(),
		customerlinks.DataTask(),
		qualifytasks.DuplicatesDataTask(now), qualifytasks.TextDataTask(),
		companyprofiles.OfferDuplicatesTask(),
		qualifytasks.SystemTextDataTask(), employmentrepair.DataTask(contacts.NewStore(InstallationDB(pool))), halfvectors.DataTask(), recordsources.DataTask(), recordsources.SignalDataTask(), recordsources.PartyDataTask(), recordsources.LeadDataTask(), rolemailboxseats.DataTask(), participantaddresses.DataTask(),
		activities.TeamLinksDataTask(activities.NewStore(InstallationDB(pool)).WithTeamRequestNotice(teamRequestNotice{}), now), contacts.LegacyEvidenceDataTask(),
		negativecounts.DataTask(),
		readproposals.DataTask(),
	}
}
