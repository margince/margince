// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Children, type ReactNode } from "react";
import "./actionrow.css";

/**
 * A row of verbs with ONE call to action at its end: the secondary verbs on the
 * leading edge, the primary on the trailing one, the space between them saying
 * which is which.
 *
 * The tree already had five spellings of "some buttons in a row" —
 * `.approval-gate`, `.card-actions`, `.form-actions`, `.cell-actions`,
 * `.panel-actions` — and every one of them lays its buttons out in a single
 * flow, so a surface wanting the primary held apart from the rest re-answered
 * it with a spacer, an `auto` margin or a second nested div. This is that
 * answer once. The two groups are real elements rather than `justify-content`
 * alone because the row WRAPS: at phone width the trail drops to a line of its
 * own and has to stay on the trailing edge, which `margin-inline-start: auto`
 * gives it on both lines and `space-between` gives it on neither.
 *
 * NOT for a form's submit row (`.form-actions` — its buttons are one group and
 * the field above brings its own air) and not for a modal's footer
 * (`.modal .actions`, which `Modal` owns). Reach for it where the verbs divide:
 * a decision card's Accept against its quieter three, a panel's Save against a
 * Discard.
 *
 * No `role` and no label: the buttons name themselves, and a group wrapper
 * announcing itself would put a landmark between the reader and the verb.
 *
 * Each group is drawn only when it HAS verbs. An empty leading group is still a
 * flex item, and the row wraps: at a width that fits the primary but not the
 * primary plus a gap, a zero-width lead takes the first line and pushes the one
 * button onto a second, under a line of air holding nothing. `Children.toArray`
 * rather than `Children.count`, because a caller's `{canDiscard && <Button/>}`
 * is a child that counts and renders nothing.
 */
export function ActionRow({
  children,
  primary,
  className,
}: Readonly<{
  /** The secondary verbs, laid out on the leading edge in reading order. */
  children?: ReactNode;
  /** The one call to action, held on the trailing edge. */
  primary?: ReactNode;
  /** The host's own hook for placing the row — never for re-spacing it. */
  className?: string;
}>) {
  const secondaries = Children.toArray(children).length > 0;
  return (
    <div className={className ? `action-row ${className}` : "action-row"}>
      {secondaries ? <div className="action-row-lead">{children}</div> : null}
      {primary ? <div className="action-row-trail">{primary}</div> : null}
    </div>
  );
}
