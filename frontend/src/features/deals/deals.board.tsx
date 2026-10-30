import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  type ComponentProps,
  type DragEvent,
  type MouseEvent,
  useRef,
} from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { useSuggestionGhosts } from "../../components/dealsuggestion";
import { rosterOwnerNaming } from "../../components/entityref";
import type {
  BoardColumn,
  BoardDeal,
  PipelineBoard,
} from "../../design-system/pipelineboard";
import { LoadMoreButton, QueryGate } from "../../design-system/querystates";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { dealRecordKeys, dealWinKeys } from "../../lib/activitykeys";
import {
  type DealFilters,
  ownerDialParams,
  type useDeals,
  type useStageTotals,
} from "../../lib/deals.queries";
import { useMemberNames } from "../../lib/membernames";
import type { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import type { AdvanceInput, PendingAdvance } from "./deal360/confirmadvance";
import { useCompanyMarks } from "./dealcompanymarks";
import { DealPipelineBoard } from "./dealpipelineboard";
import type { Company, Deal, Pipeline, Stage } from "./deals";
import { buildColumns } from "./deals.columns";

/**
 * What a terminal advance says about how the deal closed, on top of the stage
 * and status every advance carries.
 *
 * A lost deal states its reason. A won deal states one only when there is no
 * signed contract to point at. The server looks for the contract first, and a
 * win that has one says nothing here. That keeps "won with paper" and "won
 * without it" apart in reports.
 */
function closingFields(input: AdvanceInput) {
  if (input.toStage.semantic === "lost") {
    return {
      lost_reason: input.lostReason,
      lost_reason_code: input.lostReasonCode,
    };
  }
  if (input.toStage.semantic === "won" && input.wonWithoutContractReason) {
    return {
      won_without_contract_reason: input.wonWithoutContractReason,
      won_without_contract_detail: input.wonWithoutContractDetail,
    };
  }
  return {};
}

/**
 * The one way this screen advances a deal, shared by the board's drag and the
 * record page's stepper.
 *
 * An advance is pinned like any other write: the version the reader's card or
 * record was drawn from rides the variables. When two readers move one deal at
 * once, the second reads the replaced version and fails 409 version_skew. It
 * does not undo a stage change nobody saw.
 */
export function useAdvanceDeal() {
  const toast = useToast();
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    // No single record: one instance serves every card on the board, and the
    // moving deal is known only at mutate() time. So the agent rail names the
    // write without naming a record (app/agentrail-copy.ts).
    mutationKey: ["deal-edit"],
    mutationFn: async (input: AdvanceInput) => {
      const terminal = input.toStage.semantic !== "open";
      return unwrap(
        await api.POST("/deals/{id}/advance", {
          params: {
            path: { id: input.dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: {
            to_stage_id: input.toStage.id,
            ...(terminal ? { status: input.toStage.semantic } : {}),
            ...closingFields(input),
          },
        }),
        t,
      );
    },
    onSuccess: (deal, input) => {
      // The advanced deal goes into the cache synchronously, before the
      // refetch the invalidation schedules. isPending clears the moment this
      // returns. Without this, a reader who clicks a second stage at once would
      // pin the version this write replaced and read a 409 they did not cause.
      if (deal) {
        queryClient.setQueryData(["deal", input.dealId], deal);
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
      for (const queryKey of dealRecordKeys(input.dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A win moves the deal's project into delivery in the same server
      // write, so the project page and list are stale the moment this
      // returns. Without this, a reader who follows the project chip within
      // the 30s stale window reads a won deal on a project still being
      // pursued. The server moves both in one transaction to prevent that.
      for (const queryKey of dealWinKeys(deal)) {
        queryClient.invalidateQueries({ queryKey });
      }
      toast.show(t("deals.advanced", { stage: input.toStage.name }));
    },
  });
}

// The board surface's body, drawn only while the board is showing.
//
// Its own component because the choice it makes is three deep. First a
// pipeline to draw in, then a first load that has not landed, then the board.
// Inside the screen, that choice would sit under every other dial it holds.
export function DealBoardBody({
  dealsQuery,
  pipelinesQuery,
  effectivePipeline,
  dealFilters,
  loadedDeals,
  stageTotalsQuery,
  companies,
  companiesSettled,
  openDeal,
  cardDragHandlers,
  columnDropHandlers,
  totalsWithheld,
}: Readonly<{
  dealsQuery: ReturnType<typeof useDeals>;
  pipelinesQuery: ReturnType<typeof usePipelines>;
  effectivePipeline?: Pipeline;
  dealFilters: DealFilters;
  loadedDeals: Deal[];
  stageTotalsQuery: ReturnType<typeof useStageTotals>;
  // Why no stage total is coming, or undefined when one is. Decided by
  // totalsWithheldBecause where the query's own `enabled` reads it, so the
  // column's explanation and the query's absence cannot disagree.
  totalsWithheld?: MessageKey;
  companies: Company[];
  companiesSettled: boolean;
  openDeal: ComponentProps<typeof PipelineBoard>["onOpen"];
  cardDragHandlers: ComponentProps<typeof PipelineBoard>["cardDragHandlers"];
  columnDropHandlers: ComponentProps<
    typeof PipelineBoard
  >["columnDropHandlers"];
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // Named by id, batched into one request for every card's owner at once.
  // An unowned deal contributes no id, so a board of them asks nothing.
  const ownerNames = useMemberNames(
    loadedDeals.flatMap((deal) => (deal.owner_id ? [deal.owner_id] : [])),
  );
  // Every company the cards name. The picker's capped page answers most of
  // them for free. The rest are resolved by id (useCompanyMarks), so no card
  // stands over a company the board failed to look up.
  //
  // Only the board asks. The table names its companies through the per-record
  // reference every other cross-record cell uses, and handing it this map too
  // would read each company twice.
  const companyMarks = useCompanyMarks(
    loadedDeals,
    companies,
    companiesSettled,
  );
  const suggestionGhosts = useSuggestionGhosts(
    effectivePipeline?.id,
    ownerDialParams(dealFilters.filters),
  );
  return (
    <QueryGate query={pipelinesQuery} pendingLabel={t("nav.deals")}>
      {() =>
        effectivePipeline ? (
          // Only the initial load goes through the gate. An infinite
          // query reports isError when any page fails, later ones
          // included. A gate around a loaded board would let one failed
          // "load more" throw away every card already on screen. Past
          // the first page the board stands and the button retries.
          (dealsQuery.data?.pages ?? []).length === 0 ? (
            <QueryGate query={dealsQuery} pendingLabel={t("nav.deals")}>
              {() => null}
            </QueryGate>
          ) : (
            <>
              <DealPipelineBoard
                cardHref={(deal) => routeHash({ screen: "deals", id: deal.id })}
                zone={recordZone}
                columns={buildColumns(
                  effectivePipeline.stages ?? [],
                  loadedDeals,
                  stageTotalsQuery.data ?? new Map(),
                  companyMarks,
                  totalsWithheld ? t(totalsWithheld) : undefined,
                  rosterOwnerNaming(ownerNames),
                )}
                onOpen={openDeal}
                cardDragHandlers={cardDragHandlers}
                columnDropHandlers={columnDropHandlers}
                columnExtras={suggestionGhosts}
              />
              <LoadMoreButton query={dealsQuery} />
            </>
          )
        ) : null
      }
    </QueryGate>
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
}: Readonly<{
  stages: Stage[];
  loadedDeals: Deal[];
  advance: ReturnType<typeof useAdvanceDeal>;
  setPending: (pending: PendingAdvance) => void;
}>) {
  // Which card is in flight, and which card a drag last ended on, with when.
  // A drop and a click arrive as the same event pair, so the board tells them
  // apart by time. The id travels with the timestamp, or the window would be
  // board-wide. A reader who drops one card and reaches for another would lose
  // that second click, and a link that does nothing looks broken.
  const dragging = useRef<string | null>(null);
  const lastDrop = useRef<{ dealId: string; at: number } | null>(null);

  const requestAdvance = (dealId: string, stageId: string) => {
    const toStage = stages.find((stage) => stage.id === stageId);
    if (!toStage) {
      return;
    }
    // The version the reader saw. The cards and this lookup read the one deal
    // array this render was handed. So the precondition names the row as drawn
    // on the board, not as it has become since: the claim optimistic
    // concurrency makes.
    const version = loadedDeals.find((deal) => deal.id === dealId)?.version;
    if (toStage.semantic === "open") {
      advance.mutate({ dealId, version, toStage });
    } else {
      // Terminal-stage advance is a 🟡 confirm (AC-deal-6).
      setPending({ dealId, version, toStage });
    }
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
        requestAdvance(dealId, column.stage);
      }
    },
  });

  return { openDeal, cardDragHandlers, columnDropHandlers };
}
