// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EmptyState, StatCard } from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { useT } from "../../i18n";
import { useMe } from "../../lib/session";
import type { HealthReading } from "./systemhealthreadings";

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

export type HealthSlot = Readonly<{
  key: string;
  label: string;
  reading: HealthReading;
}>;

/**
 * The page's readings in one row. Each states its answer in words and tone,
 * and says more only when the answer is not clean. A reading the reader may
 * not see has no slot; its card below says why.
 */
export function HealthStrip({
  slots,
  stamp,
}: Readonly<{ slots: readonly HealthSlot[]; stamp?: string }>) {
  const t = useT();
  const shown = slots.flatMap(({ key, label, reading }) =>
    reading.card ? [{ key, label, card: reading.card }] : [],
  );
  if (shown.length === 0) {
    return null;
  }
  return (
    <StatStrip
      label={t("systemHealth.strip")}
      density="compact"
      floor={stamp && t("systemHealth.asOf", { time: stamp })}
    >
      {shown.map(({ key, label, card }) => (
        <StatCard
          key={key}
          narrow="row"
          label={label}
          value={card.value}
          tone={card.tone}
          detail={card.detail}
          href={card.href}
        />
      ))}
    </StatStrip>
  );
}
