/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { useMe } from "./common";
import { CompanyRejectAction } from "./companyreject";

// Every capability predicate reads false while /me is in flight, so an absence
// asserted on the first frame passes whatever the grants say. This is the row
// only a RESOLVED snapshot draws, and every absence case waits on it first.
function SnapshotResolved() {
  return useMe().data ? <span data-testid="me-resolved" /> : null;
}

// "This is not a company", from the reader's side.
//
// The first attempt at this was two HTTP calls from here, and three of the four
// defects adversarial review found were about this component rather than the
// server: the control was offered without the archive grant, it sent a domain
// the page happened to be holding, and a half-completed pair reported total
// failure. The server does the whole thing in one transaction now, so what is
// left to hold here is that the control asks for it ONCE, asks only when both
// halves are available, and reports the answer the server actually gave.

type Organization = components["schemas"]["Organization"];

// Typed, not asserted: a fixture cast into the contract type can drop a
// required field and go on compiling while the wire shape moves under it.
const ORG: Organization = {
  writable: true,
  id: "o-1",
  display_name: "Expensify Ltd",
  owner_id: "u-owner",
  captured_by: "human:u-author",
  source: "manual",
  version: 4,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  domains: [
    {
      id: "d-1",
      domain: "expensify.test",
      is_primary: true,
      source: "capture",
      captured_by: "connector:gmail",
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Every request the component made, so a case can assert there was exactly
// ONE — the property that replaced the two-call version and the only one a
// count can see.
type Sent = { method: string; url: string; body: unknown };

function stub(allow: GrantSpec, reject: (body: unknown) => Response): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname.endsWith("/me")) {
        return new Response(JSON.stringify(meFixture({ allow })), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      const body = request.body ? await request.clone().json() : null;
      sent.push({ method: request.method, url: url.pathname, body });
      return reject(body);
    }),
  );
  return sent;
}

const rejected = () =>
  new Response(
    JSON.stringify({
      organization: { ...ORG, archived_at: "2026-09-08T10:00:00Z" },
      domain: {
        // Deliberately NOT the domain the fixture above shows. The server reads
        // the company's current primary inside the transaction, so the two can
        // differ — and the reader must be told which one was actually refused.
        domain: "mail.expensify.test",
        admission: "suppressed",
        reason: "a tool we use",
        source: "human",
        decided_at: "2026-09-08T10:00:00Z",
      },
    }),
    { status: 200, headers: { "content-type": "application/json" } },
  );

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The whole write, in one request, with the reason the caller typed and the
// version the page was holding.
it("rejects a company in one request and reports the domain the server refused", async () => {
  const user = userEvent.setup();
  const sent = stub({ organization: ["read", "update", "delete"] }, rejected);
  render(<CompanyRejectAction org={ORG} />);

  await user.click(await screen.findByTestId("reject-company"));
  await user.type(
    screen.getByLabelText(/why is this not a company/i),
    "a tool we use",
  );
  await user.click(screen.getByTestId("reject-company-confirm"));

  // ONE. Two calls is the defect this verb exists to end, and a count is the
  // only assertion that sees it: both orders of the old pair produced a
  // plausible screen for one of the two failure cases.
  expect(await screen.findByRole("status")).toBeInTheDocument();
  expect(sent).toHaveLength(1);
  expect(sent[0]?.method).toBe("POST");
  expect(sent[0]?.url).toBe("/v1/organizations/o-1/reject");
  // The reason travels; the DOMAIN does not. A domain in the body is the stale
  // snapshot defect, and this is the case that fails if one is added back.
  expect(sent[0]?.body).toEqual({ reason: "a tool we use" });

  // And the reader is told which domain was refused — the server's answer, not
  // the one this page was showing when they pressed.
  expect(screen.getByRole("status")).toHaveTextContent("mail.expensify.test");
});

// The grant defect, both halves. A seat holding one of them saw the control,
// pressed it, and was refused after the first of two writes had landed.
it.each([
  ["only the archive grant", { organization: ["read", "delete"] }],
  ["only the domain grant", { organization: ["read", "update"] }],
])("draws nothing for a seat holding %s", async (_name, allow) => {
  stub(allow as GrantSpec, rejected);
  render(
    <>
      <CompanyRejectAction org={ORG} />
      <SnapshotResolved />
    </>,
  );

  await screen.findByTestId("me-resolved");
  expect(screen.queryByTestId("reject-company")).toBeNull();
});

// A company somebody typed in by hand was never derived from mail, so there is
// nothing a refusal would stop. The server refuses it; a control that is only
// ever refused should not be drawn.
it("draws nothing for a company with no primary domain", async () => {
  stub({ organization: ["read", "update", "delete"] }, rejected);
  render(
    <>
      <CompanyRejectAction org={{ ...ORG, domains: [] }} />
      <SnapshotResolved />
    </>,
  );

  await screen.findByTestId("me-resolved");
  expect(screen.queryByTestId("reject-company")).toBeNull();
});

// The reason is required by the contract, so the control says so before the
// server has to.
it("will not send until a reason is written", async () => {
  const user = userEvent.setup();
  const sent = stub({ organization: ["read", "update", "delete"] }, rejected);
  render(<CompanyRejectAction org={ORG} />);

  await user.click(await screen.findByTestId("reject-company"));
  expect(screen.getByTestId("reject-company-confirm")).toBeDisabled();
  await user.type(screen.getByLabelText(/why is this not a company/i), "   ");
  expect(screen.getByTestId("reject-company-confirm")).toBeDisabled();
  expect(sent).toHaveLength(0);
});
