// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { browserZone } from "../../format/timezone";
import type { Locale, Translator } from "../../i18n";
import { translate } from "../../i18n";
import {
  cardLine,
  consequenceLine,
  happenedLines,
  todoLine,
} from "../../lib/worklisthappened";
import { row } from "./worklist.fixtures";

const t: Translator = (key, params) => translate("en", key, params);

// The runner's own zone, and every fact at midday UTC, so each falls on the
// same day east or west of UTC. The reader's now is in the same year.
const clock = {
  locale: "en" as Locale,
  zone: browserZone(),
  recordZone: browserZone(),
  now: new Date("2026-11-10T12:00:00Z"),
};

const SONYA = {
  id: "0198c4f1-2b6a-7c3d-9e0f-00000000501a",
  label: "Sonya Beck",
};

describe("what happened behind a row", () => {
  it("names who wrote and when on a waiting message", () => {
    const told = happenedLines(
      row({
        source: "customer_waiting",
        occurred_at: "2026-11-07T12:00:00Z",
        contact: SONYA,
      }),
      t,
      clock,
    );
    expect(told.told).toBe(true);
    expect(told.lines).toEqual([
      "Sonya Beck wrote on 7 Nov, and nobody has answered yet.",
    ]);
  });

  it("says the event without a name when the row has none", () => {
    expect(
      happenedLines(
        row({ source: "bounce", occurred_at: "2026-11-07T12:00:00Z" }),
        t,
        clock,
      ).lines,
    ).toEqual(["An email bounced on 7 Nov."]);
  });

  it("leaves out an event whose moment is missing", () => {
    const told = happenedLines(
      row({ source: "customer_waiting", contact: SONYA }),
      t,
      clock,
    );
    expect(told).toEqual({ lines: [], told: false });
  });

  it("tells an open task's due day and that it is late", () => {
    expect(
      happenedLines(
        row({ due_at: "2026-11-09T12:00:00Z", overdue: true }),
        t,
        clock,
      ).lines,
    ).toEqual(["It was due on 9 Nov and is still open."]);
  });

  it("names a stopped automation by the condition it reports", () => {
    expect(
      happenedLines(
        row({
          source: "automation_run",
          occurred_at: "2026-11-09T12:00:00Z",
          cause_label: "Welcome sequence",
        }),
        t,
        clock,
      ).lines,
    ).toEqual(["The automation Welcome sequence stopped on 9 Nov."]);
  });

  it("tells a silent connection from one that stopped syncing, mail from calendar", () => {
    const capture = (kind: string, medium: "mailbox" | "calendar") =>
      happenedLines(
        row({
          source: "capture_health",
          kind,
          occurred_at: "2026-11-09T12:00:00Z",
          cause_label: "anna@example.com",
          connection: { provider: "gmail", medium },
        }),
        t,
        clock,
      ).lines;
    expect(capture("capture_silent", "mailbox")).toEqual([
      "The mailbox anna@example.com has captured no mail since 9 Nov.",
    ]);
    expect(capture("sync_failing", "mailbox")).toEqual([
      "The mailbox anna@example.com has not synced since 9 Nov.",
    ]);
    expect(capture("sync_failing", "calendar")).toEqual([
      "The calendar anna@example.com has not synced since 9 Nov.",
    ]);
    expect(capture("capture_silent", "calendar")).toEqual([
      "The calendar anna@example.com has captured no meetings since 9 Nov.",
    ]);
  });

  it("adds the last contact each way, and none a reader cannot open", () => {
    const lines = happenedLines(
      row({
        source: "relationship_decay",
        occurred_at: "2026-09-01T12:00:00Z",
        contact: {
          ...SONYA,
          touch: {
            last_inbound_at: "2026-09-01T12:00:00Z",
            last_outbound_at: null,
          },
        },
      }),
      t,
      clock,
    ).lines;
    expect(lines).toEqual([
      "The last recorded contact with Sonya Beck was on 1 Sept.",
      "Last contact from Sonya Beck: 1 Sept.",
      "Last contact to Sonya Beck: none you can open.",
    ]);
  });

  it("gives a deal's own figures under its name", () => {
    const lines = happenedLines(
      row({
        source: "deal_at_risk",
        subject: { type: "deal", id: SONYA.id, label: "Depot rollout" },
        deal: { amount_minor: 5_800_000, currency: "EUR" },
      }),
      t,
      clock,
    ).lines;
    expect(lines).toEqual(["Depot rollout: €58,000.00."]);
  });
});

