// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every workspace-scoped worker refuses args that name no workspace, and
// refuses them BEFORE touching anything.
//
// The role declaration is only load-bearing if the worker acts on it, and
// "acts on it" has to include the empty case: a zero id binds a GUC to
// nothing, and the pass then reads and writes as whatever the connection
// happens to carry. The guard turning that into a failed row is the whole
// reason workspaceJobCtx returns an error rather than a context.
//
// The workers below are constructed with NIL collaborators on purpose. A
// worker that reached its store, its model lane or its pool before checking
// the workspace would panic here rather than fail — so this suite also pins
// the ORDER, which no gate can see.
//
// WHICH workers it must drive is the contract's to say: the set is every
// workspace-role kind api/jobs.yaml declares, read from the declaration below
// rather than counted. A count is satisfied by any N entries, so a kind added
// to the file and forgotten here would leave the number right and its guard
// unproven — which is the same silence the suite exists to break.

import (
	"context"
	"log/slog"
	"maps"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// workspaceKindFloor guards against a vacuous pass. The contract declares 31
// workspace kinds today; the floor sits low enough that retiring a few passes
// does not drag it along, and high enough that a declaration answering nothing
// — which would make the derivation below demand nothing — is reported rather
// than read as full coverage.
const workspaceKindFloor = 25

func TestEveryWorkspaceWorkerRefusesArgsNamingNoWorkspace(t *testing.T) {
	refusals := workspaceRefusalDrivers()

	declared := 0
	for kind, spec := range jobs.Declared() {
		if !bindsAWorkspace(spec) {
			continue
		}
		declared++
		if _, driven := refusals[kind]; !driven {
			t.Errorf("%s carries one tenant's pass, but nothing here proves it refuses args naming no workspace — drive its worker above, or it can stop refusing and no test will notice", kind)
		}
	}
	if declared < workspaceKindFloor {
		t.Fatalf("the contract declares only %d workspace kinds, expected at least %d — the derivation resolved almost nothing and this suite would demand almost nothing",
			declared, workspaceKindFloor)
	}
	for kind := range refusals {
		spec, declaredKind := jobs.SpecFor(kind)
		if !declaredKind || !bindsAWorkspace(spec) {
			t.Errorf("%s is driven here but api/jobs.yaml declares no workspace-binding kind by that name — the suite is pinning something the fleet does not run", kind)
		}
	}

	for kind, work := range refusals {
		t.Run(kind, func(t *testing.T) {
			if err := work(context.Background()); err == nil {
				t.Fatalf("%s accepted args naming no workspace — it would bind an empty GUC and read whatever the connection carries", kind)
			}
		})
	}
}

// A worker given a REAL workspace must get past the guard, bound to THAT
// workspace. Without the positive case the suite above would still pass
// against a guard that refused everything; without the identity check it would
// pass against one that bound the wrong tenant.
func TestTheWorkspaceGuardBindsTheWorkspaceTheArgsDeclare(t *testing.T) {
	want := ids.NewV7()

	ctx, err := workspaceJobCtx(context.Background(), CloseDateWorkspaceArgs{Workspace: want})
	if err != nil {
		t.Fatalf("the guard refused a workspace it was given: %v", err)
	}
	got, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("the guard admitted the workspace but bound nothing — every tenant query would fail on an unset GUC")
	}
	if got != want {
		t.Fatalf("the guard bound %s, want the %s its args declared", got, want)
	}
}

// bindsAWorkspace reads the obligation off the ARGS rather than off the role.
// A worker binds a tenant while its args still name one, and ADR-0091 §8 is
// removing those a module at a time — so a collapsed pass that no longer
// carries a workspace has nothing to refuse, and demanding a refusal from it
// would be demanding a guard against a field that does not exist.
func bindsAWorkspace(spec jobs.Spec) bool {
	for _, arg := range spec.Args {
		if arg.Name == "Workspace" {
			return true
		}
	}
	return false
}

// workspaceRefusalDrivers names one driver per workspace-binding job kind.
//
// Named by kind rather than by Go type: a failure should say which JOB is
// unguarded, because that is what an operator and the ledger both talk in. It
// sits apart from the test so the assertions there stay readable as the fleet
// grows — the table is the fixture, not the reasoning.
func workspaceRefusalDrivers() map[string]func(context.Context) error {
	// Two halves, merged: they are the same census and they differ in what a
	// driver has to hand its job before the guard is reached, which is worth
	// stating once rather than in a comment halfway down one table.
	drivers := zeroPayloadRefusalDrivers()
	maps.Copy(drivers, idBearingRefusalDrivers())
	return drivers
}

