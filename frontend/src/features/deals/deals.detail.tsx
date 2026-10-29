import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import { DealStatusBadge } from "../../components/dealstatusbadge";
import {
  LatestInboundLine,
  useLatestInbound,
} from "../../components/latestinbound";
import { Badge } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import {
  type RecordTimeline,
  useRecordTimeline,
} from "../../design-system/recordtimeline";
import { RecordView } from "../../design-system/recordview";
import { useLocale, useT } from "../../i18n";
import { useTargetApprovals } from "../../lib/approvals.queries";
import { useRecordWriteRefusal } from "../../lib/capability";
import { usePipeline } from "../../lib/deals.queries";
import { useLegalReviewAvailable } from "../../lib/legalreviews.queries";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { scrollPageToTop } from "../../lib/reveal";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
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
import { DealSubtitle } from "./deal360/dealheaderfacts";
import { DealHistoryTab, DealLatestInbound } from "./deal360/dealhistorytab";
import { DealLegalTab } from "./deal360/deallegaltab";
import { DealRoomTab } from "./deal360/dealroomtab";
import { useDealTabCounts } from "./deal360/dealtabcounts";
import { currentReplyTarget } from "./deal360/dealwaiting";
import { useDealCoverage } from "./deal360/usedealcoverage";
import { DealAside } from "./dealaside";
import { DealFiles } from "./dealfiles";
import { DealProjectLine } from "./dealproject";
import type { Deal, Offer, Stage } from "./deals";
import { useAdvanceDeal } from "./deals.board";
import { DealDetailsDrawer } from "./deals.details";
import { DealOverviewPane } from "./deals.overview";
import { useDealStatusCard } from "./dealstatus";

const DEAL_TABS = ["overview", "room", "files", "legal", "history"] as const;

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

// One sentence for why this deal takes no changes, whichever reason applies,
// said once across the header so every refused control can point at it.
// Absent while the deal is live and the reader's: `undefined`, not an element
// that renders null, because RecordView reserves space for anything it is
// handed. A line always reserved would read as a record with nothing said.
function dealBand({
  reason,
  reasonId,
}: Readonly<{
  reason: string | undefined;
  reasonId: string;
}>): ReactNode | undefined {
  if (reason === undefined) {
    return undefined;
  }
  return <p id={reasonId}>{reason}</p>;
}

// The pills under the deal's name: whose move it is, when a buyer is owed a
// reply, then the deal's status.
function DealPulsePills({
  status,
  yourMove,
}: Readonly<{ status: Deal["status"]; yourMove: boolean }>) {
  const t = useT();
  return (
    <>
      {yourMove && (
        <Badge tone="warning" dot>
          {t("deal.standing.teamReply")}
        </Badge>
      )}
      <DealStatusBadge status={status} />
    </>
  );
}

/**
 * useDeal is the deal page's read of the deal record itself.
 *
 * A hook rather than a query inline in the screen, so the four record reads
 * are mounted the same way. That lets one suite prove the live cadence over
 * all of them (FE-PARAM-5, app/queryclient.ts). The cadence itself is the
 * client's, keyed on the read: nothing is spread in here to forget.
 */
export function useDeal(id: string) {
  return useQuery({
    queryKey: ["deal", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });
}

export function DealScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const { locale } = useLocale();
  // The instant this page was read at, pinned once, so every "today" and
  // "days since" on it is measured from the same moment.
  const [asOf] = useState(() => new Date().toISOString());
  const recordZone = useRecordZone();
  // Minted here because the band that carries the sentence and the verbs that
  // point at it are two different slots of the same header.
  const readOnlyReasonId = useId();
  const [tab, setTab] = useState<DealTab>("overview");
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
  // Every write affordance on this page answers one question, asked once. An
  // archived deal takes no changes, and one this caller cannot write takes
  // none from them. Asking only the first half would offer a rep Edit on a
  // colleague's deal, then refuse the save with a 403.
  const readOnlyReason = useRecordWriteRefusal("deal", dealQuery.data, {
    archived: t("deal.archivedReadOnly"),
    notYours: t("deal.notYoursToChange"),
  });
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
  const createOffer = useMutation({
    mutationFn: async (currency: string) => {
      return unwrap(
        await api.POST("/deals/{id}/offers", {
          params: { path: { id } },
          body: { currency, source: "manual" },
        }),
      );
    },
    onSuccess: (offer: Offer) => {
      navigate({ screen: "offers", id: offer.id });
    },
  });
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
      <QueryGate query={dealQuery} pendingLabel={t("nav.deals")}>
        {(deal) => {
          const stages = [...(pipelineQuery.data?.stages ?? [])].sort(
            (a, b) => a.position - b.position,
          );
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
              advance.mutate(input);
            } else {
              setPending(input);
            }
          };
          const shownTab = tabsOf(deal, legalReview).includes(tab)
            ? tab
            : "overview";
          return (
            <RecordView
              back={<WorklistReturnLink />}
              aside={<DealAside deal={deal} stages={stages} />}
              name={deal.name}
              identity={deal.id}
              // One fact per line under the name: the account, then the
              // project the deal delivers into.
              nameBadge={
                <>
                  <DealSubtitle deal={deal} />
                  <DealProjectLine deal={deal} />
                </>
              }
              zone={recordZone}
              pulse={
                <DealPulsePills
                  status={deal.status}
                  yourMove={Boolean(
                    statusQuery.data && currentReplyTarget(statusQuery.data),
                  )}
                />
              }
              latest={
                shownTab !== "overview" && (
                  <LatestInboundLine
                    latest={latest}
                    onOpen={() => setTab("overview")}
                  />
                )
              }
              actions={
                <DealActions
                  deal={deal}
                  openStages={openStages}
                  refusedReasonId={refusedReasonId}
                  onEditDetails={() => setEditingDetails(true)}
                />
              }
              // The sentence naming why this deal takes no changes, while
              // it does not.
              band={dealBand({
                reason: readOnlyReason,
                reasonId: readOnlyReasonId,
              })}
              tabs={
                <DealTabs
                  deal={deal}
                  legalReview={legalReview}
                  thread={threadQuery}
                  value={shownTab}
                  onChange={(next) => {
                    setTab(next);
                    scrollPageToTop();
                  }}
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
                      offers: offersQuery.data?.data,
                      coverage: coverageRead,
                      card: statusQuery.data,
                      ladder: {
                        advancing: advance.isPending,
                        // A deal that takes no changes is not moved through
                        // the pipeline. A closed one is reopened by its own
                        // action, whose dialog says what reopening clears.
                        advanceRefused: readOnly || deal.status !== "open",
                        refusedReasonId,
                        onAdvance,
                      },
                    }}
                    onOpenEmail={setOpenEmail}
                    onOpenTab={(next) => {
                      setTab(next);
                      scrollPageToTop();
                    }}
                    onEditDetails={() => setEditingDetails(true)}
                    threadComplete={
                      threadQuery.isSuccess && !threadQuery.hasNextPage
                    }
                    creatingOffer={createOffer.isPending}
                    locale={locale}
                    onCreateOffer={(currency) => createOffer.mutate(currency)}
                    readOnly={readOnly}
                    refusedReasonId={refusedReasonId}
                    dealApprovals={dealApprovals}
                    onDecide={(input) => decide.mutate(input)}
                  />
                </DealLatestInbound>
              )}
              {shownTab === "room" && (
                <DealRoomTab dealId={deal.id} dealName={deal.name} />
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
                zone={recordZone}
                onClose={() => setOpenEmail(null)}
              />
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
            </RecordView>
          );
        }}
      </QueryGate>
    </div>
  );
}
