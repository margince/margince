/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatDateTime,
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../../format/format";
import { browserZone } from "../../format/timezone";
import { de } from "../../i18n/de";
import { en } from "../../i18n/en";
import { vi as viCatalog } from "../../i18n/vi";
import { MAGIC_SENTENCE_KEYS } from "../../lib/magickeys";
import { line, receipt, renderMagic, stub } from "../../testing/magic.testkit";
import { lineNotes } from "./magic.lines";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const fleet = {
  type: "deal",
  id: "00000000-0000-7000-8000-0000000000aa",
  label: "Fleet retrofit",
} as const;

const undoable = {
  undoable: true,
  audit_id: "00000000-0000-7000-8000-0000000000bb",
  version: 7,
} as const;

// A line and its visible explanation.
function rowOf(item: HTMLElement) {
  return {
    mark: item.querySelector<HTMLElement>(".magic-mark")?.dataset.kind,
    sentence: item.querySelector(".magic-line-text")?.firstChild?.textContent,
    subject: item.querySelector(".magic-line-subject")?.textContent,
    detail: item
      .querySelector(".magic-line-text > .t-caption")
      ?.textContent?.trim(),
    when: item.querySelector("time"),
  };
}

// Found whether or not its lane is folded, as the done lane is.
async function onlyLine(name: string): Promise<HTMLElement> {
  const list = await screen.findByRole("list", { name, hidden: true });
  return within(list).getByRole("listitem", { hidden: true });
}

describe("a receipt line is one row", () => {
  it("formats recorded forecast numbers for the reader's locale", async () => {
    stub(
      receipt({
        done: [
          line({
            entity: fleet,
            reason: {
              key: "magic.why.forecast_date_basis",
              values: {
                sample_count: "24",
                median_days: "12.555555555",
                remaining_stages: "2",
              },
            },
          }),
        ],
      }),
    );
    renderMagic("de");
    expect(
      await screen.findByText(
        new RegExp(
          `Median ${formatNumber(12.555555555, "de").replaceAll(".", "\\.")} Tage pro Phase`,
        ),
      ),
    ).not.toBeNull();
    expect(screen.queryByText(/12\.555555555/)).toBeNull();
  });

  it.each<{
    key: string;
    values: Readonly<Record<string, string>> | undefined;
    text: string;
  }>([
    {
      key: "magic.why.forecast_date_basis",
      values: {
        sample_count: "24",
        median_days: "12.5",
        remaining_stages: "2",
      },
      text: "Unconfirmed estimate from 24 won deals: median 12.5 days per stage, 2 stages remaining.",
    },
    {
      key: "magic.why.forecast_date",
      values: undefined,
      text: "Unconfirmed estimate; no usable recorded basis to show.",
    },
    {
      key: "magic.why.stated",
      values: { why: "Her signature on 14 Nov names her CTO." },
      text: "Reason given: Her signature on 14 Nov names her CTO.",
    },
  ])("shows the reason a line gives: $key", async ({ key, values, text }) => {
    stub(receipt({ done: [line({ entity: fleet, reason: { key, values } })] }));
    renderMagic();
    expect(rowOf(await onlyLine(en["magic.lane.done"])).detail).toContain(text);
  });

  it("prints what happened, its record and the reason visibly", async () => {
    stub(
      receipt({
        done: [
          line({
            entity: fleet,
            actor: {
              type: "agent",
              id: "runner",
              label: { key: "magic.by.overnight_agent" },
            },
            consequence: "magic.consequence.stage_moved",
          }),
        ],
      }),
    );
    renderMagic();
    const row = rowOf(await onlyLine("Done for you"));
    expect(row.mark).toBe("agent");
    expect(row.sentence).toBe(en["magic.action.advance_stage"]);
    expect(row.subject).toBe("Fleet retrofit");
    expect(row.detail).toBe(
      "Overnight agent This kind of change cannot be undone.",
    );
    // The hour on the clock the reader is on, the full instant one hover away.
    const zone = browserZone();
    expect(row.when?.textContent).toBe(
      formatTimeOfDay("2026-09-13T07:30:00Z", "en", zone),
    );
    expect(row.when?.title).toBe(
      formatDateTime("2026-09-13T07:30:00Z", "en", zone),
    );
    expect(row.when?.dateTime).toBe("2026-09-13T07:30:00Z");
  });

  it("names the day rather than the hour once the window runs past a day", async () => {
    stub(
      receipt({
        since: "2026-09-06T08:00:00Z",
        done: [line({ occurred_at: "2026-09-09T14:00:00Z" })],
      }),
    );
    renderMagic();
    expect(rowOf(await onlyLine(en["magic.lane.done"])).when?.textContent).toBe(
      formatDayMonth("2026-09-09T14:00:00Z", "en", browserZone()),
    );
  });

  it("carries one undo for a change to one record and none for a job over many, a cut count included", async () => {
    stub(
      receipt({
        done: [
          line({ entity: fleet, undo: undoable }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            entity: fleet,
            count: 3,
            undo: undoable,
          }),
          // A read cut short saw one record, and there may be more.
          line({
            id: "00000000-0000-7000-8000-000000000003",
            entity: fleet,
            count_is_floor: true,
            undo: undoable,
          }),
        ],
      }),
    );
    renderMagic();
    const done = await screen.findByRole("list", {
      name: en["magic.lane.done"],
      hidden: true,
    });
    expect(
      within(done)
        .getAllByRole("button", { hidden: true })
        .map((b) => b.textContent),
    ).toEqual([
      // The record's name opens what changed on it; the undo stands beside.
      "Fleet retrofit",
      "Undo",
      // The job's undos are inside, one per record, behind its change count.
      "View 3 changes",
      "View at least 1 change",
    ]);
  });

  it("folds a mailbox import into one line that opens to an undo per record", async () => {
    stub(
      receipt({
        done: [
          line({
            summary: { key: "magic.action.create_contact" },
            entity: {
              type: "contact",
              id: "00000000-0000-7000-8000-0000000000cc",
              label: "Anna Weber",
            },
            count: 2146,
            undo: undoable,
          }),
        ],
      }),
    );
    renderMagic();
    const row = rowOf(await onlyLine("Done for you"));
    expect(row.sentence).toBe(en["magic.action.create_contact"]);
    expect(row.subject).toBe("Anna Weber and 2,145 more");
    const done = await screen.findByRole("list", {
      name: en["magic.lane.done"],
      hidden: true,
    });
    // One press that archived 2,146 contacts nobody had looked at would be the
    // unasked bulk write this page reports, so the row carries no Undo.
    expect(
      within(done)
        .getAllByRole("button", { hidden: true })
        .map((b) => b.textContent),
    ).toEqual(["View 2,146 changes"]);
  });

  it("marks a source to restore in its lane's colour, since the clock does not place it", async () => {
    stub(
      receipt({
        watching: [
          line({
            lane: "watching",
            summary: {
              key: "magic.action.capture_reauth_required",
              values: { provider: "google" },
            },
          }),
        ],
      }),
    );
    renderMagic();
    expect(rowOf(await onlyLine(en["magic.lane.watching"])).mark).toBe(
      "restore",
    );
  });

  it("names a failing connector by its readable name, never its provider id", async () => {
    stub(
      receipt({
        watching: [
          line({
            lane: "watching",
            summary: {
              key: "magic.action.capture_reauth_required",
              values: { provider: "gcal", account: "anna@example.com" },
            },
          }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            lane: "watching",
            summary: {
              key: "magic.action.capture_sync_failing",
              values: { provider: "imap" },
            },
          }),
        ],
      }),
    );
    renderMagic();
    const watching = await screen.findByRole("list", {
      name: en["magic.lane.watching"],
      hidden: true,
    });
    const text = watching.textContent ?? "";
    expect(text).toContain("Google Calendar needs to be connected again");
    expect(text).toContain("IMAP mailbox is not keeping up");
    expect(text).not.toMatch(/\bgcal\b|\bimap\b/);
  });
});

