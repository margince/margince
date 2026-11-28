/** @vitest-environment happy-dom */

import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { line, receipt, renderMagic, stub } from "../../testing/magic.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const saved = (n: number, over = {}) =>
  line({
    id: `00000000-0000-7000-8000-00000000010${n}`,
    occurred_at: `2026-09-13T0${n}:00:00Z`,
    summary: { key: "magic.action.create_activity" },
    actor: { type: "system", id: "capture" },
    entity: {
      type: "activity",
      id: `00000000-0000-7000-8000-00000000020${n}`,
      label: `Mail ${n}`,
    },
    ...over,
  });

const archived = line({
  id: "00000000-0000-7000-8000-000000000301",
  occurred_at: "2026-09-13T07:30:00Z",
  summary: { key: "magic.action.archive_activity" },
  count: 2,
});

function renderDone() {
  return renderMagic(
    "en",
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    { lanes: ["done"], embedded: true },
  );
}

function summaryOf(text: string): HTMLElement {
  const summary = screen.getByText(text).closest("summary");
  if (!summary) throw new Error(`${text} is not a summary`);
  return summary;
}

describe("the small changes on Home", () => {
  it("counts every record without previewing small changes while closed", async () => {
    stub(receipt({ done: [archived, saved(5), saved(4), saved(3)] }));
    renderDone();
    await screen.findByText("5 small changes");
    expect(summaryOf("5 small changes").getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(summaryOf("5 small changes").textContent).not.toContain(
      en["magic.action.archive_activity"],
    );
    expect(summaryOf("5 small changes").textContent).not.toContain(
      en["magic.action.create_activity"],
    );
    expect(screen.queryByText(en["magic.lane.done"])).toBeNull();
  });

  it("draws repeated lines as one step with a count, keeping each record", async () => {
    stub(receipt({ done: [saved(5), saved(4), saved(3)] }));
    const user = userEvent.setup();
    renderDone();
    await screen.findByText("3 small changes");
    await user.click(summaryOf("3 small changes"));
    const steps = await screen.findByRole("list", {
      name: en["brief.done.smallLabel"],
    });
    expect(steps.querySelectorAll(":scope > li")).toHaveLength(1);
    expect(screen.getByText("3 changes")).toBeTruthy();
    for (const name of ["Mail 5", "Mail 4", "Mail 3"]) {
      expect(screen.getByText(name)).toBeTruthy();
    }
  });

  it("keeps why a change cannot be undone inside its Details", async () => {
    stub(
      receipt({
        done: [
          saved(5, {
            undo: { undoable: false, reason: "no_completed_change" },
          }),
        ],
      }),
    );
    const user = userEvent.setup();
    renderDone();
    await screen.findByText("1 small change");
    await user.click(summaryOf("1 small change"));
    const note = await screen.findByText(
      en["magic.undoReason.noCompletedChange"],
    );
    const details = note.closest("details");
    expect(details?.querySelector("summary")?.textContent).toBe(
      en["receipt.detail.summary"],
    );
  });

  it("starts closed when small changes are the only completed work", async () => {
    stub(receipt({ done: [saved(5)] }));
    renderDone();
    await waitFor(() =>
      expect(summaryOf("1 small change").getAttribute("aria-expanded")).toBe(
        "false",
      ),
    );
  });
});
