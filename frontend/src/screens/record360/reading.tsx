// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// ONE READING, IN PARTS — the shape every record page reads in.
//
// The same reading in the same order on an account, a contact, a lead and a
// deal: the call the machine reached, with the thread it was read from; what
// needs a person today; and, under them, the two reference sections a reader
// consults rather than reads, side by side. The parts are cards; what binds
// them is the interval, not a box — a bordered container holding bordered
// cards is a card inside a card.
//
// It came out of the company page, where it was three screen classes and a
// Panel spelled inline. A reader who works a deal and then a contact should
// meet the same shape, and three pages spelling it themselves is how one of
// them came to open with a stage bar instead of a call.

import type { ReactNode } from "react";
import { Panel } from "../../design-system/panel";
import { BriefTitle } from "./brieftitle";
import { type Grounding, type StandingTone, VerdictHead } from "./verdict";
// The group's own rules — `.co-reading`, `.co-reading-pair`,
// `.co-reading-call` — live in company360.css, for the reason README.md gives:
// renaming them reaches four stylesheets. Imported HERE rather than left to
// the caller, so the second page to mount the reading does not render it
// unstyled.
import "../company360.css";

/** RecordReading is the group: the parts, in the page's own rhythm. */
export function RecordReading({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="co-reading">{children}</div>;
}

/**
 * RecordReadingPair is two reference sections side by side, at half the
 * measure each: a reader consults them rather than reads them through. One
 * column as soon as two would not fit, since a list squeezed to nothing is
 * worse than a fold.
 */
export function RecordReadingPair({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <div className="co-reading-pair">{children}</div>;
}

/**
 * CallCard is THE CALL: the head whose mark says a machine read this record,
 * the standing it reached with the sentence it rests on, and under them
 * whatever the call was read from — the record's thread, its findings.
 *
 * Its own card rather than a section of the day's work, because a verdict and
 * a to-do list are two different things to a reader who is scanning: one is
 * the record's state and the other is a queue, and sharing a box they read as
 * one continuous section where the state happens to come first.
 *
 * `standing` is absent while the read that produces it is in flight or when
 * the record has no verdict: a head holding a spinner where the call goes is
 * the reading claiming to have reached one. The caller renders what belongs in
 * that state as children.
 */
export function CallCard({
  name,
  standing,
  because,
  restsOn,
  footer,
  children,
}: Readonly<{
  // What the reading is a reading OF — the record's own name.
  name?: string;
  standing?: { label: string; tone: StandingTone };
  // One line saying what the call rests on: the half a scanner reads.
  because?: ReactNode;
  // The readings behind the call, one disclosure away.
  restsOn?: readonly Grounding[];
  footer?: ReactNode;
  children?: ReactNode;
}>) {
  return (
    <Panel
      tone="ai"
      className="co-reading-call"
      title={<BriefTitle name={name} />}
      footer={footer}
    >
      {standing ? (
        <VerdictHead
          label={standing.label}
          tone={standing.tone}
          because={because}
          restsOn={restsOn}
        />
      ) : null}
      {children}
    </Panel>
  );
}
