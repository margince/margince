// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import { unwrap } from "../../../lib/problem";
import { useRecordZone } from "../../../lib/recordzone";
import {
  type DealStandingInputs,
  type StageTime,
  stageEntries,
  stageTimes,
} from "./dealstanding";

// Far more advances than a deal makes in its life, so one page is the whole
// history; `has_more` still says when it is not.
const STAGE_HISTORY_LIMIT = 100;

/**
 * The deal's stage moves, from the field history the audit log projects. The
 * read is for a signed-in person only, so for any other reader it fails and
 * the ladder draws no time per stage.
 */
export function useDealStageHistory(dealId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["deal-stage-history", dealId],
    enabled,
    queryFn: async () => {
      return unwrap(
        await api.GET("/field-history", {
          params: {
            query: {
              entity_type: "deal",
              entity_id: dealId,
              field: "stage_id",
              limit: STAGE_HISTORY_LIMIT,
            },
          },
        }),
      );
    },
  });
}

/**
 * Whole days the deal spent in each open stage it has stood in. `undefined`
 * while the history is unread or refused to this reader, so a caller never
 * mistakes "unknown" for "no days".
 */
export function useStageTimes({
  deal,
  stages,
  asOf,
}: DealStandingInputs): ReadonlyMap<string, StageTime> | undefined {
  const zone = useRecordZone();
  const page = useDealStageHistory(deal.id, true).data;
  if (!page?.data) {
    return undefined;
  }
  return stageTimes({
    stages: stages.filter((stage) => stage.semantic === "open"),
    currentId: deal.stage_id,
    entries: stageEntries(page.data, deal, !page.page?.has_more),
    asOf,
    zone,
  });
}
