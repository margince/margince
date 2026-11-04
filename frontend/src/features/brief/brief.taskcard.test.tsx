/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import type { WorklistItem } from "../../lib/worklisttypes";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefFeed } from "./brief.feed";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function waitingCards(container: HTMLElement): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(".brief-task")];
}

it("marks a waiting task done from its tick, without opening it", async () => {
  const user = userEvent.setup();
  const calls = stubApi({
    "PATCH /activities/task-1": () =>
      jsonResponse({ id: "task-1", version: 2 }),
  });
  const rows = [
    taskRow("task-0", "Call Weber"),
    { ...taskRow("task-1", "Send the sheet"), version: 1 },
  ];
  const { container } = render(
    <BriefFeed day={readingsDay({}, rows)} state="ready" />,
  );
  const [card] = waitingCards(container);
  await user.click(
    within(card).getByRole("button", {
      name: en["brief.task.done"].replace("{title}", "Send the sheet"),
    }),
  );
  await waitFor(() =>
    expect(calls.find((call) => call.method === "PATCH")?.body).toMatchObject({
      is_done: true,
    }),
  );
  // The tick writes; it never puts the card in hand.
  expect(
    container.querySelector(".brief-triage-lead .worklist-row-title")
      ?.textContent,
  ).toBe("Call Weber");
});

it("offers no tick on a row the reader may not complete", () => {
  stubApi({});
  const readOnly: WorklistItem = {
    ...taskRow("task-1", "Send the sheet"),
    actions: ["open"],
  };
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [taskRow("task-0", "Call Weber"), readOnly])}
      state="ready"
    />,
  );
  const [card] = waitingCards(container);
  expect(within(card).queryAllByRole("button")).toHaveLength(1);
});

it("says what to do from the row's reason, and nothing when it has none", () => {
  stubApi({});
  const reasoned = {
    ...taskRow("task-1", "Send the sheet"),
    detail: "Sonya asked for the new prices.",
  };
  const bare = taskRow("task-2", "Call Lumen");
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [taskRow("task-0", "Call Weber"), reasoned, bare])}
      state="ready"
    />,
  );
  const [first, second] = waitingCards(container);
  expect(first.querySelector(".brief-task-line")?.textContent).toBe(
    "Sonya asked for the new prices.",
  );
  expect(second.querySelector(".brief-task-line")).toBeNull();
});

it("draws no clock on an undated task, and the due day on a dated one", () => {
  stubApi({});
  const dated = {
    ...taskRow("task-1", "Send the sheet"),
    due_at: "2026-09-10T08:30:00Z",
    overdue: true,
  };
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [
        taskRow("task-0", "Call Weber"),
        dated,
        taskRow("task-2", "Call Lumen"),
      ])}
      state="ready"
    />,
  );
  const [late, undated] = waitingCards(container);
  expect(late.querySelector(".badge-danger")).toBeTruthy();
  expect(undated.querySelector(".badge")).toBeNull();
  expect(screen.queryByText(en["brief.task.undated"])).toBeNull();
});