// The subject beside a sentence draws these values, so a sentence holding one
// names its record twice.
const SUBJECT_VALUES = ["{target}", "{account}", "{name}"];

const occurrences = (text: string, part: string) => text.split(part).length - 1;

describe("a line names its record once", () => {
  it.each([
    ["en", en],
    ["de", de],
    ["vi", viCatalog],
  ] as const)(
    "no %s sentence holds the value its subject draws",
    (_, catalog) => {
      const repeating = MAGIC_SENTENCE_KEYS.filter((key) =>
        SUBJECT_VALUES.some((value) => catalog[key].includes(value)),
      );
      expect(repeating).toEqual([]);
    },
  );

  const failed = (entity?: { label: string }) =>
    line({
      lane: "could_not_complete",
      summary: {
        key: "magic.action.automation_troubled",
        values: {
          name: "Route new lead to a task",
          outcome: "failed",
          reason_code: "target_missing",
        },
      },
      ...(entity && {
        entity: {
          type: "automation",
          id: "00000000-0000-7000-8000-0000000000cc",
          ...entity,
        },
      }),
    });

  it.each([
    [
      "with its rule as the entity",
      failed({ label: "Route new lead to a task" }),
    ],
    ["without an entity", failed()],
  ])("names a failed rule once, %s", async (_, row) => {
    stub(receipt({ could_not_complete: [row] }));
    renderMagic();
    const item = await onlyLine("Could not be finished");
    const text = item.textContent ?? "";
    expect(occurrences(text, "Route new lead to a task")).toBe(1);
    expect(rowOf(item).sentence).toBe(
      "Could not finish (Failed): the record it acts on no longer exists",
    );
    expect(rowOf(item).subject).toBe("Route new lead to a task");
    const separator = item.querySelector(".magic-line-sep");
    expect(separator?.textContent).toBe("·");
    expect(separator?.getAttribute("aria-hidden")).toBe("true");
    expect(separator?.nextElementSibling).toBe(
      item.querySelector(".magic-line-subject"),
    );
  });

  it("names a new sender once", async () => {
    stub(
      receipt({
        needs_you: [
          line({
            lane: "needs_you",
            summary: {
              key: "magic.action.approval_capture_counterparty",
              values: { target: "Ines Kraft" },
            },
          }),
        ],
      }),
    );
    renderMagic();
    const item = await onlyLine("Waiting on you");
    expect(occurrences(item.textContent ?? "", "Ines Kraft")).toBe(1);
    // A decision is a task card: its sentence is the title, its record a chip.
    expect(item.querySelector(".brief-task-title")?.textContent).toBe(
      en["magic.action.approval_capture_counterparty"],
    );
    expect(within(item).getByText("Ines Kraft")).toBeTruthy();
  });
});

describe("the coverage a watching line states", () => {
  const watching = (consequence: string) =>
    lineNotes(line({ lane: "watching", consequence }), (key) => key, "en")
      .coverage;

  it("states a capture consequence", () => {
    expect(watching("magic.consequence.capture_not_collecting")).toBe(
      "magic.consequence.capture_not_collecting",
    );
  });

  it("states nothing for another registered consequence", () => {
    expect(watching("magic.consequence.stage_moved")).toBeNull();
  });
});
