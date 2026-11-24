// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import {
  LIST_PAGE_SIZES,
  type ListQuery,
  listQueryFromParams,
} from "../../components/listqueryparams";
import type { DealFilters } from "../../lib/deals.queries";
import { defaultPipelineOf } from "../../lib/pipelines.queries";
import { NO_URL_PARAMS } from "../../lib/urlstate";
import { stageColumnOptions } from "./dealboard.queries";

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

/**
 * The deal list's opening state. The list reads its own fixed page, so
 * perPage is the shape's default and not a dial the footer offers.
 */
export const DEAL_LIST_OPENING: ListQuery = {
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
  perPage: LIST_PAGE_SIZES[0],
};

/** The filters the board and table read for a pipeline and a decoded query. */
export function dealFiltersOf(
  pipeline: Pipeline | undefined,
  query: ListQuery,
): DealFilters {
  return {
    pipelineId: pipeline?.id ?? "",
    // Spaces alone find nothing a reader meant to find, so they search nothing.
    q: query.q.trim(),
    sort: query.sort,
    includeArchived: query.includeArchived,
    filters: query.filters,
  };
}

/**
 * The first board column a bare `#/deals` link opens: the default pipeline's
 * earliest stage, under the key the mounted board reads it with. Undefined when
 * the catalog names no stage, so nothing unscoped is ever asked for.
 * `readerScope` is `healthScope(me)` for the reader the column is read for.
 */
export function dealsArrivalColumnOptions(
  pipelines: readonly Pipeline[],
  readerScope: string,
) {
  const pipeline = defaultPipelineOf(pipelines);
  const first = (pipeline?.stages ?? []).reduce<Stage | undefined>(
    (earliest, stage) =>
      earliest === undefined || stage.position < earliest.position
        ? stage
        : earliest,
    undefined,
  );
  if (!first) return undefined;
  const query = listQueryFromParams(NO_URL_PARAMS, DEAL_LIST_OPENING, true);
  return stageColumnOptions(
    readerScope,
    dealFiltersOf(pipeline, query),
    undefined,
    first.id,
  );
}
