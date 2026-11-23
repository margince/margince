import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { useIdempotencyKey } from "../../api/idempotencykey";
import {
  LatestInboundLine,
  useLatestInbound,
  useLatestInboundRead,
} from "../../components/latestinbound";
import { useRailRename } from "../../components/railfield";
import { RecordGone } from "../../components/recordgone";
import { ErrorLine } from "../../design-system/errorline";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import {
  type RecordTimeline,
  useRecordTimeline,
} from "../../design-system/recordtimeline";
import {
  RecordView,
  type RecordViewProps,
} from "../../design-system/recordview";
import { useLocale, useT } from "../../i18n";
import { useTargetApprovals } from "../../lib/approvals.queries";
import { useDeal } from "../../lib/dealread";
import { usePipeline } from "../../lib/deals.queries";
import { useLegalReviewAvailable } from "../../lib/legalreviews.queries";
import { useDealChangesWhere } from "../../lib/marginchanges";
import { fromHere } from "../../lib/navigatefrom";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import type { RecordTabId } from "../../lib/recordtaborder";
import { scrollPageToTop } from "../../lib/reveal";
import { useMe, useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import { useKindWriteRefusal } from "../../lib/writerefusal";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { WorklistReturnLink } from "../worklist/worklist.return";
import {
  type ClosedDeal,
  CloseReviewOffer,
  closedDealOf,
} from "./deal360/closereviewoffer";
import {
  advanceErrorBehind,
  ConfirmAdvanceModal,
  type PendingAdvance,
} from "./deal360/confirmadvance";
import { DealActions } from "./deal360/dealactions";
import { dealBand } from "./deal360/dealband";
import { DealSubtitle } from "./deal360/dealheaderfacts";
import { DealHistoryTab, DealLatestInbound } from "./deal360/dealhistorytab";
import { DealLegalTab } from "./deal360/deallegaltab";
import { marginceLineOf } from "./deal360/dealmarginceline";
import { DealRoomTab } from "./deal360/dealroomtab";
import { useDealTabCounts } from "./deal360/dealtabcounts";
import { currentReplyTarget } from "./deal360/dealwaiting";
import { useDealCoverage } from "./deal360/usedealcoverage";
import { useAdvanceDeal } from "./dealadvance";
import { DealAside } from "./dealaside";
import { DealEditContext, useDealEdit } from "./dealedit";
import { DealFiles } from "./dealfiles";
import { DealReferralLine } from "./dealpartner";
import { DealProjectLine } from "./dealproject";
import type { Deal, Offer, Stage } from "./deals";
import { stagesInOrder } from "./deals.columns";
import { DealDetailsDrawer } from "./deals.details";
import { DealOverviewPane } from "./deals.overview";
import { DealPulsePills } from "./deals.pulsepills";
import { useDealStatusCard } from "./dealstatus";
import { offerWriteRefusal, settleUnsureOfferWrite } from "./offers.actions";

const DEAL_TABS = [
  "overview",
  "history",
  "room",
  "legal",
  "files",
] as const satisfies readonly RecordTabId[];

type DealTab = (typeof DEAL_TABS)[number];

// The tabs this deal offers. The server refuses the legal reviews of an
// archived deal, so the tab is left out there. A reader who had it open lands
// on the overview.
function tabsOf(deal: Deal, legalReview: boolean): readonly DealTab[] {
  return DEAL_TABS.filter(
    (each) => each !== "legal" || (legalReview && !deal.archived_at),
  );
}

function DealTabs({
  deal,
  legalReview,
  thread,
  value,
  onChange,
}: Readonly<{
  deal: Deal;
  legalReview: boolean;
  // The page's own thread read, so History is counted off the rows it holds.
  thread: RecordTimeline;
  value: DealTab;
  onChange: (next: DealTab) => void;
}>) {
  const t = useT();
  const counts = useDealTabCounts(deal, legalReview, thread);
  return (
    <RecordTabs
      options={tabsOf(deal, legalReview)}
      value={value}
      onChange={onChange}
      keepPlace
      counts={counts}
      labels={{
        overview: t("tab.overview"),
        room: t("tab.dealRoom"),
        files: t("tab.documents"),
        legal: t("legalReview.tab"),
        history: t("tab.history"),
      }}
    />
  );
}

export function DealScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const { locale } = useLocale();
  // The instant this page was read at, pinned once, so every "today" and
  // "days since" on it is measured from the same moment.
  const [asOf] = useState(() => new Date().toISOString());
  const readerZone = useViewerZone();
  // Minted here because the band that carries the sentence and the verbs that
  // point at it are two different slots of the same header.
  const readOnlyReasonId = useId();
  const [tab, setTab] = useState<DealTab>("overview");
  // The tab lives in state, so no move of the address opens the new one at its
  // top. A door in a tab's body does that here; the strip keeps the place.
  const openTab = (next: DealTab) => {
    setTab(next);
    scrollPageToTop();
  };
  const [editingDetails, setEditingDetails] = useState(false);
  const legalReview = useLegalReviewAvailable() === true;
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  // The deal that just closed, while the review is on offer for it. Separate
  // from `pending`, which is the question before the close; this is the offer
  // after one landed, and the close dialog is already gone by then.
  const [closed, setClosed] = useState<ClosedDeal | null>(null);
  const queryClient = useQueryClient();
  const advance = useAdvanceDeal();
  const dealQuery = useDeal(id);
  const pipelineQuery = usePipeline(dealQuery.data?.pipeline_id);
  const pipelineRetired = Boolean(pipelineQuery.data?.archived_at);
  // Asked for by what the header line is about, so it is found however many
  // other changes the deal has. A closed deal asks for the close it is on, an
  // open one for the opening or reopening Margince may still take back.
  const closingOccurrence = dealQuery.data?.closing_occurrence_id;
  const marginceReceipts = useDealChangesWhere(
    id,
    closingOccurrence
      ? { stageHistoryId: closingOccurrence }
      : { changeKinds: ["deal_open", "deal_revive"] },
    dealQuery.isSuccess,
  );
  // Every write affordance on this page answers one question, asked once. An
  // archived deal takes no changes, and its verbs stay, refused. One this
  // caller cannot write takes none from them, so its verbs are left out.
  const readOnlyReason = useKindWriteRefusal("deal", dealQuery.data);
  const readOnly = Boolean(readOnlyReason);
  // The id every refused control points at, or nothing while the deal takes
  // changes, resolved once here so the render below reads one name.
  const refusedReasonId = readOnly ? readOnlyReasonId : undefined;
  const me = useMe();
  // Only this deal's pending approvals, filtered by the server.
  const approvalsQuery = useTargetApprovals("deal", id);
  // Read here and shared through the cache. The head's "Your move" pill and
  // How it stands read the status card. The Committee reading and the
  // committee card read coverage. None of them asks a second time.
  //
  // The server answers neither the deal's status nor its coverage once it is
  // archived. So they are asked for only after the deal has said it is live.
  const live = dealQuery.isSuccess && !dealQuery.data.archived_at;
  const statusQuery = useDealStatusCard(id, live);
  const latest = useLatestInbound("deal", id);
  const lastInbound = useLatestInboundRead("deal", id).data?.last_inbound;
  const coverageRead = useDealCoverage(id, live);
  // How it stands reads the whole history, not what the History tab's own
  // filter has narrowed. A "no reply" read off a filtered view would be false.
  const threadQuery = useRecordTimeline("deal", id);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const offersQuery = useQuery({
    queryKey: ["deal-offers", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}/offers", {
          params: { path: { id } },
        }),
      );
    },
  });
  const { key: offerKey, renew: renewOfferKey } = useIdempotencyKey();
  const createOffer = useMutation({
    mutationFn: async (input: {
      dealId: string;
      currency: string;
      fromRevision?: number;
      key: string;
    }) => {
      return unwrap(
        await api.POST("/deals/{id}/offers", {
          params: {
            path: { id: input.dealId },
            header: { "Idempotency-Key": input.key },
          },
          body: {
            currency: input.currency,
            source: "manual",
            from_deal_revision: input.fromRevision,
          },
        }),
      );
    },
    onMutate: fromHere,
    onSuccess: (offer: Offer, _input, from) => {
      renewOfferKey();
      from.navigate({ screen: "offers", id: offer.id });
    },
    onError: (error, input) =>
      settleUnsureOfferWrite(error, queryClient, input.dealId, renewOfferKey),
  });
  const createRefusal = createOffer.isError
    ? offerWriteRefusal(createOffer.error, t)
    : undefined;
  const decide = useMutation({
    mutationFn: async (input: {
      approvalId: string;
      verdict: "approve" | "reject";
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approvals/{id}/approve"
          : "/approvals/{id}/reject";
      unwrap(
        await api.POST(path, {
          params: { path: { id: input.approvalId } },
        }),
      );
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["approvals", "pending"] }),
  });

  return (
    <div className="wrap">
      <QueryGate
        query={dealQuery}
        pendingLabel={t("nav.deals")}
        gone={<RecordGone kind="deal" />}
      >
        {(deal) => {
          const stages = stagesInOrder(pipelineQuery.data?.stages ?? []);
          const openStages = stages.filter(
            (stage) => stage.semantic === "open",
          );
          const dealApprovals = approvalsQuery.data?.data ?? [];
          // Pinned to one closure, so every control that advances this deal
          // shares one version of what an advance means.
          const onAdvance = (toStage: Stage) => {
            // The version this record was drawn from, as the board pins the
            // version its card was drawn from. The write names the deal as
            // the reader saw it, so a change made elsewhere fails loud.
            const input = { dealId: deal.id, version: deal.version, toStage };
            if (toStage.semantic === "open") {
              advance.mutate({
                ...input,
                fromStage: stages.find((stage) => stage.id === deal.stage_id),
              });
            } else {
              setPending(input);
            }
          };
          const shownTab = tabsOf(deal, legalReview).includes(tab)
            ? tab
            : "overview";
          return (
            <DealRecordView
              deal={deal}
              back={<WorklistReturnLink />}
              aside={<DealAside deal={deal} stages={stages} />}
              name={deal.name}
              identity={deal.id}
              // One fact per line under the name: the account, the project
              // the deal delivers into, then the partner who referred it.
              nameBadge={
                <>
                  <DealSubtitle deal={deal} />
                  <DealProjectLine deal={deal} />
                  <DealReferralLine deal={deal} />
                </>
              }
              zone={readerZone}
              pulse={
                <DealPulsePills
                  status={deal.status}
                  archived={Boolean(deal.archived_at)}
                  yourMove={Boolean(
                    statusQuery.data && currentReplyTarget(statusQuery.data),
                  )}
                />
              }
              latest={
                shownTab !== "overview" && (
                  <LatestInboundLine
                    latest={latest}
                    onOpen={() => openTab("overview")}
                  />
                )
              }
              actions={
                <DealActions
                  deal={deal}
                  openStages={openStages}
                  pipelineRetired={pipelineRetired}
                  refusedReasonId={refusedReasonId}
                  onEditDetails={() => setEditingDetails(true)}
                />
              }
              // The sentence naming why this deal takes no changes, while
              // it does not.
              band={dealBand({
                deal,
                reason: readOnlyReason,
                reasonId: readOnlyReasonId,
                marginceLine: marginceLineOf({
                  deal,
                  receipts: marginceReceipts,
                  openStages,
                  pipelineRetired,
                  refusedReasonId,
                }),
              })}
              tabs={
                <DealTabs
                  deal={deal}
                  legalReview={legalReview}
                  thread={threadQuery}
                  value={shownTab}
                  onChange={setTab}
                />
              }
            >
              {shownTab === "overview" && (
                <DealLatestInbound
                  latest={latest}
                  dealId={deal.id}
                  onOpenEmail={setOpenEmail}
                >
                  <DealOverviewPane
                    inputs={{
                      deal,
                      stages,
                      asOf,
                      activities: threadQuery.activities,
                      lastInbound,
                      history: threadQuery,
                      offers: offersQuery.data?.data,
                      coverage: coverageRead,
                      card: statusQuery.data,
                      ladder: {
                        advancing: advance.isPending,
                        // A deal that takes no changes is not moved through
                        // the pipeline. A closed one is reopened by its own
                        // action, whose dialog says what reopening clears.
                        advanceRefused:
                          readOnly || deal.status !== "open" || pipelineRetired,
                        pipelineRetired,
                        refusedReasonId,
                        onAdvance,
                      },
                    }}
                    onOpenEmail={setOpenEmail}
                    onOpenTab={openTab}
                    threadComplete={
                      threadQuery.isSuccess && !threadQuery.hasNextPage
                    }
                    creatingOffer={createOffer.isPending}
                    createRefusal={createRefusal}
                    locale={locale}
                    onCreateOffer={(currency, fromRevision) =>
                      createOffer.mutate({
                        dealId: id,
                        currency,
                        fromRevision,
                        key: offerKey,
                      })
                    }
                    readOnly={readOnly}
                    refusedReasonId={refusedReasonId}
                    dealApprovals={dealApprovals}
                    onDecide={(input) => decide.mutate(input)}
                  />
                </DealLatestInbound>
              )}
              {shownTab === "room" && (
                <DealRoomTab
                  dealId={deal.id}
                  dealName={deal.name}
                  dealWritable={!readOnly}
                />
              )}
              {shownTab === "files" && (
                <DealFiles deal={deal} onOpenEmail={setOpenEmail} />
              )}
              {shownTab === "legal" && <DealLegalTab deal={deal} />}
              {shownTab === "history" && (
                <DealHistoryTab deal={deal} onOpenEmail={setOpenEmail} />
              )}
              {/* One drawer over the deal. Both the overview's citations and
                the history tab's own rows open it, so a message found from
                either place lands in the same reader. */}
              <DealDetailsDrawer
                deal={deal}
                meId={me.data?.user.id ?? ""}
                stages={stages}
                open={editingDetails}
                onClose={() => setEditingDetails(false)}
              />
              <OpenEmailDrawer
                activityId={openEmail}
                zone={readerZone}
                onClose={() => setOpenEmail(null)}
              />
              <ErrorLine error={pipelineQuery.error} />
              <ErrorLine
                error={advanceErrorBehind(advance.error, pending !== null)}
              />
              <ConfirmAdvanceModal
                pending={pending}
                onClose={() => setPending(null)}
                onConfirm={(input) => advance.mutateAsync(input)}
                onClosed={(deal, reason) =>
                  setClosed(closedDealOf(deal, reason))
                }
              />
              <CloseReviewOffer
                closed={closed}
                onDismiss={() => setClosed(null)}
              />
            </DealRecordView>
          );
        }}
      </QueryGate>
    </div>
  );
}

// The record whose head renames it through the details' own edit.
function DealRecordView({
  deal,
  ...view
}: RecordViewProps & Readonly<{ deal: Deal }>) {
  const t = useT();
  const edit = useDealEdit(deal, useViewerId() ?? "");
  const rename = useRailRename(edit, "name", t("create.dealName"));
  return (
    <DealEditContext.Provider value={edit}>
      <RecordView {...view} rename={rename} />
    </DealEditContext.Provider>
  );
}
