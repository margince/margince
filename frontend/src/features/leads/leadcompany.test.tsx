/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { LeadCompany, useLeadCompanyName } from "./leadcompany";

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 40,
  captured_by: "human:u-1",
  source: "manual",
  writable: true,
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

const LINK_GRANTS: GrantSpec = {
  lead: ["read", "update"],
  company: ["read"],
};

const match: components["schemas"]["LeadCompanyMatch"] = {
  id: "c-9",
  name: "Nordwind Logistik GmbH",
  matched_by: "name",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Serves the company the link names, and records every link the component
// posts.
function stubServer() {
  const posted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.includes("/v1/companies/c-7")) {
        return jsonResponse({ id: "c-7", display_name: "Nordwind Holding SE" });
      }
      posted.push(await request.json());
      return jsonResponse({ ...lead, company_id: match.id });
    }),
  );
  return posted;
}

// The session is seeded before the first render, so a hidden offer is the
// grants' answer and not a session still on its way.
function render(ui: ReactNode, allow: GrantSpec = LINK_GRANTS) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ allow }));
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("LeadCompany", () => {
  it("opens the linked company under the company's own name", async () => {
    stubServer();
    render(<LeadCompany lead={{ ...lead, company_id: "c-7" }} />);
    const link = await screen.findByRole("link", {
      name: "Nordwind Holding SE",
    });
    expect(link.getAttribute("href")).toContain("c-7");
    expect(screen.queryByText("Nordwind Logistik")).toBeNull();
  });

  it("offers the matched company and says the contact waits for qualifying", async () => {
    const posted = stubServer();
    render(<LeadCompany lead={{ ...lead, company_match: match }} />);
    expect(screen.getByText("Nordwind Logistik")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Link to Nordwind Logistik GmbH" }),
    );
    expect(
      screen.getByText(
        "The company name on this lead then opens Nordwind Logistik GmbH.",
        { exact: false },
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "No contact is created now. When you qualify this lead, it becomes a contact at Nordwind Logistik GmbH.",
      ),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Link company" }));
    await waitFor(() => expect(posted).toEqual([{ company_id: "c-9" }]));
  });

  it("says the name matched, and what that rule saw", () => {
    stubServer();
    render(<LeadCompany lead={{ ...lead, company_match: match }} />);
    const offer = screen.getByRole("button", {
      name: "Link to Nordwind Logistik GmbH",
    });
    expect(offer).toHaveAccessibleDescription("Same company name");
    fireEvent.click(offer);
    expect(
      screen.getByText(
        "Nordwind Logistik GmbH has the company name on this lead. The company name on this lead then opens Nordwind Logistik GmbH.",
      ),
    ).toBeTruthy();
  });

  it("offers a company the website domain matched, with no company name typed", () => {
    stubServer();
    render(
      <LeadCompany
        lead={{
          ...lead,
          company_name: null,
          candidate_company_key: "nordwind.example",
          company_match: { ...match, matched_by: "domain" },
        }}
      />,
    );
    const offer = screen.getByRole("button", {
      name: "Link to Nordwind Logistik GmbH",
    });
    expect(offer).toHaveAccessibleDescription("Same website domain");
    fireEvent.click(offer);
    expect(
      screen.getByText(
        "Nordwind Logistik GmbH uses the website domain on this lead. The company name on this lead then opens Nordwind Logistik GmbH.",
      ),
    ).toBeTruthy();
  });

  it("shows the typed name alone when no company matches", () => {
    stubServer();
    render(<LeadCompany lead={lead} />);
    expect(screen.getByText("Nordwind Logistik")).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers no link to a reader who may not change the lead", () => {
    stubServer();
    render(
      <LeadCompany lead={{ ...lead, writable: false, company_match: match }} />,
    );
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers no link to a reader without lead update", () => {
    stubServer();
    render(<LeadCompany lead={{ ...lead, company_match: match }} />, {
      ...LINK_GRANTS,
      lead: ["read"],
    });
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("useLeadCompanyName", () => {
  function Name({ of }: Readonly<{ of: Lead }>) {
    return <output>{useLeadCompanyName(of)}</output>;
  }

  it("names a linked company by its own name, not the typed one", async () => {
    stubServer();
    render(<Name of={{ ...lead, company_id: "c-7" }} />);
    await waitFor(() =>
      expect(screen.getByRole("status").textContent).toBe(
        "Nordwind Holding SE",
      ),
    );
  });

  it("names an unlinked company by the typed name", () => {
    stubServer();
    render(<Name of={lead} />);
    expect(screen.getByRole("status").textContent).toBe("Nordwind Logistik");
  });
});
