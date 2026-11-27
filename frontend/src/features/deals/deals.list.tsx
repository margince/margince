import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { usePageName } from "../../app/pagemeta";
import { BULK_MAX_ITEMS } from "../../components/bulkchangeapi";
import { useObjectCustomFields } from "../../components/customfields.form";
import { usePartnerOptions } from "../../components/dealpartnerfields";
import {
  type ListState,
  ListTable,
  useTagChips,
} from "../../components/listquery";
import { useOwnerChips } from "../../components/ownerdials";
import { mineEmptyLine, standardViews } from "../../components/recordlist";
import { RefusedFiltersNote } from "../../components/refusedfiltersnote";
import { ErrorLine } from "../../design-system/errorline";
import { useRowSelection } from "../../design-system/listselection";
import { useLocale, useT } from "../../i18n";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import { useCan } from "../../lib/capability";
import { maskRefusedDialOf } from "../../lib/dealranges";
import {
  totalsRefusedBecause,
  totalsWithheldBecause,
} from "../../lib/deals.queries";
import { useHealthScope } from "../../lib/healthscope";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { useReReadMeWhen } from "../../lib/refusedfilters";
import { useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import {
  SavedViewsFailure,
  SaveViewAction,
  useSavedViewTabs,
} from "../lists/savedviews";
import { LegalReviewQueueLink } from "../privacy/legalreviews";
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
import { useAdvanceDeal } from "./dealadvance";
import type { Deal } from "./deals";
import {
  BoardHeadCount,
  BoardReopenModal,
  boardIsEmpty,
  DealBoardBody,
  useBoardInteractions,
} from "./deals.board";
import { dealColumns } from "./deals.columns";
import { DealCreateAction, useCreateRefusal } from "./deals.createaction";
import {
  dealRowSelection,
  dealViewDials,
  railDealView,
  savableDealQuery,
  useDealChips,
  useDealScreenDials,
} from "./deals.listtools";
import { useSurfaceReads } from "./deals.surfacereads";

// The board names its cards' companies from this page, and a reader without
// company.read is refused it.
function useBoardCompanies() {
  const readsCompanies = useCan("company", "read");
  const readerScope = useHealthScope();
  return useQuery({
    queryKey: ["companies", readerScope ?? ""],
    enabled: readsCompanies && readerScope !== undefined,
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies", {
          params: { query: { limit: 50 } },
        }),
      );
    },
  });
}

