/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { LeadScreen } from "./leads";

// The lead header's Log activity / Add task pair, and the Answer row's own
// Reply verb opening the SAME composer as the header's Email verb. Kept out
// of leads.test.tsx, already at its own line ceiling, the way leadheader.test.tsx
// keeps the rest of the head out of it.

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
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

// The six shipped lead sources GET /lead-sources serves on a fresh install,
// read by the overview even where no test here cares which one this lead
// carries.
const SHIPPED_LEAD_SOURCES = {
  data: [
    {
      id: "src-manual",
      key: "manual",
      label: "Created manually",
      intent: "neutral",
      sort_order: 10,
      active: true,
      system: true,
      lead_count: 0,
      version: 1,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
  ],
  discovered: [],
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

// A URL-capturing fetch stub answering everything the overview reads, the
// same shape leads.test.tsx's own stubFetchWithMe carries, kept local rather
// than imported so this file has no dependency on a suite already at its own
// line ceiling.
function stubLead(
  overrides: Partial<Lead> = {},
  connectors: unknown = { data: [] },
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "GET" && request.url.endsWith("/v1/connectors")) {
        return jsonResponse(connectors);
      }
      if (request.url.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
        });
      }
      if (new URL(request.url).pathname.endsWith("/context")) {
        return jsonResponse({
          anchor: { type: "lead", id: "l-1" },
          sections: [],
        });
      }
      if (
        request.method === "GET" &&
        request.url.endsWith("/v1/lead-sources")
      ) {
        return jsonResponse(SHIPPED_LEAD_SOURCES);
      }
      if (
        request.method === "GET" &&
        request.url.endsWith("/v1/leads/settings")
      ) {
        return jsonResponse({
          first_response_enabled: true,
          first_response_target_minutes: 240,
        });
      }
      if (request.method === "GET" && request.url.includes("/manual-signals")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({ ...lead, ...overrides });
    }),
  );
}

describe("the lead header's Log activity and Add task", () => {
  it("opens the drawer on a note from Log activity, and on a task from Add task", async () => {
    stubLead();
    render(<LeadScreen id="l-1" />);

    const logButton = await screen.findByRole("button", {
      name: "Log activity",
    });
    await vi.waitFor(() =>
      expect(logButton.hasAttribute("disabled")).toBe(false),
    );
    await userEvent.click(logButton);
    expect((await screen.findByLabelText("Type")).textContent).toContain(
      "Note",
    );
    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    await userEvent.click(
      await screen.findByRole("button", { name: "Add task" }),
    );
    expect((await screen.findByLabelText("Type")).textContent).toContain(
      "Task",
    );
  });

  it("refuses both verbs with the same reason the header's Email verb reads on a terminal lead", async () => {
    stubLead({
      status: "disqualified",
      archived_at: "2026-07-01T00:00:00Z",
    });
    render(<LeadScreen id="l-1" />);

    const email = await screen.findByRole("button", { name: "Email" });
    const logButton = screen.getByRole("button", { name: "Log activity" });
    const addTask = screen.getByRole("button", { name: "Add task" });
    expect(logButton.hasAttribute("disabled")).toBe(true);
    expect(addTask.hasAttribute("disabled")).toBe(true);
    // The SAME id, not a second sentence: one reason serves every refused
    // control on the page (ADR-0108 §6).
    expect(logButton.getAttribute("aria-describedby")).toBe(
      email.getAttribute("aria-describedby"),
    );
    expect(addTask.getAttribute("aria-describedby")).toBe(
      email.getAttribute("aria-describedby"),
    );
  });
});

describe("the Answer row's Reply verb", () => {
  it("opens the exact composer the header's Email verb opens", async () => {
    stubLead(
      { first_response_at: null },
      {
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      },
    );
    render(<LeadScreen id="l-1" />);

    const reply = await screen.findByRole("button", { name: "Reply" });
    await userEvent.click(reply);

    const dialog = await screen.findByRole("dialog", { name: /Draft email/ });
    expect(
      within(dialog).getByRole("button", {
        name: "Remove jonas@nordwind.example",
      }),
    ).toBeTruthy();
  });
});
