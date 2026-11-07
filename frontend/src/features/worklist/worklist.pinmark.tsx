// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pin } from "lucide-react";
import { useTooltip } from "../../design-system/tooltip";
import { useT } from "../../i18n";
import type { WorklistItem } from "./worklist.queries";

/** Whether the reader pinned this row: the row's own `pinned` reason. */
export function isPinned(item: WorklistItem): boolean {
  return (item.because ?? []).some((why) => why.kind === "pinned");
}

/**
 * The pin on a row the reader pinned, beside the row's name. The order alone
 * does not say it: a new pin or a change in the ranking moves the row.
 */
export function PinnedMark() {
  const t = useT();
  const label = t("worklist.pinnedMark");
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(label);
  // The label already names the mark, so the tip adds no description.
  const { "aria-describedby": _spoken, ...onHover } = trigger;
  return (
    <span
      ref={ref}
      {...onHover}
      role="img"
      aria-label={label}
      className="worklist-row-pinned"
    >
      <Pin size={14} aria-hidden />
      {tip}
    </span>
  );
}
