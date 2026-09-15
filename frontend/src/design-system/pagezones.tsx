// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import { usePresence } from "./presence";
import "./pagezones.css";

// PageZones is the page-level column layout: one work column, and up to two
// rails of context beside it.
//
// It carries the grid and nothing else — no name, no mark, no badge, no tabs.
// Those belong to whatever composes it (`RecordView` for a record page), which
// is what makes the shape available to a page that is not a record: the Brief
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
 *
 * A shape does not change when the details pane folds: `asideOpen` travels the
 * aside's TRACK to zero and leaves the named areas where they are, because two
 * different templates do not interpolate and the column would jump.
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
  asideOpen = true,
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
  // Whether that right rail STANDS OPEN. A page with a details toggle hands
  // its pane over whether or not it is showing and says so here, rather than
  // handing `aside` over only while open: a column has to still BE a column on
  // its way out, or there is nothing for the track to travel from. The node is
  // only ever mounted while the pane is open or leaving, so a closed pane
  // costs its content nothing. Absent means open, which is what a page with no
  // such toggle means.
  asideOpen?: boolean;
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
  const asideColumn = useRef<HTMLElement>(null);
  // Held past `asideOpen` going false for exactly as long as the fold takes:
  // the landmark has to be there while the track travels, and gone the moment
  // it stops, or a screen-reader user is left a region nobody can see.
  const asidePresence = usePresence({ open: asideOpen, element: asideColumn });
  const armed = useArmedAfterMount();
  return (
    <div
      className={cls(
        shape === "single" ? undefined : gridClass(shape),
        className,
      )}
      // Only a page that HAS a pane declares this: on a `rail` or `single`
      // shape there is no track to fold and no gutter to hand back, and a
      // closed state there would take the gutter off a work column that never
      // had a pane beside it.
      data-aside={aside ? (asideOpen ? "open" : "closed") : undefined}
      data-motion={armed ? "armed" : undefined}
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
      {aside && asidePresence.mounted && (
        <aside
          ref={asideColumn}
          className={cls("page-zones-aside-column", asideClassName)}
          aria-label={asideLabel}
          data-state={asidePresence.state}
          // A column on its way out takes no focus and answers no pointer: it
          // is already gone as far as the reader who closed it is concerned,
          // and focus landing inside a pane that is one frame from unmounting
          // is focus dropped to the top of the document when it goes.
          inert={asidePresence.state === "closing"}
        >
          {aside}
        </aside>
      )}
    </div>
  );
}

/**
 * Whether the column transition is armed yet — false for the first frame, so a
 * page that LOADS with its pane open, or with it folded, draws that state
 * rather than animating into it.
 *
 * One frame rather than none, because the pane's state does not arrive with
 * the first render: `usePageAside` claims the pane in an effect, so the screen
 * re-renders with the remembered answer immediately after mount. React flushes
 * that update before the frame callback below runs, which is what makes this
 * the arming point rather than a race with it.
 */
function useArmedAfterMount(): boolean {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    const frame = requestAnimationFrame(() => setArmed(true));
    return () => cancelAnimationFrame(frame);
  }, []);
  return armed;
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
