// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The signals a 360 page's call rests on.
//
// A record page is read in two ways and only one of them is reading. A rep
// working one deal reads the prose. A rep working thirty before a forecast
// call scans for the one that needs them. The prose serves the first and
// defeats the second. Five headed paragraphs have no shape to scan, so
// finding the deal in trouble means reading all thirty.
//
// So each signal states the NUMBER that tripped it. "Going cold" is a
// label a reader has to trust; "no contact in 84 days" is the finding itself,
// and a reader can disagree with it. A chip that cannot say its number says
// its rule's own words instead — never nothing, which would read as a signal
// somebody cleared.
//
// Entity-agnostic: every record kind that grows signals takes this
// strip rather than a second shaped like it.

import { Badge } from "../../../design-system/atoms";
import { PanelBody } from "../../../design-system/panel";
import "./record360.css";

/**
 * How loud one SIGNAL chip is, in Badge's own vocabulary: a chip is a badge
 * among badges.
 */
export type SignalTone = "danger" | "warning" | "accent" | undefined;

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
  tone: SignalTone;
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
              <span className="r360-figure">{signal.figure}</span>
            ) : null}
          </li>
        ))}
      </ul>
    </PanelBody>
  );
}
