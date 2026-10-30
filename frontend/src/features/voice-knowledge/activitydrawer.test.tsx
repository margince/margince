/** @vitest-environment happy-dom */

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
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

it("names a meeting's outside guests beside the contacts who attended", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Quarterly review",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        links: [],
        guests: [
          { address: "ana@client.example", display_name: "Ana Client" },
          { address: "ben@client.example", display_name: "" },
        ],
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
  expect(screen.getByTitle("ana@client.example").textContent).toBe(
    "Ana Client",
  );
  expect(screen.getByTitle("ben@client.example").textContent).toBe(
    "ben@client.example",
  );
});

it("labels each guest's answer, and counts the guests a crowd leaves out", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/source": () =>
      jsonResponse({
        id: "source",
        kind: "meeting",
        subject: "Quarterly review",
        occurred_at: "2026-07-01T09:00:00Z",
        content_state: "available",
        links: [],
        guests_left_out: 50,
        guests: [
          {
            address: "ana@client.example",
            display_name: "Ana",
            response: "declined",
          },
          {
            address: "ben@client.example",
            display_name: "Ben",
            response: "tentative",
          },
          {
            address: "cleo@client.example",
            display_name: "Cleo",
            response: "needs_action",
          },
          {
            address: "dan@client.example",
            display_name: "Dan",
            response: "accepted",
          },
          { address: "eve@client.example", display_name: "Eve" },
        ],
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
  const line = screen.getByTitle("ana@client.example").parentElement;
  expect(line?.textContent).toContain("Ana Declined");
  expect(line?.textContent).toContain("Ben Maybe");
  expect(line?.textContent).toContain("Cleo No answer");
  expect(line?.textContent).toContain("Dan, Eve");
  expect(line?.textContent).toContain("and 50 more guests not listed");
});
it("shows a website form's fields under their labels instead of the note's text", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/form": () =>
      jsonResponse({
        id: "form",
        kind: "note",
        subject: "Website form: Contact",
        body: "Consent: not ticked\n\n> Please call me\n>\n> Email: ada@visitor.example",
        occurred_at: "2026-10-28T13:33:00Z",
        content_state: "available",
        source_system: "web_form",
        links: [],
        web_form: {
          message: "Please call me",
          email: "ada@visitor.example",
          extra: [{ name: "budget", value: "10k" }],
          page_url: "javascript:alert(1)",
          campaign: [
            { name: "utm_source", value: "newsletter" },
            { name: "ref", value: "partner" },
          ],
          consent: { ticked: false, wording: "You may contact me." },
        },
      }),
  });
  renderWithQueryClient(
    <OpenActivityDrawer
      activityId="form"
      zone={viewerZone()}
      onClose={() => {}}
    />,
  );
  expect(await screen.findByText("Please call me")).toBeTruthy();
  const term = (name: string) =>
    screen.getByText(name, { selector: "dt" }).nextElementSibling?.textContent;
  expect(term("Email")).toBe("ada@visitor.example");
  expect(term("budget")).toBe("10k");
  expect(term("Campaign source")).toBe("newsletter");
  expect(term("ref")).toBe("partner");
  expect(term("Consent")).toContain("Not agreed");
  expect(term("Consent")).toContain("You may contact me.");
  // A page address that is no web address stays text, never a link.
  expect(term("Page")).toBe("javascript:alert(1)");
  expect(screen.queryByRole("link", { name: /javascript/ })).toBeNull();
  expect(screen.queryByText(/Consent: not ticked/)).toBeNull();
});
