/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { en } from "../i18n/en";
import { BriefFeed } from "./brief.feed";
import { readingsDay, taskRow } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows every loaded row in server order, including rows after five", () => {
  stubApi({});
  const rows = Array.from({ length: 9 }, (_, index) =>
    taskRow(`task-${index}`, `Call buyer ${index}`),
  );
  const { container } = render(
    <BriefFeed day={readingsDay({}, rows)} state="ready" />,
  );
  expect(
    [...container.querySelectorAll(".worklist-row-title")].map(
      (row) => row.textContent,
    ),
  ).toEqual(rows.map((row) => row.title));
  expect(screen.getByText("9 items loaded")).toBeTruthy();
});

it("continues inline and offers retry if loading the next page failed", async () => {
  stubApi({});
  const more = vi.fn();
  const day = {
    ...readingsDay({}, [taskRow("t", "Call the buyer")]),
    next_cursor: "next",
  };
  render(<BriefFeed day={day} state="ready" onMore={more} />);
  await userEvent.click(
    screen.getByRole("button", { name: en["brief.feed.showMore"] }),
  );
  expect(more).toHaveBeenCalledOnce();
  cleanup();
  render(<BriefFeed day={day} state="ready" onMore={more} moreFailed />);
  expect(
    screen.getByRole("button", { name: en["brief.feed.retryMore"] }),
  ).toBeTruthy();
});

it("warns about urgent work beyond the loaded page using server urgency facts", () => {
  stubApi({});
  const row = { ...taskRow("t", "Pinned task"), urgent: false };
  render(
    <BriefFeed
      day={{
        ...readingsDay({}, [row], [], { urgent: 3 }),
        next_cursor: "next",
      }}
      state="ready"
    />,
  );
  expect(screen.getByText("3 urgent items are not loaded yet.")).toBeTruthy();
});

it("shows dates and does not repeat the ranking comparator", () => {
  stubApi({});
  const row = {
    ...taskRow("t", "Call the buyer"),
    due_at: "2026-06-09T08:30:00Z",
  };
  render(<BriefFeed day={readingsDay({}, [row])} state="ready" />);
  expect(
    screen.getByText(
      `due ${formatDateTime(row.due_at, "en", "Europe/Berlin")}`,
    ),
  ).toBeTruthy();
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
