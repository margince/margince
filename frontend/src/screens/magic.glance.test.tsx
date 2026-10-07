/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { MAGIC_PAGE_LINES } from "./magic.queries";
import { line, receipt, renderMagic, stub } from "./magic.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const mailFiling = {
  type: "system",
  id: "link-reconcile",
  label: { key: "magic.by.mail_filing" },
} as const;

const lineId = (n: number) =>
  `00000000-0000-7000-8000-${String(n).padStart(12, "0")}`;

// Each reading as a reader sees it: its name, its figure, and who did it.
function readings(strip: HTMLElement): string[][] {
  return Array.from(strip.querySelectorAll(".stat-card")).map((card) =>
    [".stat-card-label-text", ".stat-card-value", ".stat-card-detail"].map(
      (part) => card.querySelector(part)?.textContent ?? "",
    ),
  );
}

describe("the receipt at a glance", () => {
  it("counts what got done by kind, in records, largest first", async () => {
    stub(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001" }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
            count: 1200,
          }),
          line({
            id: "00000000-0000-7000-8000-000000000003",
            summary: {
              key: "magic.action.fields_changed",
              values: { fields: "phone" },
            },
            count: 5,
          }),
          line({
            id: "00000000-0000-7000-8000-000000000004",
            summary: { key: "magic.action.update" },
          }),
          line({ id: "00000000-0000-7000-8000-000000000005" }),
          // A sentence this build predates counts toward no reading.
          line({
            id: "00000000-0000-7000-8000-000000000006",
            summary: { key: "magic.action.something_newer" },
            count: 99,
          }),
        ],
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([
      ["Emails filed", "1,200", "Mail filing"],
      ["Records updated", "6", ""],
      ["Deals moved on", "2", ""],
    ]);
  });

  // "Done for you" is what an agent did; mail filing and other sync keep the
  // records current, which is maintenance, so it is counted apart.
  it("counts an agent's work apart from what sync kept current", async () => {
    stub(
      receipt({
        done: [
          line({ id: lineId(1), count: 2 }),
          line({
            id: lineId(2),
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
            count: 1200,
          }),
        ],
      }),
    );
    renderMagic();
    expect(
      await within(
        await screen.findByRole("list", { name: "Summary" }),
      ).findByText("2 done for you · 1,200 kept in sync"),
    ).toBeTruthy();
  });

  it("counts records created and archived as readings of their own", async () => {
    stub(
      receipt({
        done: [
          line({
            id: lineId(1),
            summary: { key: "magic.action.create_contact" },
            actor: mailFiling,
            count: 30,
          }),
          line({
            id: lineId(2),
            summary: { key: "magic.action.create_company" },
            actor: mailFiling,
            count: 4,
          }),
          line({
            id: lineId(3),
            summary: { key: "magic.action.archive_activity" },
            actor: mailFiling,
            count: 9,
          }),
        ],
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([
      ["Records created", "34", "Mail filing"],
      ["Records archived", "9", "Mail filing"],
    ]);
  });

  it("names the job with the most records across a reading's lines", async () => {
    const mailReader = {
      type: "system",
      id: "mail-reader",
      label: { key: "magic.by.mail_reader" },
    } as const;
    const filed = (
      id: string,
      actor: typeof mailFiling | typeof mailReader,
      count: number,
    ) =>
      line({ id, summary: { key: "magic.action.mail_filed" }, actor, count });
    stub(
      receipt({
        done: [
          filed("00000000-0000-7000-8000-000000000001", mailFiling, 4),
          filed("00000000-0000-7000-8000-000000000002", mailReader, 5),
          filed("00000000-0000-7000-8000-000000000003", mailFiling, 4),
        ],
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([["Emails filed", "13", "Mail filing"]]);
  });

  it("names nobody where a cut-short count could hide a larger share", async () => {
    stub(
      receipt({
        done: [
          line({
            id: "00000000-0000-7000-8000-000000000001",
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
            count: 10,
          }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            summary: { key: "magic.action.mail_filed" },
            count: 3,
            count_is_floor: true,
          }),
        ],
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([["Emails filed", "13+", ""]]);
  });

  it("reads every sum as a minimum, and names no job, when the done lane fills its page", async () => {
    stub(
      receipt({
        done: Array.from({ length: MAGIC_PAGE_LINES }, (_, n) =>
          line({
            id: lineId(n + 1),
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
          }),
        ),
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([["Emails filed", "100+", ""]]);
    expect(
      within(screen.getByRole("list", { name: "Summary" })).getByText(
        "100+ kept in sync",
      ),
    ).toBeTruthy();
    expect(screen.getByText("100+ changes, one by one")).toBeTruthy();
  });

  it("counts a full lane of sources to restore exactly, since the page does not bound it", async () => {
    stub(
      receipt({
        watching: Array.from({ length: MAGIC_PAGE_LINES }, (_, n) =>
          line({
            id: lineId(n + 1),
            lane: "watching",
            summary: {
              key: "magic.action.capture_reauth_required",
              values: { provider: "google" },
            },
          }),
        ),
      }),
    );
    renderMagic();
    const summary = await screen.findByRole("list", { name: "Summary" });
    expect(within(summary).getByText("100 need restoring")).toBeTruthy();
  });

  it("folds the done lines under the changes they stand for, and counts those", async () => {
    stub(
      receipt({
        done: [
          line({ id: "00000000-0000-7000-8000-000000000001", count: 42 }),
          line({ id: "00000000-0000-7000-8000-000000000002" }),
        ],
      }),
    );
    renderMagic();
    const fold = (
      await screen.findByText("All 43 changes, one by one")
    ).closest("details");
    expect(fold?.open).toBe(false);
    // The summary counts the same records the fold does, not the lines.
    expect(
      within(screen.getByRole("list", { name: "Summary" })).getByText(
        "43 done for you",
      ),
    ).toBeTruthy();
    expect(
      // Folded, so hidden until opened: present is what is being asked.
      within(fold ?? document.body).getByRole("list", {
        name: "Done for you",
        hidden: true,
      }),
    ).toBeTruthy();
  });

  it("carries a cut-short count as a floor into every sum it joins", async () => {
    stub(
      receipt({
        done: [
          line({
            id: "00000000-0000-7000-8000-000000000001",
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
            count: 5000,
            count_is_floor: true,
          }),
          line({ id: "00000000-0000-7000-8000-000000000002" }),
        ],
      }),
    );
    renderMagic();
    const glance = await screen.findByRole("region", { name: "What got done" });
    expect(readings(glance)).toEqual([
      ["Emails filed", "5,000+", "Mail filing"],
      ["Deals moved on", "1", ""],
    ]);
    expect(
      within(screen.getByRole("list", { name: "Summary" })).getByText(
        "1 done for you · 5,000+ kept in sync",
      ),
    ).toBeTruthy();
    // "All" would claim the whole of a figure that is only a floor.
    expect(screen.getByText("5,001+ changes, one by one")).toBeTruthy();
    const bulk = screen
      .getByRole("img", { name: /When each line in this receipt happened/ })
      .querySelector<HTMLElement>('.magic-mark[data-shape="bar"]');
    expect(bulk?.title).toMatch(/ · at least 5,000 records · /);
  });

  it("places each line on the window's clock, coloured by who acted", async () => {
    stub(
      receipt({
        done: [
          line({
            id: "00000000-0000-7000-8000-000000000001",
            occurred_at: "2026-09-13T02:00:00Z",
          }),
          line({
            id: "00000000-0000-7000-8000-000000000002",
            occurred_at: "2026-09-12T20:00:00Z",
            summary: { key: "magic.action.mail_filed" },
            actor: mailFiling,
            count: 42,
          }),
        ],
        needs_you: [
          line({
            id: "00000000-0000-7000-8000-000000000003",
            lane: "needs_you",
            occurred_at: "2026-09-13T05:00:00Z",
            summary: { key: "magic.action.approval_send_email" },
          }),
        ],
        could_not_complete: [
          line({
            id: "00000000-0000-7000-8000-000000000004",
            lane: "could_not_complete",
            occurred_at: "2026-09-12T14:00:00Z",
            summary: { key: "magic.action.automation_troubled" },
          }),
        ],
        // Dated when it was SEEN, so it has no place on the clock.
        watching: [
          line({
            id: "00000000-0000-7000-8000-000000000005",
            lane: "watching",
            summary: { key: "magic.action.capture_sync_failing" },
          }),
        ],
      }),
    );
    renderMagic();
    const zone = viewerZone();
    const plot = await screen.findByRole("img", {
      name: `When each line in this receipt happened, from ${formatTimeOfDay("2026-09-12T08:00:00Z", "en", zone)} to now`,
    });
    const marks = Array.from(plot.querySelectorAll<HTMLElement>(".magic-mark"));
    expect(
      marks.map((mark) => [
        mark.dataset.kind,
        mark.style.getPropertyValue("--at"),
      ]),
    ).toEqual([
      ["failed", "25.00%"],
      ["sync", "50.00%"],
      ["agent", "75.00%"],
      ["waiting", "87.50%"],
    ]);
    expect(marks[1].dataset.shape).toBe("bar");
    expect(marks[1].title).toBe(
      `Filed captured email under this contact · 42 records · ${formatTimeOfDay("2026-09-12T20:00:00Z", "en", zone)}`,
    );
    expect(
      within(screen.getByRole("figure"))
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Agents",
      "Sync and rules",
      "Waiting on you",
      "Could not be finished",
    ]);
  });

  it("draws neither readings nor a clock when nothing ran", async () => {
    stub(receipt());
    renderMagic();
    await screen.findByText("Nothing done for you");
    expect(screen.queryByRole("region", { name: "What got done" })).toBeNull();
    expect(screen.queryByRole("figure")).toBeNull();
  });
});
