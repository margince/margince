// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { EmailEntry } from "./emailentry";
import { MoveFlag, type TimelineEntry } from "./timelineentry";

// The thread card's chip and the message row's label are one answer: the
// server's move for the message. The direction says who wrote it, which is not
// whose turn it is, so every direction is drawn against every move.

type EmailSummary = components["schemas"]["EmailSummary"];
type Move = EmailSummary["move"];

const MOVES: readonly Move[] = ["needs_reply", "waiting_for_them", "none"];
const DIRECTIONS = ["inbound", "outbound"] as const;
const LABEL: Record<Move, string | null> = {
  needs_reply: en["email.move.needsReply"],
  waiting_for_them: en["email.move.waitingForThem"],
  none: null,
};

function summaryOf(
  move: Move,
  direction: (typeof DIRECTIONS)[number],
  withheld = false,
): EmailSummary {
  return {
    activity_id: "11111111-1111-4111-8111-111111111111",
    occurred_at: "2026-09-01T09:12:00Z",
    display_status: withheld ? "withheld" : "team",
    attachment_count: 0,
    move,
    version: 1,
    subject: "Kickoff",
    direction,
  };
}

function entryOf(
  summary: EmailSummary,
  kind: TimelineEntry["kind"] = "email",
): TimelineEntry {
  return {
    id: summary.activity_id,
    kind,
    title: "Kickoff",
    atIso: summary.occurred_at,
    provenance: { kind: "human", self: false },
    direction: summary.direction ?? undefined,
    withheld: summary.display_status === "withheld",
    emailSummary: kind === "email" ? summary : undefined,
  };
}

function cardLabel(entry: TimelineEntry): string | null {
  const { container } = render(
    <LocaleProvider initial="en">
      <MoveFlag entry={entry} />
    </LocaleProvider>,
  );
  const text = container.textContent;
  cleanup();
  return text === "" ? null : text;
}

function rowShows(summary: EmailSummary, label: string): boolean {
  render(
    <LocaleProvider initial="en">
      <EmailEntry summary={summary} whyNotOpenable="noDetail" />
    </LocaleProvider>,
  );
  const shown = screen.queryByText(label) !== null;
  cleanup();
  return shown;
}

afterEach(cleanup);

describe("the thread card and the message row read one move", () => {
  for (const move of MOVES) {
    for (const direction of DIRECTIONS) {
      it(`${direction} with move ${move}`, () => {
        const summary = summaryOf(move, direction);
        const want = LABEL[move];
        expect(cardLabel(entryOf(summary))).toBe(want);
        for (const label of Object.values(LABEL)) {
          if (label) {
            expect(rowShows(summary, label)).toBe(label === want);
          }
        }
      });
    }
  }

  it("draws no move on a colleague's invitation the server says needs nothing", () => {
    expect(cardLabel(entryOf(summaryOf("none", "inbound")))).toBeNull();
  });

  it("draws no move on a withheld message, whatever the server's move", () => {
    const summary = summaryOf("needs_reply", "inbound", true);
    expect(cardLabel(entryOf(summary))).toBeNull();
    expect(rowShows(summary, en["email.move.needsReply"])).toBe(false);
  });

  it("draws no move on a chat row in either direction", () => {
    for (const direction of DIRECTIONS) {
      expect(
        cardLabel(entryOf(summaryOf("none", direction), "message")),
      ).toBeNull();
    }
  });
});
