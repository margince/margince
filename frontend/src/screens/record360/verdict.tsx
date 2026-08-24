// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a 360 page: the call, and the signals it rests on.
//
// A record page is read in two ways and only one of them is reading. A rep
// working one deal reads the prose; a rep working thirty before a forecast
// call scans for the one that needs them. The prose serves the first and
// defeats the second — five headed paragraphs have no shape to scan, so
// finding the deal in trouble means reading all thirty.
//
// So the call goes on top, alone, in the one loud element on the card, and the
// signals under it each state the NUMBER that tripped them. "Going cold" is a
// label a reader has to trust; "no contact in 84 days" is the finding itself,
// and a reader can disagree with it. A chip that cannot say its number says
// its rule's own words instead — never nothing, which would read as a signal
// somebody cleared.
//
// Entity-agnostic on purpose. Company360 and Person360 answer the same
// question about a different record, and the day they grow a verdict they take
// this one rather than a second shaped like it.

import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import "./record360.css";

/**
 * How loud a standing is. The four words are the deal card's today; a record
 * kind with its own vocabulary passes its own map.
 *
 * `live` is deliberately untoned. A card where every state is coloured has no
 * colour left for the state that needs one, and a healthy record shouting is
 * how a reader learns to stop looking.
 */
export type StandingTone = "danger" | "warn" | "accent" | undefined;

/**
 * VerdictHead is the call and the one line under it.
 *
 * `standing` is an open wire string, like every other enum this product reads
 * back: a word this build does not know renders its own text rather than
 * vanishing. A missing verdict renders nothing at all — a head that said
 * "unknown" would be the card inventing a call it was not given.
 */
export function VerdictHead({
  label,
  tone,
  because,
}: Readonly<{
  label: string;
  tone: StandingTone;
  // One line saying what the call rests on. The prose underneath says the
  // rest; this is the half a scanner reads.
  because?: ReactNode;
}>) {
  return (
    <PanelBody>
      <div className="r360-verdict">
        <span className={`r360-standing r360-standing-${tone ?? "plain"}`}>
          {label}
        </span>
        {because ? <span className="r360-because">{because}</span> : null}
      </div>
    </PanelBody>
  );
}

/** One scannable finding, stating the number that tripped it. */
export type Signal = {
  // Stable across renders and unique within the strip: the chips are a set of
  // findings, and two rules can legitimately produce the same words.
  key: string;
  // What tripped, in the rule's own words.
  label: string;
  // The number behind it — "84 days", "1 of 4". Absent when the rule has no
  // figure, which is a real state and not a missing one.
  figure?: string;
  tone: StandingTone;
};

/**
 * SignalStrip renders the findings a reader scans before deciding to read.
 *
 * Renders nothing when there are none. An empty strip would be a row of
 * whitespace where findings go, which reads as a card still loading — and the
 * page has a real "nothing is wrong" sentence for that, which says it.
 */
export function SignalStrip({ signals }: Readonly<{ signals: Signal[] }>) {
  if (signals.length === 0) {
    return null;
  }
  return (
    <PanelBody>
      <ul className="r360-signals">
        {signals.map((signal) => (
          <li key={signal.key}>
            <Badge tone={signal.tone}>{signal.label}</Badge>
            {signal.figure ? (
              <span className="t-mono r360-figure">{signal.figure}</span>
            ) : null}
          </li>
        ))}
      </ul>
    </PanelBody>
  );
}
