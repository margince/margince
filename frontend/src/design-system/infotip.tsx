// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Info } from "lucide-react";
import { useTooltip } from "./tooltip";
import "./infotip.css";

/**
 * The quiet ⓘ beside a value whose reason fits one tooltip: `text` shows on
 * hover and on focus. `label` names the mark, since the glyph says nothing.
 * A longer explanation, or one with a link, is a `HelpPopover`.
 */
export function InfoTip({
  label,
  text,
}: Readonly<{ label: string; text: string }>) {
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(text);
  return (
    <button
      ref={ref}
      type="button"
      className="info-tip"
      aria-label={label}
      {...trigger}
    >
      <Info aria-hidden />
      {tip}
    </button>
  );
}
