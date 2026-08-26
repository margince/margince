/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { AppErrorBoundary } from "../app/errorboundary";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./organizations";

// What the company record page does when its composite read FAILS.
//
// The 360 is one request behind most of the page, and the account row is
// another: the header's name, the tab strip and the record's identity come from
// `GET /organizations/{id}`, while the strip, the tab bodies and the rail come
// from `GET /organizations/{id}/360`. So a failed 360 has an honest reading —
// the header stands, and each section says it could not be loaded — and the
// page has no business throwing the whole tree away and taking the one fact
// still knowable (which record you are on) with it.
//
// These mount the page INSIDE the app's own error boundary on purpose. Without
// it a render throw fails the test with a stack, which is a clearer signal but
// not the product's behaviour; with it, the test asserts what a reader gets,
// which is the thing that regressed.

type Organization = components["schemas"]["Organization"];

const org: Organization = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const emptyPage = { has_more: false, next_cursor: null };

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Every read the page makes answers, EXCEPT the 360, which refuses with the
// status under test. A harness that also broke the account read would be
// measuring a different page — one with no identity to keep.
function stubWith360Status(status: number) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/360")) {
        return jsonResponse(
          { type: "about:blank", title: "Server error", status },
          status,
        );
      }
      if (pathname.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({ allow: { organization: ["read", "update"] } }),
        );
      }
      if (pathname.endsWith("/organizations/o-1")) {
        return jsonResponse(org);
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
}

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

function renderCompany(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AppErrorBoundary>{ui}</AppErrorBoundary>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the company record page when its 360 read fails", () => {
  // Every status a real installation produces this way: an outage, a row-scope
  // refusal, and a record that is gone. All three used to reach the same
  // whole-page boundary, which tells the reader nothing about which happened.
  it.each([500, 403, 404])(
    "keeps the record's own heading when the 360 answers %i",
    async (status) => {
      stubWith360Status(status);
      renderCompany(<CompanyScreen id="o-1" />);

      // The h1 comes off the ACCOUNT read, which succeeded. Losing it is the
      // symptom: a reader dropped onto "This view stopped working" cannot even
      // tell which company they were looking at.
      const heading = await screen.findByRole("heading", { level: 1 });
      expect(heading.textContent).toContain("Brandt Automotive GmbH");
      expect(screen.queryByText("This view stopped working.")).toBeNull();
    },
  );

  it("says the sections could not be loaded rather than that there is nothing", async () => {
    // The other half of degrading honestly. An empty state on a failed read is
    // the worse reading of the two: "no people on this account" is a claim, and
    // this page has not earned it. `sectionState` draws `unavailable` for
    // exactly this case, so at least one section must be saying so.
    stubWith360Status(500);
    renderCompany(<CompanyScreen id="o-1" />);

    await screen.findByRole("heading", { level: 1 });
    const unavailable = await screen.findAllByText(
      "Could not be loaded — this may not be the whole picture",
    );
    expect(unavailable.length).toBeGreaterThan(0);
  });
});
