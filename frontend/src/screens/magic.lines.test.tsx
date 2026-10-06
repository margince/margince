/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatDateTime,
  formatDayMonth,
  formatTimeOfDay,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { line, receipt, renderMagic, stub } from "./magic.testkit";

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

// A line as a reader scans it, and the detail it keeps for a hover.
function rowOf(item: HTMLElement) {
  return {
    mark: item.querySelector<HTMLElement>(".magic-mark")?.dataset.kind,
    sentence: item.querySelector(".magic-line-text")?.firstChild?.textContent,
    subject: item.querySelector(".magic-line-subject")?.textContent,
    detail: item.title,
    when: item.querySelector("time"),
  };
}

// Found whether or not its lane is folded, as the done lane is.
async function onlyLine(name: string): Promise<HTMLElement> {
  const list = await screen.findByRole("list", { name, hidden: true });
  return within(list).getByRole("listitem", { hidden: true });
}

describe("a receipt line is one row", () => {
  it("prints what happened and its record, and keeps who, why and why not for a hover", async () => {
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
    expect(row.sentence).toBe("A deal moved to its next stage");
    expect(row.subject).toBe("Fleet retrofit");
    expect(row.detail).toBe(
      "Overnight agent · The deal sits in a later stage now. · This kind of change cannot be undone.",
    );
    // The hour on the clock the reader is on, the full instant one hover away.
    const zone = viewerZone();
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
    expect(rowOf(await onlyLine("Done for you")).when?.textContent).toBe(
      formatDayMonth("2026-09-09T14:00:00Z", "en", viewerZone()),
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
      name: "Done for you",
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
      // The job's undos are inside, one per record, behind its record count.
      "Fleet retrofit and 2 more",
      "Fleet retrofit, and possibly others",
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
    expect(row.sentence).toBe("Created contact");
    expect(row.subject).toBe("Anna Weber and 2,145 more");
    const done = await screen.findByRole("list", {
      name: "Done for you",
      hidden: true,
    });
    // One press that archived 2,146 contacts nobody had looked at would be the
    // unasked bulk write this page reports, so the row carries no Undo.
    expect(
      within(done)
        .getAllByRole("button", { hidden: true })
        .map((b) => b.textContent),
    ).toEqual(["Anna Weber and 2,145 more"]);
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
    expect(rowOf(await onlyLine("Needs restoring")).mark).toBe("restore");
  });
});