// zeroPayloadRefusalDrivers are the kinds whose guard is the FIRST thing their
// Work method reaches, so a zero-value job is enough to drive it.
func zeroPayloadRefusalDrivers() map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		GeocodeOrganizationArgs{}.Kind(): func(ctx context.Context) error {
			return (&geocodeWorker{}).Work(ctx, &river.Job[GeocodeOrganizationArgs]{})
		},
		AccountScanArgs{}.Kind(): func(ctx context.Context) error {
			return (&accountScanWorker{}).Work(ctx, &river.Job[AccountScanArgs]{})
		},
		CheckOrganizationVatArgs{}.Kind(): func(ctx context.Context) error {
			return (&vatCheckWorker{}).Work(ctx, &river.Job[CheckOrganizationVatArgs]{})
		},
		TechnicalEnrichOrganizationArgs{}.Kind(): func(ctx context.Context) error {
			return (&technicalEnrichWorker{}).Work(ctx, &river.Job[TechnicalEnrichOrganizationArgs]{})
		},
		KnowledgeIngestArgs{}.Kind(): func(ctx context.Context) error {
			return (&knowledgeIngestWorker{}).Work(ctx, &river.Job[KnowledgeIngestArgs]{})
		},
		VCardIngestArgs{}.Kind(): func(ctx context.Context) error {
			return (&vcardIngestWorker{}).Work(ctx, &river.Job[VCardIngestArgs]{})
		},
		AssuranceWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&assuranceWorkspaceWorker{}).Work(ctx, &river.Job[AssuranceWorkspaceArgs]{})
		},
		CloseDateWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&closeDateWorkspaceWorker{}).Work(ctx, &river.Job[CloseDateWorkspaceArgs]{})
		},
		ForecastSnapshotWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&forecastSnapshotWorkspaceWorker{}).Work(ctx, &river.Job[ForecastSnapshotWorkspaceArgs]{})
		},
		FollowUpWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&followUpWorkspaceWorker{}).Work(ctx, &river.Job[FollowUpWorkspaceArgs]{})
		},
		TimeScanWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&timeScanWorkspaceWorker{}).Work(ctx, &river.Job[TimeScanWorkspaceArgs]{})
		},
		IdempotencyRetentionWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&idempotencyRetentionWorkspaceWorker{}).Work(ctx, &river.Job[IdempotencyRetentionWorkspaceArgs]{})
		},
		GraphEdgeWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&graphEdgeWorkspaceWorker{}).Work(ctx, &river.Job[GraphEdgeWorkspaceArgs]{})
		},
		ParticipantBackfillWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&participantBackfillWorkspaceWorker{}).Work(ctx, &river.Job[ParticipantBackfillWorkspaceArgs]{})
		},
		LinkedInRematchWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&linkedInRematchWorkspaceWorker{}).Work(ctx, &river.Job[LinkedInRematchWorkspaceArgs]{})
		},
		LinkReconcileWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&linkReconcileWorkspaceWorker{}).Work(ctx, &river.Job[LinkReconcileWorkspaceArgs]{})
		},
		OrgNamePromotionWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&orgNamePromotionWorkspaceWorker{}).Work(ctx, &river.Job[OrgNamePromotionWorkspaceArgs]{})
		},
		CaptureDigestWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&captureDigestWorkspaceWorker{}).Work(ctx, &river.Job[CaptureDigestWorkspaceArgs]{})
		},
		BriefGenerateWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&briefGenerateWorkspaceWorker{}).Work(ctx, &river.Job[BriefGenerateWorkspaceArgs]{})
		},
		WeeklyReviewGenerateWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&weeklyGenerateWorkspaceWorker{}).Work(ctx, &river.Job[WeeklyReviewGenerateWorkspaceArgs]{})
		},
		OverlayReconcileWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&overlayReconcileWorkspaceWorker{}).Work(ctx, &river.Job[OverlayReconcileWorkspaceArgs]{})
		},
		GmailWatchRenewArgs{}.Kind(): func(ctx context.Context) error {
			return (&gmailWatchRenewWorker{}).Work(ctx, &river.Job[GmailWatchRenewArgs]{})
		},
		GraphWatchRenewArgs{}.Kind(): func(ctx context.Context) error {
			return (&graphWatchRenewWorker{}).Work(ctx, &river.Job[GraphWatchRenewArgs]{})
		},
		FxRateRefreshArgs{}.Kind(): func(ctx context.Context) error {
			return (&fxRefreshWorker{}).Work(ctx, &river.Job[FxRateRefreshArgs]{})
		},
		AiModelRateRefreshArgs{}.Kind(): func(ctx context.Context) error {
			return (&aiModelRateRefreshWorker{}).Work(ctx, &river.Job[AiModelRateRefreshArgs]{})
		},
	}
}

