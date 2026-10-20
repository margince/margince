import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { PageAsideToggle, usePageAside } from "../../components/pageaside";
import { Badge } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { IdentityLine } from "../../design-system/identityline";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { useRecordTimeline } from "../../design-system/recordtimeline";
import { RecordRefusal, RecordView } from "../../design-system/recordview";
import { useLocale, useT } from "../../i18n";
import { useRecordWriteRefusal } from "../../lib/capability";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { scrollPageToTop } from "../../lib/reveal";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { usePendingApprovals } from "../automations/approvals.queries";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { useLegalReviewAvailable } from "../privacy/legalreviews.queries";
import { TimelineThread } from "../records/record360";
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
import { DealCockpit } from "./deal360/dealcockpit";
import { DealIdentityFacts, DealSubtitle } from "./deal360/dealheaderfacts";
import { DealHistoryTab } from "./deal360/dealhistorytab";
import { DealLegalTab } from "./deal360/deallegaltab";
import { DealPulse } from "./deal360/dealpulse";
import { DealRoomTab } from "./deal360/dealroomtab";
import { useDealCoverage } from "./deal360/usedealcoverage";
import { DealFiles } from "./dealfiles";
import { DealProjectChip } from "./dealproject";
import type { Deal, Offer, Stage } from "./deals";
import { useAdvanceDeal } from "./deals.board";
import { dealStatusTone } from "./deals.columns";
import { DealContext } from "./deals.details";
import { DealOverviewPane } from "./deals.overview";
import { usePipeline } from "./deals.queries";
import { useDealStatusCard } from "./dealstatus";

const DEAL_TABS = ["overview", "room", "files", "legal", "history"] as const;

