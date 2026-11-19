// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { undoAction, useToast } from "../../design-system/toast";
import { useLocale, useT } from "../../i18n";
import { dealStageMoveKeys, dealWinKeys } from "../../lib/activitykeys";
import { problemFieldErrorsOf, unwrap } from "../../lib/problem";
import type { AdvanceInput } from "./deal360/confirmadvance";
import { advanceNote } from "./dealprobability";
import type { Stage } from "./deals";

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
 * The one way this screen advances a deal, shared by the board's drag, each
 * card's menu and the record page's stepper.
 *
 * An advance is pinned like any other write: the version the reader's card or
 * record was drawn from rides the variables. When two readers move one deal at
 * once, the second reads the replaced version and fails 409 version_skew. It
 * does not undo a stage change nobody saw.
 */
export function useAdvanceDeal() {
  const toast = useToast();
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const advance = useMutation({
    // No single record: one instance serves every card on the board, and the
    // moving deal is known only at mutate() time. So the agent rail names the
    // write without naming a record (app/agentrail-copy.ts).
    mutationKey: ["deal-edit"],
    mutationFn: async (input: AdvanceInput) => {
      // A reopen states its status as the record page's Reopen does.
      const statusStated = input.toStage.semantic !== "open" || input.reopens;
      return unwrap(
        await api.POST("/deals/{id}/advance", {
          params: {
            path: { id: input.dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: {
            to_stage_id: input.toStage.id,
            ...(statusStated ? { status: input.toStage.semantic } : {}),
            ...closingFields(input),
          },
        }),
        t,
      );
    },
    onError: (error) => {
      // A pipeline retired since the page drew it: read the catalog again so
      // the stages stop being offered.
      if (
        problemFieldErrorsOf(error).some((f) => f.code === "pipeline_retired")
      ) {
        queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      }
    },
    onSuccess: (deal, input) => {
      // The advanced deal goes into the cache synchronously, before the
      // refetch the invalidation schedules. isPending clears the moment this
      // returns. Without this, a reader who clicks a second stage at once would
      // pin the version this write replaced and read a 409 they did not cause.
      if (deal) {
        const { probability_set: _, ...record } = deal;
        queryClient.setQueryData(["deal", input.dealId], record);
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
      for (const queryKey of dealStageMoveKeys(input.dealId)) {
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
      const back = undoStage(input);
      toast.show(
        advanceNote(deal, input, t, locale),
        back && deal
          ? {
              action: undoAction(t("common.undo"), () => {
                advance.mutate({
                  dealId: input.dealId,
                  version: deal.version,
                  toStage: back,
                });
              }),
            }
          : undefined,
      );
    },
  });
  return advance;
}

// The stage an Undo moves the deal back to. Only a move between open stages
// is taken back this way: a close asks why, and a reopen cleared how it closed.
function undoStage(input: AdvanceInput): Stage | undefined {
  const from = input.fromStage;
  return from?.semantic === "open" &&
    input.toStage.semantic === "open" &&
    !input.reopens &&
    from.id !== input.toStage.id
    ? from
    : undefined;
}
