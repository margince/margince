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
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { LeadScreen } from "./leads";

// The lead page's head, read the way contactpage.test.tsx reads the contact's:
// the inline subtitle, the pills, and the facts strip a reader glances before
// anything else. Kept out of leads.test.tsx, already at its own line ceiling.

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

function stubLead(overrides: Partial<Lead> = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
        });
      }
      if (request.url.endsWith("/v1/connectors")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({ ...lead, ...overrides });
    }),
  );
}

describe("the lead page's head", () => {
  it("reads the title and company on the name's own line", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    expect(document.querySelector(".record-sub-inline")?.textContent).toBe(
      "VP Sales · Nordwind Logistik",
    );
  });

  it("shows the lead marker and the ladder status as pills, and nothing else there", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    // The status also reads as a StatCard on the overview and as a read-only
    // field in the rail, so the pulse row is queried by its own container
    // rather than by the word alone.
    const row = document.querySelector(".record-pulse");
    expect(row?.textContent).toBe("LeadContacted");
  });

  it("carries the facts strip: email, company, owner, score, created and source", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const strip = document.querySelector(".record-facts") as HTMLElement;
    expect(strip).toBeTruthy();

    // Scoped to the strip: the email and the company both repeat in the
    // details pane's own fields, open by default beside it.
    expect(within(strip).getByText("jonas@nordwind.example")).toBeTruthy();
    expect(within(strip).getByText("Nordwind Logistik")).toBeTruthy();
    // The score fact: the number and its reason word, without a repeated
    // "Score:" label, since the strip's own Eyebrow already says what this is.
    expect(within(strip).getByText(/72 · Decision-maker title/)).toBeTruthy();
    expect(within(strip).getByText("Webinar signup")).toBeTruthy();
    // The owner's Assign control stays reachable from the strip.
    expect(within(strip).getByRole("button", { name: "Assign" })).toBeTruthy();
  });

  it("puts a terminal status in its own tone", async () => {
    stubLead({
      status: "disqualified",
      archived_at: "2026-07-01T00:00:00Z",
    });
    render(<LeadScreen id="l-1" />);

    const row = await screen.findByText("Lead");
    const pill = row
      .closest(".record-pulse")
      ?.querySelector(".badge:last-child");
    expect(pill?.textContent).toBe("Disqualified");
    expect(pill?.classList.contains("badge-warning")).toBe(true);
  });
});
