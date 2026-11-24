// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail, Phone } from "lucide-react";
import type { ReactNode } from "react";
import type { DetailRow } from "./recorddetails";
import { useTruncationTooltipAround } from "./tooltip";
import "./reachcard.css";

/**
 * Every way to reach a record, as one card in its details. Each address or
 * number is a line led by its glyph, with its kind at the card's edge.
 */
export function ReachCard({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="reachcard">{children}</div>;
}

/**
 * One address or number. `kind` picks the glyph, `children` is the handle
 * itself (its link, copy and chat), `caption` its kind ("Work") and `mark` a
 * mark that follows the caption. A handle too long for the line ends in "…",
 * and `text` shows it whole in a tip.
 */
export function ReachLine({
  kind,
  text,
  children,
  caption,
  mark,
}: Readonly<{
  kind: "email" | "phone";
  text: string;
  children: ReactNode;
  caption?: ReactNode;
  mark?: ReactNode;
}>) {
  const Glyph = kind === "email" ? Mail : Phone;
  const { ref, trigger, tip } =
    useTruncationTooltipAround<HTMLSpanElement>(text);
  return (
    <span className="reachline">
      <Glyph aria-hidden="true" className="reachline-glyph" />
      <span className="reachline-handle" ref={ref} {...trigger}>
        {children}
        {tip}
      </span>
      {caption && <span className="reachline-kind t-caption">{caption}</span>}
      {mark}
    </span>
  );
}

/**
 * The details row that holds the card: the term is read out but not drawn,
 * because each line's glyph already says what it is.
 */
export function reachRow(term: string, card: ReactNode): DetailRow {
  return { key: "reach", term, termHidden: true, block: true, value: card };
}
