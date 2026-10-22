/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { viewerZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubRoutes } from "./compose.testkit";
import { ScheduledSendsScreen } from "./scheduledsends";

const readerZone = viewerZone();

const NEW_MESSAGE = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  status: "scheduled",
  scheduled_at: "2026-09-01T07:00:00Z",
  scheduled_tz: readerZone,
  subject: "The renewal quote",
  to: ["ceo@acme.test"],
  cc: ["cfo@acme.test"],
  body: "Here is the quote.",
  links: [{ entity_type: "company", entity_id: "company-1" }],
  communication_context: "requested_followup",
  version: 3,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

const REPLY = {
  ...NEW_MESSAGE,
  id: "019f7e65-fbf7-7114-b114-40af4af63ae9",
  subject: "Re: the quote",
  anchor_activity_id: "019f7e65-fbf7-7114-b114-40af4af63aff",
  links: undefined,
};

const MARKETING = {
  ...NEW_MESSAGE,
  id: "019f7e65-fbf7-7114-b114-40af4af63aea",
  subject: "The newsletter",
  marketing_purpose: "marketing_email",
};

function mount(rows: unknown[]) {
  const sent = stubRoutes({
    "GET /scheduled-sends": () => jsonResponse(rows),
    "POST /emails": () => jsonResponse({ id: "act-new" }, 201),
    [`POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`]: () =>
      new Response(null, { status: 204 }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return sent;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-08-31T10:00:00Z") });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("offers Edit on a new message and not on a reply or a marketing send", async () => {
  mount([NEW_MESSAGE, REPLY, MARKETING]);
  await screen.findByText("The renewal quote");
  expect(screen.getAllByRole("button", { name: /^Edit: / })).toHaveLength(1);
  expect(
    screen.getByRole("button", { name: "Edit: The renewal quote" }),
  ).toBeVisible();
});

it("opens the composer on the message as it was scheduled", async () => {
  const user = userEvent.setup();
  mount([NEW_MESSAGE]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote",
  );
  const composer = within(await screen.findByRole("dialog"));
  expect(composer.getByText("ceo@acme.test")).toBeVisible();
  expect(composer.getByText("cfo@acme.test")).toBeVisible();
});

it("schedules the edited message first and withdraws the old one after", async () => {
  const user = userEvent.setup();
  const sent = mount([NEW_MESSAGE]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const subject = await screen.findByLabelText("Subject");
  await user.clear(subject);
  await user.type(subject, "The renewal quote, revised");
  writeMessage("Body", "Here is the revised quote.");
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.map((call) => call.key)).toContain(
      `POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`,
    ),
  );
  const keys = sent.map((call) => call.key);
  const create = sent.find((call) => call.key === "POST /emails");
  expect(create?.body).toMatchObject({
    subject: "The renewal quote, revised",
    to: ["ceo@acme.test"],
    cc: ["cfo@acme.test"],
    scheduled_tz: readerZone,
  });
  expect(keys.indexOf("POST /emails")).toBeLessThan(
    keys.indexOf(`POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`),
  );
});

it("leaves the original waiting when the new send is refused", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes({
    "GET /scheduled-sends": () => jsonResponse([NEW_MESSAGE]),
    "POST /emails": () =>
      new Response(
        JSON.stringify({ type: "about:blank", title: "No", status: 422 }),
        {
          status: 422,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.some((call) => call.key === "POST /emails")).toBe(true),
  );
  expect(sent.some((call) => call.key.endsWith("/cancel"))).toBe(false);
});
