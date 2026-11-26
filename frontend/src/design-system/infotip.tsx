// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CircleHelp, Info } from "lucide-react";
import { useTooltip } from "./tooltip";
import "./infotip.css";

/**
 * The quiet ⓘ beside a value whose reason fits one tooltip: `text` shows on
 * hover and on focus. `label` names the mark, since the glyph says nothing.
 * `mark="help"` draws a "?" instead, for what a whole section is for.
 * A longer explanation, or one with a link, is a `HelpPopover`.
 */
export function InfoTip({
  label,
  text,
  mark = "info",
}: Readonly<{ label: string; text: string; mark?: "info" | "help" }>) {
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(text);
  return (
    <button
      ref={ref}
      type="button"
      className="info-tip"
      aria-label={label}
      {...trigger}
    >
      {mark === "help" ? <CircleHelp aria-hidden /> : <Info aria-hidden />}
      {tip}
    </button>
  );
}
