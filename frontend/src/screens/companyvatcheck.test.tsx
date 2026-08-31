/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
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

// The card's own read, plus the grant read the ask button gates on. Without a
// /me answer every viewer reads as unable to write, and the button this suite is
// about would be absent for the correct reason — which is how a test can pass
// while proving nothing.
function answerWith(body: unknown, status = 200) {
  return stubFetch(async (request) =>
    new URL(request.url).pathname.endsWith("/me")
      ? jsonResponse(meFixture({ allow: { organization: ["read", "update"] } }))
      : jsonResponse(body, status),
  );
}

// Every call this card made, so a test can assert the METHOD and the PATH
// rather than that something happened.
function stubFetch(answer: (request: Request) => Promise<Response>) {
  const calls: { method: string; pathname: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push({
        method: request.method,
        pathname: new URL(request.url).pathname,
      });
      return answer(request);
    }),
  );
  return calls;
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

  it("reports a body it cannot read as a fault, never as never-consulted", async () => {
    // A 200 carrying something else entirely. Two things must not happen: the
    // unparseable date must not reach the formatter, which throws and blanks
    // the whole company record; and the card must not state that this
    // company's VAT ID was never consulted, which is a business fact it has no
    // evidence for.
    answerWith({ data: [], page: {} });

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(
      await screen.findByText(/cannot read|couldn't|could not/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/has not been consulted/),
    ).not.toBeInTheDocument();
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

  it("asks the register when a person presses the button", async () => {
    const user = userEvent.setup();
    const calls = answerWith({
      organization_id: ORG_ID,
      vat_number: "DE811907980",
      status: "valid",
      consultation_number: "WAPIAAAAXk3rN2p9",
      checked_at: "2026-08-14T09:12:00Z",
    });
    render(<VatCheckCard orgId={ORG_ID} />);

    await user.click(
      await screen.findByRole("button", { name: "Check again" }),
    );

    // The POST is the whole feature: nothing else in the product re-asks about
    // a number that has not changed.
    await waitFor(() => {
      expect(
        calls.some(
          (one) =>
            one.method === "POST" &&
            one.pathname === `/v1/organizations/${ORG_ID}/vat-check`,
        ),
      ).toBe(true);
    });
    expect(
      await screen.findByText(/answer appears here once it replies/),
    ).toBeInTheDocument();
  });

  it("offers the ask on a company nobody has consulted", async () => {
    // The state the button matters in most: a number the crawl never checked
    // read "never consulted" with no way to change that.
    answerWith({}, 404);

    render(<VatCheckCard orgId={ORG_ID} />);

    expect(
      await screen.findByRole("button", { name: "Check with the register" }),
    ).toBeInTheDocument();
  });

  it("tells a reader to wait rather than that something broke", async () => {
    const user = userEvent.setup();
    stubFetch(async (request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { organization: ["read", "update"] } }),
        );
      }
      if (request.method === "POST") {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Too Many Requests",
            status: 429,
            detail: "this number was consulted less than 5m0s ago",
          },
          429,
        );
      }
      return jsonResponse({
        organization_id: ORG_ID,
        vat_number: "DE811907980",
        status: "valid",
        checked_at: "2026-08-14T09:12:00Z",
      });
    });
    render(<VatCheckCard orgId={ORG_ID} />);

    await user.click(
      await screen.findByRole("button", { name: "Check again" }),
    );

    // The register's own words, not a generic failure: "wait a moment" and
    // "something is wrong" send a reader to do different things.
    expect(
      await screen.findByText(/consulted less than 5m0s ago/),
    ).toBeInTheDocument();
    // The answer already on the card still stands, so it stays on screen.
    expect(screen.getByText("DE811907980")).toBeInTheDocument();
  });

  it("offers no ask to a reader who cannot change the company", async () => {
    stubFetch(async (request) =>
      new URL(request.url).pathname.endsWith("/me")
        ? jsonResponse(meFixture({ allow: { organization: ["read"] } }))
        : jsonResponse({
            organization_id: ORG_ID,
            vat_number: "DE811907980",
            status: "valid",
            checked_at: "2026-08-14T09:12:00Z",
          }),
    );
    render(<VatCheckCard orgId={ORG_ID} />);

    // The verdict is readable — withholding the ask is not withholding the
    // record — and the ask is not offered.
    expect(await screen.findByText("DE811907980")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Check/ })).toBeNull();
  });
});
