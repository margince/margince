/** @vitest-environment happy-dom */

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { formatDateTime } from "../../format/format";
import { UTC_ZONE, viewerZone } from "../../format/timezone";
import { renderWithQueryClient } from "../../testing/queryrender";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { OpenActivityDrawer } from "./activitydrawer";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});
it.each([403, 404])(
  "explains unavailable activity after a %s read",
  async (status) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/source": () =>
        jsonResponse(
          {
            type: `urn:margince:problem:${status === 403 ? "permission_denied" : "not_found"}`,
            status,
            code: status === 403 ? "permission_denied" : "not_found",
            title: "Unavailable",
          },
          status,
        ),
    });
    renderWithQueryClient(
      <OpenActivityDrawer
        activityId="source"
        zone={viewerZone()}
        onClose={() => {}}
      />,
    );
    expect(
      await screen.findByText("Activity content is withheld"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /retry/i })).toBeNull();
  },
);
it("labels archived evidence while keeping its permitted text", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "note",
        subject: "Pricing discussion",
        body: "The buyer asked for a revised quote.",
        occurred_at: "2026-07-01T09:00:00Z",
        archived_at: "2026-07-02T09:00:00Z",
        content_state: "available",
        links: [],
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Archived")).toBeTruthy();
  expect(screen.getByText("The buyer asked for a revised quote.")).toBeTruthy();
});
it("lists the activity's files and offers an upload while it is live", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Kickoff",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        links: [],
      }),
    "GET /attachments": () =>
      jsonResponse({
        data: [
          {
            id: "att-price",
            entity_type: "activity",
            entity_id: "source",
            filename: "price-list-2026.pdf",
            created_at: "2026-07-01T09:00:00Z",
          },
        ],
        page: { has_more: false, next_cursor: null },
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  const link = await screen.findByRole("link", {
    name: /price-list-2026\.pdf/,
  });
  expect(link.getAttribute("href")).toBe("/v1/attachments/att-price");
  expect(screen.getByRole("button", { name: "Add document" })).toBeTruthy();
});
it("reads out the value of a field the workspace added to activities", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /custom-fields": () =>
      jsonResponse({
        data: [
          {
            id: "cf-visit-type",
            object: "activity",
            label: "Visit type",
            slug: "visit_type",
            type: "picklist",
            options: ["on_site", "remote"],
            status: "active",
            column_name: "cf_visit_type",
            show_in_create: true,
            created_by: "human:u-1",
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
        ],
        page: { has_more: false },
      }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Plant walk-through",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        links: [],
        cf_visit_type: "on_site",
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Visit type")).toBeTruthy();
  expect(screen.getByText("on_site")).toBeTruthy();
});

it("keeps linked contacts as filing context without claiming they attended", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Project review",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        links: [
          {
            entity_type: "contact",
            entity_id: "contact-1",
            label: "Morgan Reed",
          },
        ],
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={UTC_ZONE}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Filed under")).toBeTruthy();
  expect(
    screen.getByText(
      formatDateTime("2026-07-01T09:00:00Z", "en", viewerZone()),
    ),
  ).toBeTruthy();
  expect(screen.queryByText("Attendees")).toBeNull();
});

it("shows only the meeting's explicit participant projection as attendees", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Project review",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        meeting_participants: [
          {
            contact_id: "11111111-1111-4111-8111-111111111111",
            full_name: "Dana Buyer",
          },
        ],
        links: [],
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="source"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Attendees")).toBeTruthy();
  const contact = screen.getByRole("link", { name: "Dana Buyer" });
  expect(contact.getAttribute("href")).toContain("contacts");
  expect(contact.hasAttribute("title")).toBe(false);
});
