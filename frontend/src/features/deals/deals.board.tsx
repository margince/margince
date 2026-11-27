import {
  type ComponentProps,
  type DragEvent,
  type MouseEvent,
  useRef,
} from "react";
import { useSuggestionGhosts } from "../../components/dealsuggestion";
import { rosterOwnerNaming } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { CountLine } from "../../design-system/listsurface";
import type {
  BoardColumn,
  BoardDeal,
  PipelineBoard,
} from "../../design-system/pipelineboard";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../../design-system/querystates";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { translatePlural, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type DealFilters,
  ownerDialParams,
  type useStageTotals,
} from "../../lib/deals.queries";
import { useMemberAvatars, useMemberNames } from "../../lib/membernames";
import {
  inPositionOrder,
  type usePipelines,
} from "../../lib/pipelines.queries";
import { routeHash } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import type { AdvanceInput, PendingAdvance } from "./deal360/confirmadvance";
import { ReopenModal } from "./deal360/reopenmodal";
import type { useAdvanceDeal } from "./dealadvance";
import type { StageColumnRead, useStageColumns } from "./dealboard.queries";
import { useCompanyMarks } from "./dealcompanymarks";
import { DealPipelineBoard } from "./dealpipelineboard";
import { probabilityReason } from "./dealprobability";
import type { Company, Deal, Pipeline, Stage } from "./deals";
import { boardDealCount, buildColumns } from "./deals.columns";

// Under a column's cards: its first read's wait or failure, then its "Load more".
// A failed later read keeps the cards and is named, with Retry once no page is left.
export function StageColumnFoot({ read }: Readonly<{ read: StageColumnRead }>) {
  const t = useT();
  return (
    <QueryStates query={read} pendingLabel={t("nav.deals")} pendingLines={3}>
      <ErrorLine
        error={read.loadMoreError}
        actions={
          !read.hasNextPage && (
            <Button variant="ghost" onClick={() => read.refetch()}>
              {t("common.retry")}
            </Button>
          )
        }
      />
      <LoadMoreButton query={read} />
    </QueryStates>
  );
}

// Stages whose first read has not answered with cards, so their count is unknown.
function uncountedStages(columns: ReadonlyMap<string, StageColumnRead>) {
  return new Set(
    [...columns].flatMap(([stageId, read]) =>
      read.isPending || read.isError ? [stageId] : [],
    ),
  );
}

// Every column read answered with no card. No column yet is a board still
// waiting on its pipeline, not an empty one.
export function boardIsEmpty(stageColumns: ReturnType<typeof useStageColumns>) {
  return (
    stageColumns.columns.size > 0 &&
    !stageColumns.isPending &&
    stageColumns.error === null &&
    stageColumns.deals.length === 0
  );
}

// The board's count beside the page's name, once its pipeline and figures are in.
// A sum of loaded cards that misses a column's deals says it counts the loaded.
export function BoardHeadCount({
  pipeline,
  stageColumns,
  stageTotalsQuery,
}: Readonly<{
  pipeline?: Pipeline;
  stageColumns: ReturnType<typeof useStageColumns>;
  stageTotalsQuery: ReturnType<typeof useStageTotals>;
}>) {
  const { locale } = useLocale();
  const totals = stageTotalsQuery.data;
  // isLoading, not isPending: a withheld report is a disabled query, pending for good.
  // An answered report counts every stage, so no column read is waited for.
  if (
    !pipeline ||
    stageTotalsQuery.isLoading ||
    (!totals && stageColumns.isPending)
  ) {
    return null;
  }
  const count = boardDealCount(
    pipeline.stages ?? [],
    totals,
    stageColumns.deals,
  );
  const loadedOnly =
    !totals &&
    [...stageColumns.columns.values()].some(
      (read) => read.hasNextPage || read.isError || read.loadMoreError !== null,
    );
  return loadedOnly ? (
    <CountLine
      first={1}
      last={count}
      total={count}
      more
      scope={pipeline.name}
    />
  ) : (
    translatePlural(locale, "deals.countInPipeline", count, {
      count: formatNumber(count, locale),
      pipeline: pipeline.name,
    })
  );
}

