// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge, Disclosure } from "./atoms";
import { type Fact, FactList } from "./factlist";
import { Panel, PanelBody } from "./panel";

// What a number was drawn FROM, beside the number itself: how many records
// were eligible, how many carried the values the figure needs, and what the
// figure therefore does not cover.
//
// The gap is the point. A total over 40 of 52 deals is not wrong, but a reader
// who takes it for all 52 has been misled by a number that never said so, and
// no amount of accuracy in the arithmetic fixes that. This panel is where the
// shortfall is stated in the same glance as the result.
//
// Presentational and controlled: every word arrives through props. Three
// surfaces draw it — the forecast, the pipeline view, and the report an agent
// composes — and a receipt that spelled its own copy would drift between them,
// which for this panel means three different accounts of what was counted.
export function EvidenceReceipt({
  title,
  state,
  counts,
  calculation,
  calculationSummary,
  children,
}: Readonly<{
  title: string;
  // The one-word verdict on the inputs, drawn as a Badge beside the title.
  // Absent where the surface has nothing to claim yet — a receipt whose state
  // is still loading says nothing rather than saying "ok".
  state?: Readonly<{ label: string; tone: BadgeTone }>;
  // Eligible, priced, date-confirmed, FX missing: whatever this surface counts,
  // as label→value rows. An empty list is a receipt with nothing to report,
  // which is different from a receipt that was never drawn.
  counts: readonly Fact[];
  // How the figure was reached, folded away. Open, it is the definition behind
  // the number; closed, it is a promise that there IS one — which is why the
  // summary names the thing being explained rather than saying "details".
  calculation?: ReactNode;
  calculationSummary?: string;
  // Anything this surface adds under the counts: a withheld-prediction
  // callout, a coverage note.
  children?: ReactNode;
}>) {
  return (
    <Panel
      title={title}
      titleAction={
        state ? <Badge tone={state.tone}>{state.label}</Badge> : undefined
      }
    >
      <PanelBody>
        {/* Numeric: the counts are figures read down a column, and the mono
            face is what lets a reader compare 40 against 52 at a glance. */}
        <FactList facts={counts} numeric />
        {children}
        {/* Both or neither. A disclosure whose summary is missing has no name
            on its own control, and a summary with nothing behind it is a
            promise of an explanation that does not exist. */}
        {calculation && calculationSummary && (
          <Disclosure summary={calculationSummary}>{calculation}</Disclosure>
        )}
      </PanelBody>
    </Panel>
  );
}

// The tones a Badge draws, named here so a caller's `state` cannot widen past
// what the Badge accepts.
type BadgeTone = Parameters<typeof Badge>[0]["tone"];
