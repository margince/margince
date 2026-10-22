/** @vitest-environment happy-dom */

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { RecordZoneProvider } from "../lib/recordzone";
import { meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { LogActivityForm } from "./logactivity";

type CustomField = components["schemas"]["CustomField"];

const VISIT_NOTE: CustomField = {
  id: "cf-visit-note",
  object: "activity",
  label: "Visit note",
  slug: "visit_note",
  type: "text",
  status: "active",
  column_name: "cf_visit_note",
  show_in_create: true,
  reportable: false,
  created_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
});

it("logs an activity with the value of a field the workspace added", async () => {
  vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["create"] } })),
    "GET /custom-fields": () =>
      jsonResponse({ data: [VISIT_NOTE], page: { has_more: false } }),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse({ id: "logged-call" }, 201);
    },
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <RecordZoneProvider zone={viewerZone()}>
      <LogActivityForm entityType="contact" entityId="contact-one" />
    </RecordZoneProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Plant walk-through" },
  });
  fireEvent.change(await screen.findByRole("textbox", { name: "Visit note" }), {
    target: { value: "Asked about the night shift" },
  });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    subject: "Plant walk-through",
    cf_visit_note: "Asked about the night shift",
  });
  await waitFor(() =>
    expect(
      (screen.getByRole("textbox", { name: "Visit note" }) as HTMLInputElement)
        .value,
    ).toBe(""),
  );
});
