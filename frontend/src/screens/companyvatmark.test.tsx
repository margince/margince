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
import { VatMark } from "./companyvatmark";

// What this mark must get right is not the layout but the DISTINCTIONS: a
// verdict is readable without opening anything, a receipt is what proves the
// check to a tax authority, and a register that did not answer says nothing
// about the company. Collapsing any of those is what would mislead somebody
// filing a return.
//
// It replaced a card two tabs away behind a collapsed section, so the case that
// matters most is the one the old surface could not serve: the verdict beside
// the number, with no click.

const ORG_ID = "00000000-0000-7000-8000-0000000000a1";
const NUMBER = "DE811907980";

const CHECKED = {
  organization_id: ORG_ID,
  vat_number: NUMBER,
  status: "valid",
  consultation_number: "WAPIAAAAXk3rN2p9",
  registered_name: "Muster Handels GmbH",
  checked_at: "2026-08-14T09:12:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Every call the mark made, so a test can assert the METHOD and the PATH rather
// than that something happened.
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

// The card's own read, plus the grant read the ask button gates on. Without a
// /me answer every viewer reads as unable to write, and the button would be
// absent for the correct reason — which is how a test passes proving nothing.
function answerWith(body: unknown, status = 200) {
  return stubFetch(async (request) =>
    new URL(request.url).pathname.endsWith("/me")
      ? jsonResponse(meFixture({ allow: { organization: ["read", "update"] } }))
      : jsonResponse(body, status),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function mark(stated = NUMBER) {
  return <VatMark orgId={ORG_ID} stated={stated} />;
}

describe("the VAT mark beside the number", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("says the verdict without anything being opened", async () => {
    answerWith(CHECKED);

    render(mark());

    // The reason this replaced a card on another tab: a rep looking at the
    // number sees whether it holds up, in the same glance.
    expect(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    ).toBeInTheDocument();
  });

  it("names an invalid number as the finding it is", async () => {
    // A copied imprint states somebody else's VAT ID. That is what this exists
    // to surface, and it must not need a click.
    answerWith({ ...CHECKED, status: "invalid", consultation_number: null });

    render(mark());

    expect(
      await screen.findByRole("button", { name: "VAT ID: Not valid" }),
    ).toBeInTheDocument();
  });

  it("says a company nobody has consulted is unchecked", async () => {
    answerWith({}, 404);

    render(mark());

    // Not a verdict. "We have not asked" and "we asked and were told no" are
    // different facts, and the mark must not spend the second on the first.
    expect(
      await screen.findByRole("button", {
        name: "VAT ID: not checked with the register yet",
      }),
    ).toBeInTheDocument();
  });

  it("opens to the receipt that proves the check", async () => {
    const user = userEvent.setup();
    answerWith(CHECKED);
    render(mark());

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );

    // The consultation number is the half a tax authority accepts; the
    // registered name is what exposes a number belonging to another company.
    expect(await screen.findByText("WAPIAAAAXk3rN2p9")).toBeVisible();
    expect(screen.getByText("Muster Handels GmbH")).toBeVisible();
  });

  it("asks the register when a person presses the button", async () => {
    const user = userEvent.setup();
    const calls = answerWith(CHECKED);
    render(mark());

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Check again" }),
    );

    await waitFor(() => {
      expect(
        calls.some(
          (one) =>
            one.method === "POST" &&
            one.pathname === `/v1/organizations/${ORG_ID}/vat-check`,
        ),
      ).toBe(true);
    });
  });

  it("does not report a refused request as asked", async () => {
    const user = userEvent.setup();
    // 403 with NO BODY, the shape that matters: openapi-fetch has nothing to
    // parse, so `error` comes back falsy and a handler checking it alone
    // reports a refusal as success.
    stubFetch(async (request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { organization: ["read", "update"] } }),
        );
      }
      if (request.method === "POST") {
        return new Response(null, { status: 403 });
      }
      return jsonResponse(CHECKED);
    });
    render(mark());

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Check again" }),
    );

    await waitFor(() => {
      expect(
        screen.queryByText(/answer appears here once it replies/),
      ).toBeNull();
    });
  });

  it("says so when the number moved after the check", async () => {
    const user = userEvent.setup();
    answerWith(CHECKED);

    // The row above this mark is editable, so a receipt can end up beside a
    // number nobody ever consulted. A verdict alone would then be a lie about
    // the number the reader is looking at.
    render(mark("DE999999999"));

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );

    expect(
      await screen.findByText(/number on this record has changed/),
    ).toBeVisible();
  });

  it("offers no ask to a reader who cannot change the company", async () => {
    const user = userEvent.setup();
    stubFetch(async (request) =>
      new URL(request.url).pathname.endsWith("/me")
        ? jsonResponse(meFixture({ allow: { organization: ["read"] } }))
        : jsonResponse(CHECKED),
    );
    render(mark());

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );

    // The verdict is theirs to read — withholding the ask is not withholding
    // the record.
    expect(await screen.findByText("WAPIAAAAXk3rN2p9")).toBeVisible();
    expect(screen.queryByRole("button", { name: /Check/ })).toBeNull();
  });

  it("draws nothing while the read is in flight", () => {
    stubFetch(
      () =>
        new Promise(() => {
          // Never settles: the state between mount and the first answer.
        }),
    );

    render(mark());

    // A glyph that appeared a beat later, under the eye of somebody reading the
    // number, is worse than one that arrives with the row.
    expect(screen.queryByRole("button", { name: /VAT ID:/ })).toBeNull();
  });
});
