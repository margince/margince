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

it("offers a past meeting with no status the same outcome choice as the lane", async () => {
  const patched = show(MEETING);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("button", { name: "Update" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "No-show" }));
  await user.click(within(dialog).getByRole("button", { name: "Log" }));

  expect(await screen.findByText("Meeting outcome recorded")).toBeTruthy();
  expect(patched).toHaveLength(1);
  expect(patched[0]).toMatchObject({ meeting_status: "no_show" });
});

it.each([
  ["not writable", { writable: false }],
  ["already has a status", { meeting_status: "held" as const }],
  ["still ahead", { occurred_at: "2099-03-02T09:00:00Z" }],
  ["is not a meeting", { kind: "call" as const }],
])("offers no outcome for a meeting that is %s", async (_name, change) => {
  show({ ...MEETING, ...change });
  expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Update" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Mark as canceled" })).toBeNull();
});