type DealTab = (typeof DEAL_TABS)[number];

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
  const details = usePageAside();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  // Minted here because the band that carries the sentence and the verbs that
  // point at it are two different slots of the same header.
  const readOnlyReasonId = useId();
  const [tab, setTab] = useState<DealTab>("overview");
  const legalReview = useLegalReviewAvailable();
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  // The deal that just closed, while the review is on offer for it. Separate
  // from `pending`, which is the question before the close; this is the offer
  // after one landed, and the close dialog is already gone by then.
  const [closed, setClosed] = useState<ClosedDeal | null>(null);
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
  // The shared pending-approvals hook, not a second query on the same key. A
  // private queryFn under ["approvals","pending"] takes over the cache entry
  // the Inbox and the rail badge read. One that stops at the first page and
  // keeps expired rows would cap the badge at 50 and count dead approvals.
  const approvalsQuery = usePendingApprovals();
  // Both already fetched by children: the status card by Deal360, coverage by
  // the signal chips and the coverage card. Read here so the header's sentence
  // and the readings band share those cache entries rather than adding two
  // requests for facts the page already holds.
  const statusQuery = useDealStatusCard(id);
  const coverageRead = useDealCoverage(id);
  // The pane's content, handed over whether or not it is showing: `asideOpen`
  // folds the column, and its cards mount only while it is open or leaving.
  const dealContext = (deal: Deal) => (
    <DealContext deal={deal} meId={me.data?.user.id ?? ""} />
  );
  // The thread under the call reads the whole history, not what the History
  // tab's own filter has narrowed. A filter is a view of that tab. A call that
  // said "no reply since" because the reader hid emails there would be false.
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
          const dealApprovals = (approvalsQuery.data?.data ?? []).filter(
            (approval) => approval.target_entity_id === deal.id,
          );
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
          return (
            <RecordView
              back={<WorklistReturnLink />}
              // Context first: who these contacts are, before the verbs that
              // act on them. The seats stay out of the main column because
              // the readings band counts them. Every record page draws this
              // pane, fold and memory.
              aside={dealContext(deal)}
              asideOpen={details.open}
              asideToggle={<PageAsideToggle pane />}
              name={deal.name}
              identity={deal.id}
              // One rung under the record scale. The name stays the largest
              // thing on the page. Beside a work column that opens on the
              // reader's ask, it need not be masthead size.
              scale="compact"
              // The way in (which account this deal is on) sits on the
              // name's own line. Every other record's head draws its
              // subtitle in this inline shape.
              nameBadge={<DealSubtitle deal={deal} />}
              zone={recordZone}
              actionsInline
              // The deal's standing reads beside its name, not at the far
              // end of the header among the verbs. A reader asking "is this
              // still open?" looks at the name, so the answer belongs there.
              pulse={
                <IdentityLine separator="space">
                  <Badge tone={dealStatusTone(deal.status)}>
                    {deal.status}
                  </Badge>
                  <DealProjectChip deal={deal} />
                </IdentityLine>
              }
              // What it is worth, when it is due, whose deal it is and which
              // account it is on, as one line under the name.
              subtitle={<DealIdentityFacts deal={deal} locale={locale} />}
              markShape="tile"
              actions={
                <>
                  {/* Why this deal takes no changes, while it does not: said
                      once, in the record's access note. */}
                  {readOnlyReason !== undefined && (
                    <RecordRefusal id={readOnlyReasonId}>
                      {readOnlyReason}
                    </RecordRefusal>
                  )}
                  <DealActions
                    deal={deal}
                    openStages={openStages}
                    refusedReasonId={refusedReasonId}
                  />
                </>
              }
              // The strip frames the whole record, not the work alone. It
              // runs across both columns and the details pane opens under it.
              // So choosing a body and choosing what stands beside it read as
              // two controls on one page, not one nested in the other.
              tabs={
                <RecordTabs
                  options={DEAL_TABS.filter(
                    (each) => legalReview || each !== "legal",
                  )}
                  value={tab}
                  onChange={(next) => {
                    setTab(next);
                    scrollPageToTop();
                  }}
                  labels={{
                    overview: t("tab.overview"),
                    room: t("tab.dealRoom"),
                    files: t("tab.documents"),
                    legal: t("legalReview.tab"),
                    history: t("tab.history"),
                  }}
                />
              }
            >
              {/* One stack for the whole overview: the work column draws its
                  children with no interval of its own, so the reading and the
                  contacts under it take the record's rhythm from here rather
                  than from a margin one of them carries. */}
              {tab === "overview" && (
                <div className="record-stack">
                  <DealOverviewPane
                    deal={deal}
                    onOpenEmail={setOpenEmail}
                    dealApprovals={dealApprovals}
                    onDecide={(input) => decide.mutate(input)}
                    offers={offersQuery.data?.data}
                    creatingOffer={createOffer.isPending}
                    locale={locale}
                    onCreateOffer={(currency) => createOffer.mutate(currency)}
                    pulse={
                      <DealPulse
                        card={statusQuery.data}
                        timeline={threadQuery.activities}
                      />
                    }
                    spine={
                      <TimelineThread
                        thread={threadQuery}
                        commercial={{
                          next_close_on: deal.expected_close_date,
                        }}
                        onOpenEmail={setOpenEmail}
                      />
                    }
                    coverage={coverageRead}
                    readOnly={readOnly}
                    refusedReasonId={refusedReasonId}
                    // Where the deal stands, in its 360: the stage path and
                    // how long it has stood where it is.
                    path={
                      <DealCockpit
                        deal={deal}
                        stages={stages}
                        advancing={advance.isPending}
                        // A deal that takes no changes (archived, or not this
                        // caller's to write) is not moved through the
                        // pipeline. A closed deal is refused too: reopening
                        // is its own action, with a dialog saying the close
                        // date and frozen rate are cleared.
                        advanceRefused={readOnly || deal.status !== "open"}
                        refusedReasonId={refusedReasonId}
                        onAdvance={onAdvance}
                        inStage={
                          statusQuery.data
                            ? {
                                enteredAt: statusQuery.data.stage_entered_at,
                                usualDays:
                                  statusQuery.data.stage_usual_days ?? null,
                              }
                            : undefined
                        }
                      />
                    }
                  />
                </div>
              )}
              {tab === "room" && (
                <DealRoomTab dealId={deal.id} dealName={deal.name} />
              )}
              {tab === "files" && <DealFiles deal={deal} />}
              {tab === "legal" && legalReview && <DealLegalTab deal={deal} />}
              {tab === "history" && (
                <DealHistoryTab deal={deal} onOpenEmail={setOpenEmail} />
              )}
              {/* One drawer over the deal. Both the overview's citations and
                  the history tab's own rows open it, so a message found from
                  either place lands in the same reader. */}
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