describe("what waiting costs", () => {
  it("names the contact who keeps waiting", () => {
    expect(
      consequenceLine(row({ consequence: "buyer_waits", contact: SONYA }), t),
    ).toBe("Sonya Beck keeps waiting for an answer.");
  });

  it("names the deal that drifts, never the contact on it", () => {
    expect(
      consequenceLine(
        row({
          consequence: "deal_drifts",
          contact: SONYA,
          subject: { type: "deal", id: SONYA.id, label: "Depot rollout" },
        }),
        t,
      ),
    ).toBe("Depot rollout keeps drifting without a next step.");
  });

  it("names the contact a task is with", () => {
    expect(
      consequenceLine(row({ consequence: "task_slips", contact: SONYA }), t),
    ).toBe("The task with Sonya Beck slips further past its date.");
  });

  it("says nothing when nothing follows, or when this build has no words", () => {
    expect(consequenceLine(row({ consequence: "none" }), t)).toBeNull();
    expect(
      consequenceLine(
        Object.assign(row(), { consequence: "a_newer_consequence" }),
        t,
      ),
    ).toBeNull();
  });
});

describe("what a told row asks", () => {
  it("names who to answer, and drops the half the event already said", () => {
    expect(
      todoLine(row({ source: "customer_waiting", contact: SONYA }), t, true),
    ).toBe("Reply to Sonya Beck.");
    expect(todoLine(row({ source: "customer_waiting" }), t, true)).toBe(
      "Reply to the message.",
    );
  });

  it("asks to check a calendar's connection for its meetings, not its mail", () => {
    const calendar = row({
      source: "capture_health",
      kind: "sync_failing",
      cause_label: "anna@example.com",
      connection: { provider: "gcal", medium: "calendar" },
    });
    expect(todoLine(calendar, t, true)).toBe(
      "Check the connection of anna@example.com so its meetings are captured again.",
    );
  });

  it("names the deal on a deal at risk, never the contact on it", () => {
    expect(
      todoLine(
        row({
          source: "deal_at_risk",
          contact: SONYA,
          subject: { type: "deal", id: SONYA.id, label: "Depot rollout" },
        }),
        t,
        true,
      ),
    ).toBe("Open Depot rollout and plan the next step.");
  });

  it("keeps the whole ask when the event is untold or is an invitation", () => {
    expect(todoLine(row({ source: "customer_waiting" }), t, false)).toBe(
      translate("en", "worklist.ask.customer_waiting"),
    );
    expect(
      todoLine(
        Object.assign(row({ source: "customer_waiting" }), {
          email_summary: { calendar_invitation: true },
        }),
        t,
        true,
      ),
    ).toBe(translate("en", "worklist.ask.invitation"));
  });

  it("keeps the whole ask for a source with no shorter one", () => {
    expect(todoLine(row({ source: "dedupe_candidate" }), t, true)).toBe(
      translate("en", "worklist.ask.dedupe_candidate"),
    );
  });
});

describe("a card's line", () => {
  it("says the event, which the title and buttons act on", () => {
    expect(
      cardLine(
        row({
          source: "customer_waiting",
          occurred_at: "2026-11-07T12:00:00Z",
          contact: SONYA,
        }),
        t,
        clock,
      ),
    ).toBe("Sonya Beck wrote on 7 Nov, and nobody has answered yet.");
  });

  it("adds the last contact each way on a task", () => {
    expect(
      cardLine(
        row({
          due_at: "2026-11-09T12:00:00Z",
          overdue: true,
          contact: {
            ...SONYA,
            touch: {
              last_inbound_at: "2026-11-08T12:00:00Z",
              last_outbound_at: null,
            },
          },
        }),
        t,
        clock,
      ),
    ).toBe(
      "It was due on 9 Nov and is still open. Last contact from Sonya Beck: 8 Nov. Last contact to Sonya Beck: none you can open.",
    );
  });

  it("is absent for a row with no event of its own", () => {
    expect(cardLine(row({ source: "dedupe_candidate" }), t, clock)).toBeNull();
  });
});
