/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { LeadScreen } from "./leads";

// The lead page's head, read the way contactpage.test.tsx reads the contact's.
// One fact per line sits under the name and the pills, then who holds the lead in
// the details card. Kept out of leads.test.tsx, already at its own line ceiling.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const LEAD_GRANTS = {
  lead: ["read", "create", "update", "delete"],
  activity: ["read", "create"],
} as const;

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  title: "VP Sales",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  score_reason: "decision_maker_title",
  captured_by: "human:u-1",
  source: "manual",
  source_label: "Webinar signup",
  owner_id: "u-9",
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function stubLead(overrides: Partial<Lead> = {}, grants: unknown[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.includes("/v1/record-grants")) {
        return jsonResponse({ data: grants, page: { next_cursor: null } });
      }
      if (request.url.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
        });
      }
      if (
        request.url.endsWith("/v1/connectors") ||
        new URL(request.url).pathname.endsWith("/activities/latest-inbound")
      ) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({ ...lead, ...overrides });
    }),
  );
}

describe("the lead page's head", () => {
  it("reads the title and the company each on a line of its own", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const lines = document.querySelector(".record-split-lines");
    expect(
      [...(lines?.children ?? [])].map((line) => line.textContent),
    ).toEqual(["VP Sales", "Nordwind Logistik"]);
  });

  it("shows the lead marker, the ladder status and who may open it as pills, and nothing else there", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("button", { name: "Not shared" });
    const row = document.querySelector(".record-split-pills");
    expect(row?.textContent).toBe("LeadContactedNot shared");
    // That this is a lead is a kind, so its pill takes no state colour.
    expect(row?.querySelector(".badge")?.className).not.toMatch(/badge-accent/);
  });

  it("reads Shared once the lead carries a grant, and opens the share screen", async () => {
    stubLead({}, [{ id: "g-1", subject_type: "user", subject_id: "u-2" }]);
    render(<LeadScreen id="l-1" />);

    const pill = await screen.findByRole("button", { name: "Shared" });
    pill.click();
    expect(window.location.hash).toBe("#/share/lead/l-1");
  });

  it("keeps the owner and its Assign control in the details card, once", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const details = document.querySelector(".record-aside") as HTMLElement;
    expect(
      within(details).getAllByRole("button", { name: "Assign" }),
    ).toHaveLength(1);
    // No facts strip is left in the head to hold a second copy.
    expect(
      document.querySelector(".record-split-head .record-facts"),
    ).toBeNull();
  });

  it("names an imported lead's upload as where it came from, never the stamp", async () => {
    stubLead({
      source: "mirror:csv:pipedrive-export:lead:l-7",
      source_label: null,
    });
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    // The timeline's first event is the lead's arrival, named by its source.
    const timeline = (await screen.findAllByRole("list")).find((list) =>
      list.textContent?.includes("Imported from pipedrive-export"),
    );
    expect(timeline).toBeTruthy();
    expect(timeline?.textContent).not.toMatch(/mirror/i);
  });

  it("puts a terminal status in its own tone", async () => {
    stubLead({
      status: "disqualified",
      archived_at: "2026-07-01T00:00:00Z",
    });
    render(<LeadScreen id="l-1" />);

    const row = await screen.findByText("Lead");
    const pill = row
      .closest(".record-split-pills")
      ?.querySelector(".badge:last-child");
    expect(pill?.textContent).toBe("Disqualified");
    expect(pill?.classList.contains("badge-warning")).toBe(true);
  });
});
