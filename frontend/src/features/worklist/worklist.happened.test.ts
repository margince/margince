// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { browserZone } from "../../format/timezone";
import type { Locale, Translator } from "../../i18n";
import { translate } from "../../i18n";
import { row } from "./worklist.fixtures";
import {
  cardLine,
  consequenceLine,
  happenedLines,
  todoLine,
} from "./worklist.happened";

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
  it("says the event, then what to do about it by name", () => {
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
    ).toBe(
      "Sonya Beck wrote on 7 Nov, and nobody has answered yet. Reply to Sonya Beck.",
    );
  });

  it("is absent for a row with no event of its own", () => {
    expect(cardLine(row({ source: "dedupe_candidate" }), t, clock)).toBeNull();
  });
});
