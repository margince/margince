// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// Today, Decisions and System are three reads of the day, split by the server.
// The screen asks for one view at a time and draws only that view's rows.

import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  day,
  panelNamed,
  renderWorklist,
  row,
  stub,
} from "../../testing/worklist.testkit";
import { reviewView } from "./worklist.destinations";
import { worklistLaneHref } from "./worklist.header";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

/** The query of every `/worklist` read the screen made, in order. */
function worklistReads(): URLSearchParams[] {
  return vi
    .mocked(fetch)
    .mock.calls.map(([input]) =>
      String(input instanceof Request ? input.url : input),
    )
    .filter((url) => url.split("?")[0]?.endsWith("/worklist"))
    .map((url) => new URL(url, "http://localhost").searchParams);
}

const judgement = row({
  id: "pair-1",
  source: "dedupe_candidate",
  category: "decisions",
  destination: "review",
  title: "Two records for one company",
});

describe("the work views", () => {
  it("opens on Today and asks the server for that view", async () => {
    stub(day({ queue: [row({ id: "t1", title: "A task" })] }));
    renderWorklist();

    await screen.findByText("A task");
    expect(worklistReads()[0]?.get("view")).toBe("today");
  });

  it("AC-WORKLIST-REVIEW-01: draws a judgement in the Decisions view it is asked from", async () => {
    stub(day({ queue: [judgement] }));
    globalThis.location.hash = "#/worklist?view=decisions";
    renderWorklist();

    const panel = panelNamed(
      await screen.findByText(en["worklist.view.decisions"], {
        selector: "h2, h3",
      }),
    );
    expect(within(panel).getByText("Two records for one company")).toBeTruthy();
    expect(worklistReads()[0]?.get("view")).toBe("decisions");
  });

  it("opens the matching view from an old filter=decisions link", async () => {
    stub(day({ queue: [judgement] }));
    globalThis.location.hash = "#/worklist?filter=decisions";
    renderWorklist();

    await screen.findByText("Two records for one company");
    const read = worklistReads()[0];
    expect(read?.get("view")).toBe("decisions");
    // The view's own filter rides along for a server that knows no views.
    expect(read?.get("filter")).toBe("decisions");
  });

  it("shows only Today's category buttons inside Today", async () => {
    stub(day({ queue: [row({ id: "t1", title: "A task" })] }));
    renderWorklist();

    const pills = await screen.findByRole("group", {
      name: en["worklist.filter.label"],
    });
    expect(
      within(pills).getByRole("button", { name: en["worklist.filter.tasks"] }),
    ).toBeTruthy();
    expect(
      within(pills).queryByRole("button", {
        name: en["worklist.filter.decisions"],
      }),
    ).toBeNull();
    expect(
      within(pills).queryByRole("button", {
        name: en["worklist.filter.system"],
      }),
    ).toBeNull();
  });

  it("drops the filter when the reader changes view", async () => {
    stub(day({ queue: [row({ id: "t1", title: "A task" })] }));
    globalThis.location.hash = "#/worklist?filter=tasks";
    renderWorklist();
    await screen.findByText("A task");

    await userEvent.click(screen.getByRole("button", { name: /^Decisions/ }));

    await vi.waitFor(() => {
      const last = worklistReads().at(-1);
      expect(last?.get("view")).toBe("decisions");
      expect(last?.get("filter")).toBe("decisions");
    });
  });
});

describe("links into a view", () => {
  it("names the view for a decisions or system link", () => {
    expect(worklistLaneHref("decisions")).toContain("queue_view=decisions");
    expect(worklistLaneHref("system")).not.toContain("filter=");
  });

  it("names the filter for a Today link", () => {
    expect(worklistLaneHref("tasks")).toContain("filter=tasks");
    expect(worklistLaneHref("tasks")).not.toContain("queue_view");
  });
});

describe("a group's Review verb", () => {
  it("opens the view that holds the group's members", () => {
    expect(reviewView(row({ category: "system" }))).toBe("system");
    expect(reviewView(row({ category: "decisions" }))).toBe("decisions");
  });
});

describe("Upcoming and backlog", () => {
  it("sits at the end of Today with its count, and reads its rows when opened", async () => {
    stub(
      day({
        queue: [row({ id: "t1", title: "A task" })],
        counts: [
          {
            category: "upcoming",
            considered: 4,
            shown: 0,
            more_available: false,
          },
        ],
      }),
    );
    renderWorklist();

    const summary = await screen.findByText(en["worklist.filter.upcoming"]);
    expect(summary.closest("summary")?.textContent).toContain("4");
    expect(
      worklistReads().some((read) => read.get("filter") === "upcoming"),
    ).toBe(false);

    await userEvent.click(summary);

    await vi.waitFor(() => {
      const upcoming = worklistReads().find(
        (read) => read.get("filter") === "upcoming",
      );
      expect(upcoming?.get("view")).toBe("today");
    });
  });
});
