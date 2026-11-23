// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo } from "react";
import { translate, useLocale } from "../i18n";
import { stageNames, usePipelines } from "./pipelines.queries";

type Change = Readonly<Record<string, unknown>>;

function nameOf(
  names: ReadonlyMap<string, string>,
  id: unknown,
  stored: unknown,
): unknown {
  return (typeof id === "string" ? names.get(id) : undefined) ?? stored;
}

/**
 * A proposal-stage change with its stage names in the reader's language. The
 * ids decide the lookup; a card staged before the ids were carried, or on a
 * retired stage, keeps the name stored when it was raised.
 */
export function withReaderStageNames(
  change: Change | null | undefined,
  names: ReadonlyMap<string, string>,
): Change | undefined {
  if (!change) return undefined;
  const skippedIds = change.skipped_stage_ids;
  const skippedNames = change.skipped_stage_names;
  return {
    ...change,
    to_stage_name: nameOf(names, change.to_stage_id, change.to_stage_name),
    ...(Array.isArray(skippedIds) && Array.isArray(skippedNames)
      ? {
          skipped_stage_names: skippedNames.map((stored, index) =>
            nameOf(names, skippedIds[index], stored),
          ),
        }
      : {}),
  };
}

type Localizable = Readonly<{
  kind: string;
  proposed_change?: Change | null;
  summary?: string | null;
}>;

/**
 * The approval as this reader should see it. A proposal-stage card gets its
 * stage names translated, and its headline built here, because the server
 * sentence is frozen with the name stored when it was raised and every surface
 * prints that sentence first.
 */
export function useReaderApproval(): <A extends Localizable>(approval: A) => A {
  const pipelines = usePipelines().data;
  const { locale } = useLocale();
  return useMemo(() => {
    const names = stageNames(pipelines ?? []);
    return <A extends Localizable>(approval: A): A => {
      if (approval.kind !== "deal_proposal_stage") return approval;
      const change = withReaderStageNames(approval.proposed_change, names);
      const { deal_name: deal, to_stage_name: stage } = change ?? {};
      const headline =
        typeof deal === "string" && typeof stage === "string"
          ? {
              summary: translate(locale, "approval.proposalStageAsked", {
                deal,
                stage,
              }),
              system_display: undefined,
              summary_parts: undefined,
            }
          : {};
      return { ...approval, ...headline, proposed_change: change };
    };
  }, [pipelines, locale]);
}
