// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EmptyState } from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { useMe } from "../../lib/session";

// Withheld, not absent: a missing card reads as "nothing is queued". The /me
// probe comes first because every grant reads false while /me is in flight.
export function HealthCard<Data>({
  title,
  sub,
  withheld,
  canSee,
  query,
  lead,
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
  /** What the report says first, in the intro's own band. */
  lead?: (data: Data) => ReactNode;
  children: (data: Data) => ReactNode;
}>) {
  const me = useMe();
  const report = canSee ? query.data : undefined;

  let opening: ReactNode = null;
  if (!canSee) {
    // Behind the probe, so the notice states a settled denial rather than the
    // absence of an answer: while /me is in flight nobody holds any role yet.
    opening = (
      <QueryGate query={me} pendingLabel={title}>
        {() => <EmptyState>{withheld}</EmptyState>}
      </QueryGate>
    );
  } else if (report !== undefined && lead) {
    opening = lead(report);
  }

  // The body stands straight in the panel so a report's table bleeds; each
  // body wraps its own prose in a `PanelBody`.
  return (
    <Panel title={title}>
      <PanelBody>
        <PanelIntro>{sub}</PanelIntro>
        <CardBoundary>{opening}</CardBoundary>
      </PanelBody>
      {/* One card's throw stays inside one card: a body derived from a payload
          a background system writes must not take the Maintenance tab with it.
          No `empty` predicate on the gate: an empty report is a FINDING the
          body states in its own words. */}
      {canSee && (
        <CardBoundary>
          <QueryGate query={query} pendingLabel={title}>
            {children}
          </QueryGate>
        </CardBoundary>
      )}
    </Panel>
  );
}
