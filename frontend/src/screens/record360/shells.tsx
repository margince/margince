// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The section shells every record page draws its cards in.
//
// Both know the difference between "there is nothing here" and "you may not
// read this", which is the distinction that makes a 360 page honest: a section
// the caller's role cannot read is ABSENT from the payload and named in
// `sections_omitted`, so the card says "hidden from you" instead of drawing an
// empty list that reads as "there is none".

import type { ReactNode } from "react";
import { Card, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type SectionDetail,
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { useT } from "../../i18n";
// SectionCard renders `.co-card`, which is defined in
// company360.css. Imported HERE rather than left to the caller: it worked
// only because both callers happened to be on the company page, and a third
// one anywhere else would have rendered unstyled. The classes keep their `co-`
// names for the reason README.md gives — renaming them reaches four
// stylesheets and every page that overrides one.
import "../company360.css";

/** A card that renders one 360 section in whichever of its states it is in. */
export function SectionCard({
  title,
  state,
  emptyLabel,
  detail,
  footer,
  actions,
  children,
}: Readonly<{
  title: string;
  state: SectionState;
  emptyLabel: string;
  detail?: SectionDetail;
  footer?: ReactNode;
  // Verbs that CHANGE this section, under everything that describes it.
  //
  // They render whenever the section is present — including when it is empty,
  // which is the state a create verb most belongs to. They do NOT render on a
  // withheld or unavailable section: a caller who may not read the deals has
  // no business being offered a button to add one, and a section that failed
  // to load cannot say whether the write would even make sense.
  actions?: ReactNode;
  children: ReactNode;
}>) {
  // `stale` and `partial` both carry real rows, so the footer figures and the
  // verbs that change the section belong with them — a truncated deal list is
  // still a deal list you can add to.
  const present =
    state === "ready" ||
    state === "empty" ||
    state === "stale" ||
    state === "partial";
  return (
    <Card className="co-card" title={title}>
      <SurfaceState state={state} emptyLabel={emptyLabel} detail={detail}>
        {children}
      </SurfaceState>
      {present && footer}
      {present && actions && <div className="card-actions">{actions}</div>}
    </Card>
  );
}

/**
 * RailPanel is SectionCard's four-state discipline rendered through Panel's
 * chrome — a fixed-height header and full-bleed rows — instead of the
 * negative-margin CSS breakout that shape used to need. The message states
 * (empty, withheld, unavailable, loading, failed) reuse SurfaceState verbatim,
 * padded in a PanelBody; `ready` is left to the caller, so rows passed as
 * children run edge to edge the way Panel is built to take them.
 *
 * Scoped to the rail's own cards — SectionCard itself is untouched, because
 * its other callers (the grid, the other tabs) are not this card's chrome.
 */
export function RailPanel({
  title,
  state,
  emptyLabel,
  detail,
  footer,
  children,
}: Readonly<{
  title: string;
  state: SectionState;
  emptyLabel: string;
  detail?: SectionDetail;
  // A figure belonging to the whole card rather than to one row. Shown only
  // on `ready`/`empty` — the states RailPanel's callers ever reach — because a
  // withheld or unavailable section has no figure to report either.
  footer?: ReactNode;
  children: ReactNode;
}>) {
  const present = state === "ready" || state === "empty";
  return (
    <Panel title={title} footer={present ? footer : undefined}>
      {state === "ready" ? (
        children
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={emptyLabel} detail={detail}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

/**
 * OverlayFallback replaces a 360 page when the workspace reads from an
 * incumbent mirror. The 360 read answers a refusal to assemble rather than a
 * failure, so the page falls back instead of showing an error.
 */
export function OverlayFallback() {
  const t = useT();
  return <EmptyState>{t("co.overlayFallback")}</EmptyState>;
}

/**
 * incompleteGraph says the connection graph a page read is not the whole one:
 * it capped its contact ring, or it withheld groups the caller may not read.
 * Either way the routes below it are a subset, and both the empty answer and
 * the found-someone answer have to say so.
 */
export function incompleteGraph(graph: {
  groups_omitted?: unknown[];
  dropped_count?: number;
}): boolean {
  return (
    (graph.groups_omitted?.length ?? 0) > 0 || (graph.dropped_count ?? 0) > 0
  );
}