// The board surface's body, drawn only while the board is showing.
//
// Its own component because the choice it makes is two deep: a pipeline to
// draw in, then the board. Each column states its own wait and failure.
export function DealBoardBody({
  stageColumns,
  pipelinesQuery,
  effectivePipeline,
  dealFilters,
  stageTotalsQuery,
  companies,
  openDeal,
  cardDragHandlers,
  columnDropHandlers,
  requestAdvance,
  totalsWithheld,
}: Readonly<{
  stageColumns: ReturnType<typeof useStageColumns>;
  pipelinesQuery: ReturnType<typeof usePipelines>;
  effectivePipeline?: Pipeline;
  dealFilters: DealFilters;
  stageTotalsQuery: ReturnType<typeof useStageTotals>;
  // Why no stage total is coming, or undefined when one is. Decided by
  // totalsWithheldBecause where the query's own `enabled` reads it, so the
  // column's explanation and the query's absence cannot disagree.
  totalsWithheld?: MessageKey;
  companies: Company[];
  openDeal: ComponentProps<typeof PipelineBoard>["onOpen"];
  cardDragHandlers: ComponentProps<typeof PipelineBoard>["cardDragHandlers"];
  columnDropHandlers: ComponentProps<
    typeof PipelineBoard
  >["columnDropHandlers"];
  requestAdvance: (dealId: string, stageId: string) => Promise<boolean>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const readerZone = useViewerZone();
  const loadedDeals = stageColumns.deals;
  // A card is not where the figure changes, so its reason says no verb.
  const cardReason = (deal: Deal, stageName: string) =>
    probabilityReason(deal, {
      stageName,
      t,
      locale,
      viewerId,
      editable: false,
    });
  // Named by id, batched into one request for every card's owner at once.
  // An unowned deal contributes no id, so a board of them asks nothing.
  const ownerIds = loadedDeals.flatMap((deal) =>
    deal.owner_id ? [deal.owner_id] : [],
  );
  const ownerNames = useMemberNames(ownerIds);
  const ownerPhotos = useMemberAvatars(ownerIds);
  // Every company the cards name. The picker's capped page answers most of
  // them for free. The rest are resolved by id (useCompanyMarks), so no card
  // stands over a company the board failed to look up.
  //
  // Only the board asks. The table names its companies through the per-record
  // reference every other cross-record cell uses, and handing it this map too
  // would read each company twice.
  const companyMarks = useCompanyMarks(loadedDeals, companies);
  const suggestionGhosts = useSuggestionGhosts(
    effectivePipeline?.id,
    ownerDialParams(dealFilters.filters),
  );
  const columnExtras = (column: BoardColumn) => {
    const read = stageColumns.columns.get(column.stage);
    return (
      <>
        {read && <StageColumnFoot read={read} />}
        {suggestionGhosts(column)}
      </>
    );
  };
  return (
    <QueryGate query={pipelinesQuery} pendingLabel={t("nav.deals")}>
      {() =>
        effectivePipeline ? (
          <DealPipelineBoard
            cardHref={(deal) => routeHash({ screen: "deals", id: deal.id })}
            zone={readerZone}
            columns={buildColumns(
              effectivePipeline.stages ?? [],
              loadedDeals,
              stageTotalsQuery.data ?? new Map(),
              companyMarks,
              totalsWithheld ? t(totalsWithheld) : undefined,
              rosterOwnerNaming(ownerNames),
              uncountedStages(stageColumns.columns),
              ownerPhotos,
              cardReason,
            )}
            onOpen={openDeal}
            cardDragHandlers={cardDragHandlers}
            columnDropHandlers={columnDropHandlers}
            columnExtras={columnExtras}
            menu={{
              deals: loadedDeals,
              stages: inPositionOrder(effectivePipeline.stages ?? []),
              onAdvance: requestAdvance,
            }}
          />
        ) : null
      }
    </QueryGate>
  );
}

// The Reopen question a closed card dropped on an open column asks. It closes
// once the write settles; a refusal is named on the board behind it.
export function BoardReopenModal({
  asked,
  onClose,
  openStages,
  advance,
}: Readonly<{
  asked: PendingAdvance | null;
  onClose: () => void;
  openStages: readonly Stage[];
  advance: ReturnType<typeof useAdvanceDeal>;
}>) {
  return (
    <ReopenModal
      open={asked !== null}
      onClose={onClose}
      openStages={openStages}
      proposedStageId={asked?.toStage.id}
      pending={advance.isPending}
      onConfirm={(toStage) => {
        if (asked) {
          advance.mutate(
            { ...asked, toStage, reopens: true },
            { onSettled: onClose },
          );
        }
      }}
    />
  );
}

// Everything the board does when it is touched. That is the drag it started,
// the drop it landed, and a click that is not the tail of a drag.
//
// A hook rather than four callbacks in the screen. They share the two refs
// that tell a drag from a click, and nothing else on the screen reads those.
// Holding them here keeps the drag protocol in one place, apart from the
// screen's dials.
export function useBoardInteractions({
  stages,
  loadedDeals,
  advance,
  setPending,
  setReopening,
}: Readonly<{
  stages: Stage[];
  loadedDeals: Deal[];
  advance: ReturnType<typeof useAdvanceDeal>;
  setPending: (pending: PendingAdvance | null) => void;
  // A closed deal dropped on an open column, asked about before it reopens.
  setReopening: (reopening: PendingAdvance) => void;
}>) {
  const toast = useToast();
  const t = useT();
  // Which card is in flight, and which card a drag last ended on, with when.
  // A drop and a click arrive as the same event pair, so the board tells them
  // apart by time. The id travels with the timestamp, or the window would be
  // board-wide. A reader who drops one card and reaches for another would lose
  // that second click, and a link that does nothing looks broken.
  const dragging = useRef<string | null>(null);
  const lastDrop = useRef<{ dealId: string; at: number } | null>(null);
  // Answers the last request once it settles: saved, or refused or put away.
  const landing = useRef<((landed: boolean) => void) | null>(null);
  const settle = (landed: boolean) => {
    landing.current?.(landed);
    landing.current = null;
  };

  const requestAdvance = (dealId: string, stageId: string) => {
    settle(false);
    const toStage = stages.find((stage) => stage.id === stageId);
    // The version the reader saw. The cards and this lookup read the one deal
    // array this render was handed. So the precondition names the row as drawn
    // on the board, not as it has become since: the claim optimistic
    // concurrency makes. Text dropped on a column names no card here.
    const drawn = loadedDeals.find((deal) => deal.id === dealId);
    if (!toStage || !drawn || drawn.stage_id === stageId) {
      return Promise.resolve(false);
    }
    const version = drawn.version;
    if (drawn.status !== "open") {
      // Leaving Won or Lost clears how the deal closed, so it is the record
      // page's Reopen question. A closed deal never moves to the other close.
      if (toStage.semantic === "open") {
        setReopening({ dealId, version, toStage });
      } else {
        toast.show(t("deal.closedTakesNoStage"), { tone: "warning" });
      }
      return Promise.resolve(false);
    }
    if (toStage.semantic === "open") {
      const fromStage = stages.find((stage) => stage.id === drawn.stage_id);
      return advance.mutateAsync({ dealId, version, toStage, fromStage }).then(
        () => true,
        () => false,
      );
    }
    // Terminal-stage advance is a 🟡 confirm (AC-deal-6).
    setPending({ dealId, version, toStage });
    return new Promise<boolean>((resolve) => {
      landing.current = resolve;
    });
  };
  const confirmPending = (input: AdvanceInput) =>
    advance.mutateAsync(input).then((deal) => {
      settle(true);
      return deal;
    });
  const dismissPending = () => {
    setPending(null);
    settle(false);
  };

  // Board interactions are hoisted here so the render-prop tree below doesn't
  // nest their event callbacks past the readable depth.
  // The card is a link, so the href navigates and this does not. This handles
  // what a link cannot know: the click that ends a drag is not a click on the
  // card. Following it would open a deal the reader was only moving.
  const openDeal = (deal: BoardDeal, event: MouseEvent) => {
    const drop = lastDrop.current;
    if (drop?.dealId === deal.id && Date.now() - drop.at <= 250) {
      event.preventDefault();
    }
  };

  const cardDragHandlers = (deal: BoardDeal) => ({
    draggable: true as const,
    onDragStart: (event: DragEvent) => {
      // React bubbles a drag from the card's portalled panels here too: text
      // dragged out of the stage picker's search is not the card.
      if (
        !(event.target instanceof Node) ||
        !event.currentTarget.contains(event.target)
      ) {
        return;
      }
      dragging.current = deal.id;
      event.dataTransfer.setData("text/plain", deal.id);
    },
  });

  const columnDropHandlers = (column: BoardColumn) => ({
    onDragOver: (event: DragEvent) => {
      event.preventDefault();
      (event.currentTarget as HTMLElement).classList.add("droptarget");
    },
    onDragLeave: (event: DragEvent) => {
      (event.currentTarget as HTMLElement).classList.remove("droptarget");
    },
    onDrop: (event: DragEvent) => {
      event.preventDefault();
      (event.currentTarget as HTMLElement).classList.remove("droptarget");
      const dealId =
        event.dataTransfer.getData("text/plain") || dragging.current;
      dragging.current = null;
      if (dealId) {
        lastDrop.current = { dealId, at: Date.now() };
        void requestAdvance(dealId, column.stage);
      }
    },
  });

  return {
    openDeal,
    cardDragHandlers,
    columnDropHandlers,
    requestAdvance,
    confirmPending,
    dismissPending,
  };
}