// idBearingRefusalDrivers are the kinds that parse one OTHER id before reaching
// the guard, so their args carry a valid one: with a zero-value payload the
// earlier parse would fail and the test would pass without the workspace guard
// ever running.
func idBearingRefusalDrivers() map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		SendEmailArgs{}.Kind(): func(ctx context.Context) error {
			return (&commsSendWorker{}).Work(ctx, &river.Job[SendEmailArgs]{
				Args: SendEmailArgs{DeliveryID: ids.NewV7().String()},
			})
		},
		CaptureBackfillArgs{}.Kind(): func(ctx context.Context) error {
			return (&captureBackfillWorker{}).Work(ctx, &river.Job[CaptureBackfillArgs]{
				Args: CaptureBackfillArgs{BackfillID: ids.NewV7().String()},
			})
		},
		CaptureSyncArgs{}.Kind(): func(ctx context.Context) error {
			return (&captureSyncWorker{}).Work(ctx, &river.Job[CaptureSyncArgs]{
				Args: CaptureSyncArgs{ConnectionID: ids.NewV7().String()},
			})
		},
		TelegramIngestArgs{}.Kind(): func(ctx context.Context) error {
			return (&telegramIngestWorker{}).Work(ctx, &river.Job[TelegramIngestArgs]{
				Args: TelegramIngestArgs{RawCaptureID: ids.NewV7().String()},
			})
		},
		TelegramPollArgs{}.Kind(): func(ctx context.Context) error {
			return (&telegramPollWorker{}).Work(ctx, &river.Job[TelegramPollArgs]{
				Args: TelegramPollArgs{ConnectionID: ids.NewV7().String()},
			})
		},
		VoiceBuildArgs{}.Kind(): func(ctx context.Context) error {
			return (&voiceBuildWorker{}).Work(ctx, &river.Job[VoiceBuildArgs]{
				Args: VoiceBuildArgs{RequestedBy: ids.NewV7().String()},
			})
		},
		OverlayRefetchArgs{}.Kind(): func(ctx context.Context) error {
			return (&overlayRefetchWorker{log: slog.New(slog.DiscardHandler)}).Work(
				ctx, &river.Job[OverlayRefetchArgs]{})
		},
		SiteDeepReadArgs{}.Kind(): func(ctx context.Context) error {
			return (&siteDeepReadWorker{}).Work(ctx, &river.Job[SiteDeepReadArgs]{
				Args: SiteDeepReadArgs{SiteReadID: ids.NewV7()},
			})
		},
		SignalScanWorkspaceArgs{}.Kind(): func(ctx context.Context) error {
			return (&signalScanWorkspaceWorker{}).Work(ctx, &river.Job[SignalScanWorkspaceArgs]{})
		},
		ProviderRunPollArgs{}.Kind(): func(ctx context.Context) error {
			return (&providerRunPollWorker{}).Work(ctx, &river.Job[ProviderRunPollArgs]{})
		},
		ProviderLookupArgs{}.Kind(): func(ctx context.Context) error {
			return (&providerLookupWorker{}).Work(ctx, &river.Job[ProviderLookupArgs]{})
		},
		ProviderRunSubmitArgs{}.Kind(): func(ctx context.Context) error {
			return (&providerRunSubmitWorker{}).Work(ctx, &river.Job[ProviderRunSubmitArgs]{
				Args: ProviderRunSubmitArgs{RunID: ids.NewV7().String()},
			})
		},

		ScheduledSendArgs{}.Kind(): func(ctx context.Context) error {
			return (&scheduledSendWorker{}).Work(ctx, &river.Job[ScheduledSendArgs]{
				Args: ScheduledSendArgs{ScheduledSendID: ids.NewV7().String()},
			})
		},

		TranscriptProposeArgs{}.Kind(): func(ctx context.Context) error {
			return (&transcriptProposeWorker{log: slog.New(slog.DiscardHandler)}).Work(
				ctx, &river.Job[TranscriptProposeArgs]{Args: TranscriptProposeArgs{}})
		},

		DocumentExtractArgs{}.Kind(): func(ctx context.Context) error {
			return (&documentExtractWorker{log: slog.New(slog.DiscardHandler)}).Work(
				ctx, &river.Job[DocumentExtractArgs]{Args: DocumentExtractArgs{}})
		},
	}
}
