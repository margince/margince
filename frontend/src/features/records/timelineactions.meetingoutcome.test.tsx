// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { TimelineActions } from "./timelineactions";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const MEETING: Activity = {
  id: "m-1",
  kind: "meeting",
  subject: "Site visit",
  occurred_at: "2020-03-02T09:00:00Z",
  occurred_precision: "instant",
  offers_email: false,
  is_done: false,
  source: "gcal",
  captured_by: "connector:gcal",
  content_state: "available",
  created_at: "2020-03-01T08:00:00Z",
  updated_at: "2020-03-01T08:00:00Z",
  writable: true,
  awaiting_outcome: true,
  version: 5,
};

function show(activity: Activity) {
  const patched: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/m-1": () => jsonResponse(activity),
    "PATCH /activities/m-1": (body) => {
      patched.push(body);
      return jsonResponse({ ...activity, version: 6 });
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <TimelineActions
            activity={activity}
            entityType="company"
            entityId="company-1"
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return patched;
}

it("offers a meeting the server says awaits an outcome the same choice as the lane", async () => {
  const patched = show(MEETING);
  const user = userEvent.setup();

  await user.click(
    await screen.findByRole("button", { name: "Update Site visit" }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "No-show" }));
  await user.click(within(dialog).getByRole("button", { name: "Log" }));

  expect(await screen.findByText("Meeting outcome recorded")).toBeTruthy();
  expect(patched).toHaveLength(1);
  expect(patched[0]).toMatchObject({ meeting_status: "no_show" });
});

it("offers a past booked meeting the server stamps as awaiting one", async () => {
  show({ ...MEETING, meeting_status: "booked" });
  expect(
    await screen.findByRole("button", { name: "Mark as canceled: Site visit" }),
  ).toBeTruthy();
});

it.each([
  ["the server says awaits none", { awaiting_outcome: false }],
  ["the reader may not write", { writable: false }],
])("offers no outcome on a meeting %s", async (_name, change) => {
  show({ ...MEETING, ...change });
  await userEvent.click(
    screen.getByRole("button", { name: "More actions for Site visit" }),
  );
  expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^Update/ })).toBeNull();
  expect(screen.queryByRole("button", { name: /canceled/ })).toBeNull();
});

it("offers no cancel on a meeting the server marks calendar-managed", async () => {
  show({ ...MEETING, invitation_status: "confirmed", calendar_managed: true });
  expect(
    await screen.findByRole("button", { name: "Update Site visit" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /canceled/ })).toBeNull();
});

it("offers a cancel on a meeting the server does not mark calendar-managed", async () => {
  show({ ...MEETING, invitation_status: "canceled", calendar_managed: false });
  expect(
    await screen.findByRole("button", { name: "Mark as canceled: Site visit" }),
  ).toBeTruthy();
});
