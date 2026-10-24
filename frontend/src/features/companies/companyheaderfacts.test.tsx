/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CompanyHeadLines } from "./companyheaderfacts";

// The lines under the account's name: what it is, its site, and the way in.

type Company = components["schemas"]["Company"];

// Typed, not asserted. A fixture cast into the contract type can drop a
// required field and still compile, so the test would go on passing after
// the wire shape moved under it, which is the one thing a fixture must not do.
const COMPANY: Company = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  industry: "automotive",
  size_band: "51-200",
  domains: [
    {
      id: "d-1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u-author",
    },
  ],
  owner_id: "u-owner",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderInApp(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// What the account is, its site and the way in, one line each under the name.
describe("CompanyHeadLines", () => {
  it("draws nothing for an account with neither an industry nor a website", () => {
    const { container } = renderInApp(
      <CompanyHeadLines
        company={{
          ...COMPANY,
          industry: undefined,
          domains: [],
          website_url: undefined,
        }}
      />,
    );
    expect(container.textContent).toBe("");
  });

  it("gives industry and the site a line each, the site as a link", () => {
    const { container } = renderInApp(<CompanyHeadLines company={COMPANY} />);
    expect(container.textContent).not.toContain("·");
    expect(screen.getByText("Automotive")).toBeTruthy();
    expect(screen.getByText("brandt.example").tagName).toBe("A");
  });
});
