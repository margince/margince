/** @vitest-environment happy-dom */

import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { dueInstant } from "../format/calendarday";
import { browserZone } from "../format/timezone";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { LogActivityForm } from "./logactivity";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
});

it("saves a note and its undated next step in one request", async () => {
  vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["create"] } })),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "source-note" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ViewerZoneProvider zone={browserZone()}>
      <LogActivityForm entityType="contact" entityId="contact-one" />
    </ViewerZoneProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Pricing discussion" },
  });
  await user.click(
    screen.getByRole("checkbox", { name: "Add a follow-up task" }),
  );
  expect(
    screen.getByRole("button", { name: "Log" }).hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.change(screen.getByRole("textbox", { name: "Next step" }), {
    target: { value: "Send revised quote" },
  });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    kind: "note",
    subject: "Pricing discussion",
    follow_up_task: { subject: "Send revised quote", body: null },
  });
  expect(posted[0]).not.toHaveProperty("follow_up_task.due_at");
  await waitFor(() =>
    expect(screen.queryByRole("textbox", { name: "Next step" })).toBeNull(),
  );
});

it("dates the next step on the day picked from its calendar", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["create"] } })),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "source-note" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ViewerZoneProvider zone={browserZone()}>
      <LogActivityForm entityType="contact" entityId="contact-one" />
    </ViewerZoneProvider>,
  );
  await user.type(
    screen.getByRole("textbox", { name: "Subject" }),
    "Pricing discussion",
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Add a follow-up task" }),
  );
  await user.type(
    screen.getByRole("textbox", { name: "Next step" }),
    "Send revised quote",
  );
  const dueBox = screen.getByRole("textbox", { name: "Due date" });
  const dueField = dueBox.parentElement;
  if (!dueField) throw new Error("expected the due date box inside its field");
  await user.click(
    within(dueField).getByRole("button", { name: /^Choose date/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Friday, 10 July 2026" }),
  );
  await user.click(screen.getByRole("button", { name: "Log" }));

  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    follow_up_task: {
      subject: "Send revised quote",
      due_at: dueInstant("2026-07-10", browserZone()),
    },
  });
});

// The agreed callback hour reaches the server as the task's due time, flagged
// as a time of day, on the record's clock, not the reader's. The reminder is
// the server's to set, so the form sends none.
it("sends a follow-up's callback time as its due time, flagged as a time", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-26T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["create"] } })),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "source-call" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ViewerZoneProvider zone="Europe/Berlin">
      <LogActivityForm entityType="contact" entityId="contact-one" />
    </ViewerZoneProvider>,
  );
  await user.type(
    screen.getByRole("textbox", { name: "Subject" }),
    "Spoke to the buyer",
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Add a follow-up task" }),
  );
  await user.type(
    screen.getByRole("textbox", { name: "Next step" }),
    "Call back",
  );
  const time = screen.getByRole("textbox", { name: "Time (optional)" });
  expect(time.hasAttribute("disabled")).toBe(true);
  const dueField = screen.getByRole("textbox", {
    name: "Due date",
  }).parentElement;
  if (!dueField) throw new Error("expected the due date box inside its field");
  await user.click(
    within(dueField).getByRole("button", { name: /^Choose date/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Tuesday, 27 October 2026" }),
  );
  expect(time.hasAttribute("disabled")).toBe(false);
  await user.type(time, "15:00{Enter}");
  await user.click(screen.getByRole("button", { name: "Log" }));

  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    follow_up_task: {
      subject: "Call back",
      due_at: "2026-10-27T14:00:00.000Z",
      due_has_time: true,
    },
  });
  expect(posted[0]).not.toHaveProperty("follow_up_task.remind_at");
});

it("sends a follow-up with no time as due by its day", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-26T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["create"] } })),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "source-call" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <ViewerZoneProvider zone="Europe/Berlin">
      <LogActivityForm entityType="contact" entityId="contact-one" />
    </ViewerZoneProvider>,
  );
  await user.type(
    screen.getByRole("textbox", { name: "Subject" }),
    "Spoke to the buyer",
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Add a follow-up task" }),
  );
  await user.type(
    screen.getByRole("textbox", { name: "Next step" }),
    "Call back",
  );
  const dueField = screen.getByRole("textbox", {
    name: "Due date",
  }).parentElement;
  if (!dueField) throw new Error("expected the due date box inside its field");
  await user.click(
    within(dueField).getByRole("button", { name: /^Choose date/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Tuesday, 27 October 2026" }),
  );
  await user.click(screen.getByRole("button", { name: "Log" }));

  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    follow_up_task: {
      due_at: dueInstant("2026-10-27", "Europe/Berlin"),
      due_has_time: false,
    },
  });
});
