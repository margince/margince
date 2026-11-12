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
      ?.firstChild?.textContent,
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

it("says what to do from the row's reason, then its detail, then nothing", () => {
  stubApi({});
  const detailed = {
    ...taskRow("task-1", "Send the sheet"),
    detail: "Sonya asked for the new prices.",
  };
  const reasoned: WorklistItem = {
    ...detailed,
    id: "task-3",
    because: [{ kind: "buyer_wrote_last" }],
  };
  const bare = taskRow("task-2", "Call Lumen");
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [
        taskRow("task-0", "Call Weber"),
        reasoned,
        detailed,
        bare,
      ])}
      state="ready"
    />,
  );
  const [byReason, byDetail, none] = waitingCards(container);
  expect(byReason.querySelector(".brief-task-line")?.textContent).toMatch(
    /wrote last$/,
  );
  expect(byDetail.querySelector(".brief-task-line")?.textContent).toBe(
    "Sonya asked for the new prices.",
  );
  expect(none.querySelector(".brief-task-line")).toBeNull();
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
  expect(late.textContent).toContain("10 Sept");
  expect(undated.querySelector(".badge")).toBeNull();
  expect(screen.queryByText(en["brief.task.undated"])).toBeNull();
});

it("keeps the next card in hand when the completed selection leaves the queue", async () => {
  const user = userEvent.setup();
  stubApi({});
  const rows = [
    taskRow("task-0", "Call Weber"),
    taskRow("task-1", "Send the sheet"),
    taskRow("task-2", "Call Lumen"),
  ];
  const view = render(<BriefFeed day={readingsDay({}, rows)} state="ready" />);
  const frame = () => view.container.querySelector(".brief-triage-frame");
  expect(document.activeElement).toBe(document.body);
  const [nextCard] = waitingCards(view.container);
  await user.click(within(nextCard).getByText("Send the sheet"));
  expect(
    view.container.querySelector(".brief-triage-lead .worklist-row-title")
      ?.firstChild?.textContent,
  ).toBe("Send the sheet");
  expect(document.activeElement).toBe(frame());
  view.rerender(
    <BriefFeed day={readingsDay({}, [rows[0], rows[2]])} state="ready" />,
  );
  expect(
    view.container.querySelector(".brief-triage-lead .worklist-row-title")
      ?.firstChild?.textContent,
  ).toBe("Call Lumen");
  expect(document.activeElement).toBe(frame());
});

it("asks whether a promise is done in place of the open line, on the waiting card and the card in hand", () => {
  stubApi({});
  const question = {
    contact_id: "contact-1",
    email_activity_id: "mail-1",
    wrote_at: "2026-09-02T09:00:00Z",
    wrote_precision: "instant",
    claim_key: "moment:may_be_done:task-1",
    evidence_fingerprint: "fp",
  } as const;
  const asking = (id: string, title: string): WorklistItem => ({
    ...taskRow(id, title),
    due_at: "2026-08-25T09:00:00Z",
    overdue: false,
    may_be_done: { ...question, claim_key: `moment:may_be_done:${id}` },
  });
  const { container } = render(
    <BriefFeed
      day={readingsDay({}, [
        asking("task-0", "Call Weber"),
        asking("task-1", "Send the sheet"),
      ])}
      state="ready"
    />,
  );
  const [card] = waitingCards(container);
  expect(
    within(card).getByRole("button", { name: en["worklist.mayBeDone.notYet"] }),
  ).toBeTruthy();
  expect(within(card).queryByText(en["worklist.overdue"])).toBeNull();
  const lead = container.querySelector<HTMLElement>(".brief-triage-lead");
  expect(lead).not.toBeNull();
  expect(
    within(lead as HTMLElement).getByRole("button", {
      name: en["worklist.mayBeDone.notYet"],
    }),
  ).toBeTruthy();
  expect(screen.queryByText(en["worklist.ask.task"])).toBeNull();
});
