/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefFeed } from "./brief.feed";
import { readingsDay, taskRow } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual(rows.slice(0, 6).map((row) => row.title));
  expect(screen.getByText("6 focus cards")).toBeTruthy();
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
  ).toBe("#/brief?filter=all&queue=1");
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
  render(<BriefFeed day={day} state="ready" />);
  expect(screen.queryByText(en["brief.feed.clear"])).toBeNull();
  expect(screen.getByText(en["brief.feed.incomplete"])).toBeTruthy();
});

it("keeps approvals in the agenda and offers their review action", () => {
  stubApi({});
  const row = {
    ...taskRow("a", "Approve the buyer email"),
    source: "approval" as const,
    category: "decisions" as const,
    actions: ["decide" as const],
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(screen.getByText("Approve the buyer email")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: en["worklist.verb.decide"] }),
  ).toBeTruthy();
});
