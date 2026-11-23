// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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

/** Returns the function that renders a card's stage names through the pipelines' own translations. */
export function useReaderStageNames(): (
  change: Change | null | undefined,
) => Change | undefined {
  const pipelines = usePipelines().data;
  const names = stageNames(pipelines ?? []);
  return (change) => withReaderStageNames(change, names);
}
