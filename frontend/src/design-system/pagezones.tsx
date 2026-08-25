// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./pagezones.css";

// PageZones is the page-level column layout: one work column, and up to two
// rails of context beside it.
//
// It carries the grid and nothing else — no name, no mark, no badge, no tabs.
// Those belong to whatever composes it (`RecordView` for a record page), which
// is what makes the shape available to a page that is not a record: the Home
// screen's work column with its context rail is the same layout as a company's,
// and a second stylesheet spelling the same three ratios and the same two folds
// would drift the first time either moved.

/**
 * Which columns the page has, and therefore which grid it draws.
 *
 * `single` is not a one-column grid — it is NO grid: the work column is the
 * page's only block and wrapping it in a one-track grid would add a rhythm
 * nothing needs. A shape names the TEMPLATE; the slots below carry the
 * content, and a caller keeps the two in agreement (`rail` with no rail slot
 * draws an empty track, because a grid track does not collapse when its item
 * is missing — which is the whole reason the shape is explicit).
 */
export type PageZonesShape = "single" | "rail" | "aside" | "both";

export function PageZones({
  shape,
  main,
  mainClassName,
  rail,
  railLabel,
  railClassName,
  aside,
  asideLabel,
  asideClassName,
  className,
}: Readonly<{
  shape: PageZonesShape;
  // The work column: what is happening, and what the reader came to act on.
  // Required, because a page with only rails is a page with no subject.
  main: ReactNode;
  // Classes for the work column, on top of the one this component needs to
  // place it in the grid. For the composing page's own rhythm —
  // `RecordView` marks it as an arrival stack so its blocks fade in one after
  // the next rather than as one plate.
  mainClassName?: string;
  // The left rail: what this page's subject IS. Absent leaves the column out
  // of the DOM entirely rather than rendering an empty landmark.
  rail?: ReactNode;
  // The rail's accessible name. Required in practice and optional in the type
  // for one reason: no copy lives in a primitive, so the caller translates it
  // and the default belongs to the caller too. An `<aside>` with no name is a
  // landmark a screen-reader user can reach and cannot identify — pass one.
  railLabel?: string;
  // Classes for the rail column. The grid decides the column's SHARE; what
  // stacks inside it is the composing page's rhythm — a record page's rail
  // packs its cards tighter than its work column, and that is a decision about
  // its cards rather than about the layout.
  railClassName?: string;
  // The right rail: the business around the subject. Same absent-vs-empty rule
  // as `rail`.
  aside?: ReactNode;
  // The aside's accessible name, on the same rule as `railLabel`. Two
  // landmarks sharing one name is a dead end for anyone moving between them,
  // so a page whose aside holds something other than context names it.
  asideLabel?: string;
  // Classes for the aside column, on the same rule as `railClassName`.
  asideClassName?: string;
  // Classes for the grid container itself — the composing page's own concerns
  // about the block as a whole (an arrival stack, a page-specific bottom
  // clearance), which are not this component's to decide.
  className?: string;
}>) {
  return (
    <div
      className={cls(
        shape === "single" ? undefined : gridClass(shape),
        className,
      )}
    >
      {/* The work column is FIRST here at every width, and the rails follow it
          in the order they read, because this order IS the order a screen
          reader and the tab key take (WCAG 2.2 §1.3.2, meaningful sequence). A
          grid draws its columns wherever the template puts them, so the left
          rail still appears to the LEFT of this (pagezones.css) — and that is
          the only place the two orders may differ. Nothing here may be
          reordered to fold: a rail the eye meets second and the keyboard meets
          first is a reader sent through the firmographics to reach the record
          they opened. */}
      <div className={cls("page-zones-main", mainClassName)}>{main}</div>
      {rail && (
        <aside
          className={cls("page-zones-rail-column", railClassName)}
          aria-label={railLabel}
        >
          {rail}
        </aside>
      )}
      {aside && (
        <aside
          className={cls("page-zones-aside-column", asideClassName)}
          aria-label={asideLabel}
        >
          {aside}
        </aside>
      )}
    </div>
  );
}

function gridClass(shape: Exclude<PageZonesShape, "single">): string {
  return `page-zones page-zones-${shape}`;
}

/** Joins the classes that are actually present, or nothing at all — an empty
 *  `class=""` on a column that carries no styling is noise in the DOM. */
function cls(...parts: readonly (string | undefined)[]): string | undefined {
  const kept = parts.filter((part): part is string => Boolean(part));
  return kept.length > 0 ? kept.join(" ") : undefined;
}
