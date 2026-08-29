/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { VatCheckCard } from "./companyvatcheck";

// What this card must get right is not the layout but the DISTINCTIONS: a
// verdict with a receipt is evidence, the same verdict without one is only a
// reading, and a register that did not answer says nothing about the company.
// Collapsing any of those is what would mislead somebody filing a tax return.

const ORG_ID = "00000000-0000-7000-8000-0000000000a1";

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
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function answerWith(body: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(body, status)),
  );
}

describe("VatCheckCard", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows the receipt beside the verdict, because the receipt is what proves the check", async () => {
    answerWith({
      organization_id: ORG_ID,
      vat_number: "DE811907980",
      status: "valid",
      consultation_number: "WAPIAAAAXk3rN2p9",
      registered_name: "Muster Handels GmbH",
      checked_at: "2026-08-14T09:12:00Z",
    });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(await screen.findByText("Valid")).toBeInTheDocument();
    expect(screen.getByText("WAPIAAAAXk3rN2p9")).toBeInTheDocument();
    // The registered name is how a copied imprint gets caught, so it has to
    // reach the screen rather than only the database.
    expect(screen.getByText("Muster Handels GmbH")).toBeInTheDocument();
  });

  it("says a valid answer carries no proof when the register issued no receipt", async () => {
    answerWith({
      organization_id: ORG_ID,
      vat_number: "DE811907980",
      status: "valid",
      checked_at: "2026-08-14T09:12:00Z",
    });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(await screen.findByText("Valid")).toBeInTheDocument();
    expect(screen.getByText(/None issued/)).toBeInTheDocument();
  });

  it("tells a register that did not answer apart from a number that is not valid", async () => {
    answerWith({
      organization_id: ORG_ID,
      vat_number: "DE811907980",
      status: "unavailable",
      checked_at: "2026-08-14T09:12:00Z",
    });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(
      await screen.findByText("Register did not answer"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Not valid")).not.toBeInTheDocument();
  });

  it("reports a company nobody consulted as never asked, not as a failure", async () => {
    answerWith({ title: "not found" }, 404);

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(
      await screen.findByText(/has not been consulted/),
    ).toBeInTheDocument();
  });

  it("treats a body that is not a consultation as no answer", async () => {
    // A 200 carrying something else entirely. The formatter throws on a date
    // it cannot parse, and an unguarded throw here blanks the whole company
    // record rather than this one card.
    answerWith({ data: [], page: {} });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(
      await screen.findByText(/has not been consulted/),
    ).toBeInTheDocument();
  });

  it("survives a verdict this build has no name for", async () => {
    // A server ahead of this tab, which is the ordinary state of affairs
    // during a deploy. The unknown word is worth showing; taking the whole
    // company record down over it is not.
    answerWith({
      organization_id: ORG_ID,
      vat_number: "DE811907980",
      status: "pending_review",
      consultation_number: "WAPIAAAAXk3rN2p9",
      checked_at: "2026-08-14T09:12:00Z",
    });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(await screen.findByText("pending_review")).toBeInTheDocument();
    // The rest of the consultation still reads, because none of it depended
    // on recognising the verdict.
    expect(screen.getByText("WAPIAAAAXk3rN2p9")).toBeInTheDocument();
  });
});