export function DealsScreen({
  startCreating = false,
}: Readonly<{ startCreating?: boolean }>) {
  const t = useT();
  const pageName = usePageName("deals");
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const pipelinesQuery = usePipelines();
  const ownerChips = useOwnerChips();
  const viewerId = useViewerId();
  const savedViews = useSavedViewTabs("deals");
  const {
    query,
    setQuery,
    baseCurrency,
    withheld,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
    refusedDials,
  } = useDealScreenDials({ pipelines: pipelinesQuery.data });
  const cf = useObjectCustomFields("deal", "create", {
    pipelineId: effectivePipeline?.id,
  });
  const stages = effectivePipeline?.stages ?? [];
  const companiesQuery = useBoardCompanies();

  const partnerOptions = usePartnerOptions(companiesQuery.data?.data ?? []);
  // The channel catalog for the filter chip. Shared cache with the edit form's
  // own read, so opening one after the other costs a single request.
  const acquisitionSources = useAcquisitionSources().data;
  const { dealChips, rangeChips, surfaceChips, nameRefusal } = useDealChips({
    stages,
    query,
    withheld,
    baseCurrency,
    partnerOptions,
    acquisitionSources,
  });
  // A refused dial the list can name is named, and no wider list is asked for.
  const roleRefusal = nameRefusal(refusedDials);
  const { dealsQuery, stageColumns, stageTotalsQuery, readError, listRead } =
    useSurfaceReads(
      dealFilters,
      stages,
      view,
      Boolean(roleRefusal),
      pipelinesQuery,
      query.perPage,
    );
  // A dial the role's mask refused, which a current snapshot would have
  // withheld. Reading /me again withholds it and the list reloads without it.
  const maskRefused = maskRefusedDialOf(readError);
  useReReadMeWhen(Boolean(maskRefused));
  const refusal = nameRefusal([...refusedDials, maskRefused]);
  // The same answer the query keys its `enabled` on, so the column explains the
  // absence by the rule that caused it.
  const totalsWithheld =
    totalsWithheldBecause(dealFilters) ??
    totalsRefusedBecause(stageTotalsQuery.data);
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  // The deal that just closed, while the review is on offer for it. Separate
  // from `pending`, which is the question before the close; this is the offer
  // after one landed, and the close dialog is already gone by then.
  const [closed, setClosed] = useState<ClosedDeal | null>(null);
  const [reopening, setReopening] = useState<PendingAdvance | null>(null);

  const advance = useAdvanceDeal();

  const stageName = new Map(stages.map((stage) => [stage.id, stage.name]));
  const loadedDeals = (dealsQuery.data?.pages ?? []).flatMap(
    (page) => page.data,
  );
  // Cleared after any bulk run except for the rows that refused, since every
  // other row's version has moved.
  const chosen = useRowSelection({
    rows: loadedDeals,
    keyOf: (deal) => deal.id,
    limit: BULK_MAX_ITEMS,
  });

  const dealsListState: ListState<Deal> = {
    rows: loadedDeals,
    query,
    setQuery,
    isPending: listRead.isPending,
    isError: listRead.isError,
    error: listRead.error,
    refetch: () => listRead.refetch(),
    hasMore: dealsQuery.hasNextPage,
    loadMore: () => {
      dealsQuery.fetchNextPage();
    },
  };

  // Open-stage targets only: a deal is born open (INV-CLOSE-PAST twin rule);
  // won/lost are reached through the confirmed advance, never at create.
  const openStages = stages.filter((stage) => stage.semantic === "open");

  const {
    openDeal,
    cardDragHandlers,
    columnDropHandlers,
    requestAdvance,
    confirmPending,
    dismissPending,
  } = useBoardInteractions({
    stages,
    loadedDeals: stageColumns.deals,
    advance,
    setPending,
    setReopening: (asked) => {
      advance.reset();
      setReopening(asked);
    },
  });

  // Shared between the board and the table surface: whichever view is
  // showing, the action lives in the surface's own header, not a wrap sibling.
  const createRefusal = useCreateRefusal(openStages.length > 0);
  const createAction = openStages.length > 0 && (
    <DealCreateAction
      pipeline={effectivePipeline}
      cf={cf}
      openStages={openStages}
      partnerOptions={partnerOptions}
      startOpen={startCreating}
      refusalId={createRefusal?.id}
    />
  );

  const pipelineId = effectivePipeline?.id ?? "";
  // The board/table switch and the pipeline picker are shared, so the reader
  // sees the same dials either way.
  const { layout, viewRows } = dealViewDials({
    view,
    setView,
    pipelines: pipelinesQuery.data ?? [],
    pipelineId,
    setPipelineId,
    setQuery,
    t,
  });
  // Saving is not shared. A view holds a sort as well as its filters. The
  // board offers no way to see or change one: its order is the pipeline's
  // stage order. Saving from there would pin an ordering the reader never
  // chose into a view they will restore on the table.
  const saveView = view === "table" && (
    <SaveViewAction
      resource="deals"
      state={{ query: savableDealQuery(dealsListState.query, pipelineId) }}
    />
  );
  const listState: ListState<Deal> = refusal
    ? {
        ...dealsListState,
        rows: [],
        isPending: false,
        isError: false,
        hasMore: false,
      }
    : dealsListState;
  const tagChips = useTagChips();
  const rowSelection = dealRowSelection({
    view,
    chosen,
    stages,
    t,
  });

  // The board is the surface's body, so the dials that narrow it stay in view.
  // Each column pages itself, so `bodyOwnsPaging` withholds the grid's pager.
  const boardBody =
    view === "board" && !refusal ? (
      <DealBoardBody
        stageColumns={stageColumns}
        pipelinesQuery={pipelinesQuery}
        effectivePipeline={effectivePipeline}
        dealFilters={dealFilters}
        stageTotalsQuery={stageTotalsQuery}
        totalsWithheld={totalsWithheld}
        companies={companiesQuery.data?.data ?? []}
        openDeal={openDeal}
        cardDragHandlers={cardDragHandlers}
        columnDropHandlers={columnDropHandlers}
        requestAdvance={requestAdvance}
      />
    ) : undefined;

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={listState}
        unit="deals.unit"
        columns={dealColumns(t, locale, readerZone, stageName)}
        rowKey={(deal) => deal.id}
        rowRoute={(deal) => ({ screen: "deals", id: deal.id })}
        refusal={createRefusal}
        action={
          <>
            <LegalReviewQueueLink />
            {createAction}
          </>
        }
        layout={layout}
        viewRows={viewRows}
        saveView={saveView}
        notice={<SavedViewsFailure resource="deals" />}
        body={boardBody}
        bodyOwnsPaging={boardBody !== undefined}
        bodyEmpty={boardIsEmpty(stageColumns)}
        countScope={effectivePipeline?.name}
        bodyCount={
          boardBody && (
            <BoardHeadCount
              pipeline={effectivePipeline}
              stageColumns={stageColumns}
              stageTotalsQuery={stageTotalsQuery}
            />
          )
        }
        // The pipeline picker is screen state, not a filter, so switching it
        // changes every row without touching `filters`. Naming it here is
        // what puts the reader back on page 1.
        scopeKey={effectivePipeline?.id ?? ""}
        dataChips={[...ownerChips, ...dealChips, ...tagChips, ...rangeChips]}
        dataViews={savedViews.map((view) => railDealView(view, pipelineId))}
        selection={rowSelection}
        chips={surfaceChips}
        views={[...standardViews(viewerId, { sort: "" })]}
        {...(refusal
          ? {
              emptyNote: (
                <RefusedFiltersNote
                  message={t("deals.filterRefused", { dial: refusal.names })}
                  dials={refusal.dials}
                  names={refusal.names}
                />
              ),
            }
          : {
              emptyLine: mineEmptyLine({
                t,
                state: dealsListState,
                viewerId,
                unit: "unit.deals",
              }),
            })}
      />
      <ErrorLine error={advanceErrorBehind(advance.error, pending !== null)} />
      <BoardReopenModal
        asked={reopening}
        onClose={() => setReopening(null)}
        openStages={openStages}
        advance={advance}
      />
      <ConfirmAdvanceModal
        pending={pending}
        onClose={dismissPending}
        onConfirm={confirmPending}
        onClosed={(deal, reason) => setClosed(closedDealOf(deal, reason))}
      />
      {/* Offered the moment a deal closes, from the list as from the record
          page: the reader who just said why is the one who can answer the
          rest. */}
      <CloseReviewOffer closed={closed} onDismiss={() => setClosed(null)} />
    </div>
  );
}
