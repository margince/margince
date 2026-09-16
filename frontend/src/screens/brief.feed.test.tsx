/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefFeed } from "./brief.feed";
import { readingsDay, taskRow } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/** The row in hand, drawn whole on the panel's left. */
function inHand(container: HTMLElement) {
  const lead = container.querySelector(".brief-triage-lead");
  if (!(lead instanceof HTMLElement)) throw new Error("no row is in hand");
  return within(lead);
}

it("renders the server focus even when the queue page contains different rows", () => {
  stubApi({});
  const rows = Array.from({ length: 9 }, (_, index) =>
    taskRow(`task-${index}`, `Call buyer ${index}`),
  );
  const { container } = render(
    <BriefFeed
      day={{
        ...readingsDay({}, [rows[8]]),
        focus: { items: rows.slice(0, 6), total: 9, urgent_remaining: 0 },
      }}
      state="ready"
    />,
  );
  // The day's first row is in hand, drawn whole; the queue beside it names
  // every focus row in the server's order, with its rank in the reader's own
  // numerals.
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual([rows[0].title]);
  expect(container.querySelector(".worklist-rank")?.textContent).toBe("1");
  expect(
    [...container.querySelectorAll(".brief-focus-item-title")].map(
      (item) => item.textContent,
    ),
  ).toEqual(rows.slice(0, 6).map((row) => row.title));
  expect(
    [...container.querySelectorAll(".brief-focus-list > li")].map(
      (item) => item.querySelector(".brief-focus-rank")?.textContent,
    ),
  ).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(screen.getByText("6 priorities in focus")).toBeTruthy();
  expect(screen.getByText(en["brief.focus.startHere"])).toBeTruthy();
  expect(screen.getByText("1 of 6")).toBeTruthy();
});

it("puts a queued row in hand when it is pressed, and says where it stands", async () => {
  stubApi({});
  const user = userEvent.setup();
  const rows = Array.from({ length: 3 }, (_, index) =>
    taskRow(`task-${index}`, `Call buyer ${index}`),
  );
  const { container } = render(
    <BriefFeed day={readingsDay({}, rows)} state="ready" />,
  );
  await user.click(screen.getByRole("button", { name: /Call buyer 2/ }));
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual([rows[2].title]);
  expect(container.querySelector(".worklist-rank")?.textContent).toBe("3");
  expect(screen.getByText("3 of 3")).toBeTruthy();
  // "Start here" names the first row and only the first: over a row the
  // reader chose it would claim the page had chosen for them.
  expect(screen.queryByText(en["brief.focus.startHere"])).toBeNull();
  expect(
    screen
      .getByRole("button", { name: /Call buyer 2/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
});

it("opens the full queue instead of growing focus when more pages exist", () => {
  stubApi({});
  render(
    <BriefFeed
      day={{
        ...readingsDay({}, [taskRow("t", "Call the buyer")]),
        next_cursor: "next",
      }}
      state="ready"
    />,
  );
  expect(
    screen.queryByRole("button", { name: en["worklist.more"] }),
  ).toBeNull();
  expect(
    screen
      .getByRole("link", { name: en["brief.feed.fullWorklist"] })
      .getAttribute("href"),
  ).toBe("#/home?filter=all&queue=1");
});

it("warns about urgent work beyond the loaded page using server urgency facts", () => {
  stubApi({});
  const row = { ...taskRow("t", "Pinned task"), urgent: false };
  render(
    <BriefFeed
      day={{
        ...readingsDay({}, [row], [], { urgent: 3 }),
        focus: { items: [row], total: 4, urgent_remaining: 3 },
      }}
      state="ready"
    />,
  );
  expect(screen.getByText("3 more urgent items in the queue")).toBeTruthy();
});

it("shows dates and does not repeat the ranking comparator", () => {
  stubApi({});
  const row = {
    ...taskRow("t", "Call the buyer"),
    due_at: "2026-06-09T08:30:00Z",
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(screen.getByText(/due 09\/06\/2026/)).toBeTruthy();
  expect(screen.queryByText("Why it is here")).toBeNull();
  expect(screen.queryByText(/Above the next/)).toBeNull();
});

it("does not claim an empty agenda when a source failed", () => {
  stubApi({});
  const day = readingsDay({}, []);
  day.sources_unavailable = [
    { source: "task", reason: "failed", category: "tasks" },
  ];
  const { container } = render(<BriefFeed day={day} state="ready" />);
  expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
  // The sentence pays the pane's inset, and no empty grid stands under it: a
  // list of nothing was only its own padding, a blank band under the caption.
  expect(
    screen.getByText(en["brief.feed.incomplete"]).closest(".panel-body"),
  ).not.toBeNull();
  expect(container.querySelector(".brief-feed-list")).toBeNull();
});

it("keeps approvals in the agenda and offers their review action", () => {
  stubApi({});
  const row = {
    ...taskRow("a", "Approve the buyer email"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: ["decide" as const],
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  expect(inHand(container).getByText("Approve the buyer email")).toBeTruthy();
  expect(
    inHand(container).getByRole("button", { name: en["worklist.verb.decide"] }),
  ).toBeTruthy();
});

it.each([false, true])(
  "Focus omits personal pin controls (previously pinned: %s)",
  (pinned) => {
    stubApi({});
    const row: ReturnType<typeof taskRow> = {
      ...taskRow("task", "Call the buyer"),
      because: pinned ? [{ kind: "pinned" }] : [],
      above_next: pinned ? { comparator: "pin" } : undefined,
    };
    const { container } = render(
      <BriefFeed day={readingsDay({}, [row])} state="ready" />,
    );
    expect(
      screen.queryByRole("button", {
        name: en["worklist.verb.pin"],
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["worklist.verb.unpin"],
      }),
    ).toBeNull();
    expect(inHand(container).getByText("Call the buyer")).toBeTruthy();
    expect(screen.queryByText(/you pinned/i)).toBeNull();
  },
);
