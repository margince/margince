// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { ContactMeetingsTab } from "./contactmeetings";

type Activity = components["schemas"]["Activity"];
type Contact360 = components["schemas"]["Contact360"];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// The read's own clock: "past" and "ahead" are measured against it.
const view: Contact360 = {
  as_of: "2026-09-30T09:00:00Z",
  sections_omitted: [],
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    first_name: "Dana",
    primary_email: "dana@buyer.example",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  activities: { data: [], page: { has_more: false } },
};

function meeting(row: Partial<Activity>): Activity {
  return {
    id: "m-1",
    kind: "meeting",
    subject: "Discovery call",
    occurred_at: "2026-08-31T09:00:00Z",
    occurred_precision: "instant",
    offers_email: false,
    is_done: false,
    source: "gcal",
    captured_by: "connector:gcal",
    content_state: "available",
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    writable: true,
    version: 3,
    ...row,
  };
}

function mount(
  rows: () => Activity[],
  onPatch: (body: unknown) => void = () => {},
) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities": () =>
      jsonResponse({ data: rows(), page: { has_more: false } }),
    "GET /activities/m-1": () => jsonResponse(rows()[0]),
    "PATCH /activities/m-1": (body) => {
      onPatch(body);
      return jsonResponse(rows()[0]);
    },
  });
  const requests = vi.spyOn(globalThis, "fetch");
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <ContactMeetingsTab view={view} />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return requests;
}

async function heldPanel() {
  const panel = await screen.findByRole("region", { name: "Held" });
  await within(panel).findByText("Discovery call");
  return panel;
}

it("records a no-show on a meeting 30 days back, then shows it on the row", async () => {
  let stored = meeting({});
  const patched: unknown[] = [];
  const requests = mount(
    () => [stored],
    (body) => {
      patched.push(body);
      stored = { ...stored, meeting_status: "no_show", version: 4 };
    },
  );
  const user = userEvent.setup();
  const panel = await heldPanel();

  await user.click(within(panel).getByRole("button", { name: "Update" }));
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "No-show" }));
  await user.click(within(dialog).getByRole("button", { name: "Log" }));

  expect(await within(panel).findByText("No-show")).toBeTruthy();
  expect(patched).toHaveLength(1);
  expect(patched[0]).toMatchObject({ meeting_status: "no_show" });
  const patch = requests.mock.calls
    .map(([input]) => input)
    .find((input) => input instanceof Request && input.method === "PATCH");
  expect(patch instanceof Request && patch.headers.get("If-Match")).toBe("3");
  expect(within(panel).queryByRole("button", { name: "Update" })).toBeNull();
});

it.each([
  ["a meeting the reader may not write", { writable: false }],
  ["a meeting that already has a status", { meeting_status: "held" as const }],
  ["a meeting still ahead", { occurred_at: "2026-10-20T09:00:00Z" }],
])("offers no outcome for %s", async (_name, change) => {
  mount(() => [meeting(change)]);
  const body = await screen.findByText("Discovery call");
  expect(body).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Update" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Mark as canceled" })).toBeNull();
});
