// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EmptyState } from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate, type QueryLike, useMe } from "./common";

// The shell every Settings → System health card wears.
//
// Three cards read an operational report — background jobs, capture's judgement
// queues, and what the core refused from a connector — and each was growing its
// own copy of the same four decisions: probe /me before deciding the grant is
// absent, withhold rather than disappear, keep one card's throw inside one
// card, and date the reading only when there is one.
//
// They are not stylistic. WITHHELD RATHER THAN ABSENT: a missing card on a page
// a non-admin reaches for its other sections reads as "nothing is queued" or
// "nothing was refused", which is a claim about the installation that nobody
// made. THE PROBE ITSELF: every capability predicate reads false while /me is
// in flight, so branching on the answer alone tells every administrator the
// card is not theirs until the session lands. THE STAMP BEHIND THE GRANT: a
// cache outlives a grant, so a footer under a withheld body would date a
// reading the card is no longer showing.
export function HealthCard<Data>({
  title,
  sub,
  withheld,
  canSee,
  query,
  footer,
  children,
}: Readonly<{
  title: string;
  /** The line under the title: what this card reports. */
  sub: string;
  /** What a reader without the grant is told instead of the report. */
  withheld: string;
  /** Whether this reader holds the grant the endpoint asks for. */
  canSee: boolean;
  query: QueryLike<Data>;
  /** The card's trailing fact — when the report was read. */
  footer: (data: Data) => ReactNode;
  children: (data: Data) => ReactNode;
}>) {
  const me = useMe();

  let body: ReactNode;
  if (!canSee) {
    // Behind the probe, so the notice states a settled denial rather than the
    // absence of an answer: while /me is in flight nobody holds any role yet.
    body = (
      <QueryGate query={me} pendingLabel={title}>
        {() => <EmptyState>{withheld}</EmptyState>}
      </QueryGate>
    );
  } else {
    // No `empty` predicate on the gate: on these cards an empty report is a
    // FINDING — the queue is idle, nothing was refused — and the generic copy
    // would understate the one thing the card exists to say, so the body owns
    // that rung.
    body = (
      <QueryGate query={query} pendingLabel={title}>
        {children}
      </QueryGate>
    );
  }

  const report = canSee ? query.data : undefined;

  // No bottom margin: `.settings-stack` owns the gap between cards, and a card
  // that adds its own gets two.
  return (
    <Panel title={title} footer={report && footer(report)}>
      <PanelBody>
        <PanelIntro>{sub}</PanelIntro>
        {/* One card's throw stays inside one card. These bodies derive every
            line from a payload a background system writes, so they have more
            ways to give out than the panels beside them — and without a
            boundary the whole Maintenance tab, navigation rail included, goes
            with them. */}
        <CardBoundary>{body}</CardBoundary>
      </PanelBody>
    </Panel>
  );
}
