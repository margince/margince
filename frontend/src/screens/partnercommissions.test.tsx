/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";

type CommissionEntry = components["schemas"]["CommissionEntry"];

import { decisionsFor } from "./commissiondecide";
import {
  formatRate,
  outstandingByCurrency,
  PartnerCommissions,
} from "./partnercommissions";

// The commission panel on a partner's company page: what the margin tier one
// card up has actually produced. A tier shown without the money it earned is a
// number nobody can check, which is the whole reason this panel exists.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

// The principal the decision controls ask about. `useCanWrite` folds two
// questions — does this role hold the grant, and is this a full seat — so a
// double answering only one of them would let a control through that the real
// page refuses.
function me(mayDecide: boolean) {
  return {
    user: { id: "u1" },
    authorization: {
      seat_type: mayDecide ? "full" : "read_only",
      objects: {
        commission: {
          create: false,
          read: true,
          update: mayDecide,
          delete: false,
        },
      },
    },
  };
}

// Routes /me separately from the ledger: answering both from one payload is
// how a capability check ends up reading a commission page and silently
// deciding nobody may do anything.
function stubCommissions(entries: unknown[], mayDecide = true) {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      urls.push(request.url);
      const body = new URL(request.url).pathname.endsWith("/me")
        ? me(mayDecide)
        : { data: entries, page: {} };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return urls;
}

const accrued: CommissionEntry = {
  id: "c-1",
  deal_id: "d-1",
  partner_org_id: "o-1",
  status: "accrued",
  attribution_at_accrual: "sourced",
  margin_tier_at_accrual: "tier2_20",
  rate_bps: 2000,
  basis_amount_minor: 100000,
  currency: "EUR",
  amount_minor: 20000,
  captured_by: "human:x",
  version: 1,
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
};

describe("the commission panel", () => {
  it("reads only this partner's entries", async () => {
    const urls = stubCommissions([accrued]);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");

    expect(new URL(urls[0] ?? "").searchParams.get("partner_org_id")).toBe(
      "o-1",
    );
  });

  it("shows what was earned, on what, and at which rate", async () => {
    stubCommissions([accrued]);

    render(<PartnerCommissions organizationId="o-1" />);
    const ledger = await screen.findByTestId("commission-ledger");
    const cells = [...ledger.querySelectorAll("tbody tr td")].map(
      (c) => c.textContent,
    );

    // 20% of a €1,000 deal, read per cell: asserting the row as one string
    // would pass just as happily with the earned and basis figures swapped.
    // The deal leads, because an entry's first question is "on what?".
    expect(cells[1]).toBe("€200.00");
    // The rate is the tier a human agreed to, not the basis points stored.
    expect(cells[2]).toBe("20%");
    expect(cells[3]).toBe("€1,000.00");
    expect(cells[4]).toContain("Accrued");
  });

  // A ledger of bare figures cannot be reconciled against anything: the entry
  // has to say which deal produced it, and let a reader open that deal.
  it("names the deal an entry was earned on, and links to it", async () => {
    // EntityRef resolves the deal's own name, so the stub has to answer that
    // read too — a reference it cannot name is deliberately not a link.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const body = request.url.includes("/deals/")
          ? { id: "d-1", name: "Northgate rollout" }
          : { data: [accrued], page: { has_more: false } };
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    render(<PartnerCommissions organizationId="o-1" />);
    const ledger = await screen.findByTestId("commission-ledger");

    // The control the design system routes with is a button, not an anchor.
    const link = await within(ledger).findByRole("button", {
      name: "Northgate rollout",
    });
    expect(link).toBeTruthy();
  });

  // A reversal keeps its own row rather than being folded into the entry it
  // cancels: a partner asking "what happened to that one" needs both halves.
  it("keeps a reversed entry visible beside the one it cancels", async () => {
    stubCommissions([
      accrued,
      {
        ...accrued,
        id: "c-2",
        status: "void",
        reversal_of: "c-1",
        void_reason: "the deal was reopened",
      },
    ]);

    render(<PartnerCommissions organizationId="o-1" />);
    const ledger = await screen.findByTestId("commission-ledger");
    const rows = [...ledger.querySelectorAll("tbody tr")];

    expect(rows).toHaveLength(2);
    expect(rows[1]?.textContent).toContain("Reversed");
  });

  // Reading page one and stopping would under-report what a partner earned,
  // silently, which is the worst way for a money figure to be wrong.
  it("follows the cursor rather than showing the first page as the whole ledger", async () => {
    const pages = [
      { data: [accrued], page: { has_more: true, next_cursor: "page-2" } },
      { data: [{ ...accrued, id: "c-2" }], page: { has_more: false } },
    ];
    let call = 0;
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        urls.push(request.url);
        const body = pages[Math.min(call, pages.length - 1)];
        call += 1;
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    render(<PartnerCommissions organizationId="o-1" />);
    const ledger = await screen.findByTestId("commission-ledger");
    const rows = [...ledger.querySelectorAll("tbody tr")];

    expect(rows).toHaveLength(2);
    expect(new URL(urls[1] ?? "").searchParams.get("cursor")).toBe("page-2");
  });

  it("says nothing is earned rather than showing an empty table", async () => {
    stubCommissions([]);

    render(<PartnerCommissions organizationId="o-1" />);

    expect(await screen.findByText("Nothing earned yet")).toBeTruthy();
    expect(screen.queryByTestId("commission-ledger")).toBeNull();
  });
});

describe("formatRate", () => {
  it("renders whole-percent tiers without trailing zeros", () => {
    expect(formatRate(1500, "en")).toBe("15%");
    expect(formatRate(2000, "en")).toBe("20%");
    expect(formatRate(2500, "en")).toBe("25%");
  });

  it("keeps a fraction that a tier genuinely carries", () => {
    expect(formatRate(1250, "en")).toBe("12.5%");
  });

  // A German reader writes "12,5 %". A hand-built string would hand them a
  // decimal point, which is the kind of wrong that reads as a different number.
  it("writes the separator the reader's locale uses", () => {
    expect(formatRate(1250, "de")).toContain(",");
  });
});

// What a reader may DO to an entry, and what the ledger's own lifecycle admits.
//
// The rules live in the commissions store (legalTransitions) and decisionsFor
// mirrors them. Getting the mirror wrong puts a control on screen the server
// refuses, so the reader learns the rule from a 422 instead of from the page.
describe("deciding a commission entry", () => {
  it("offers approve and reverse on an accrued entry, and not pay", () => {
    expect(decisionsFor("accrued")).toEqual(["approve", "void"]);
  });

  it("offers pay once it is approved, and never approve twice", () => {
    expect(decisionsFor("approved")).toEqual(["pay", "void"]);
  });

  it("still lets a paid entry be reversed — money goes out and comes back", () => {
    expect(decisionsFor("paid")).toEqual(["void"]);
  });

  it("offers nothing on a reversed entry, because void is terminal", () => {
    expect(decisionsFor("void")).toEqual([]);
  });

  it("puts the approve control on an accrued row", async () => {
    stubCommissions([accrued]);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");
    // The seat snapshot is its own query, so the verbs appear a tick after
    // the rows do.
    await screen.findByTestId("commission-approve");

    expect(screen.getByTestId("commission-approve")).toBeTruthy();
    expect(screen.getByTestId("commission-void")).toBeTruthy();
    // Paying something nobody approved is the transition the ledger refuses.
    expect(screen.queryByTestId("commission-pay")).toBeNull();
  });

  it("draws no decision control on a reversed row", async () => {
    stubCommissions([{ ...accrued, status: "void" }]);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");

    expect(screen.queryByTestId("commission-approve")).toBeNull();
    expect(screen.queryByTestId("commission-void")).toBeNull();
  });

  it("confirms before it writes — a money decision never fires on one click", async () => {
    const urls = stubCommissions([accrued]);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");
    await screen.findByTestId("commission-approve");
    const before = urls.length;
    await act(async () => {
      screen.getByTestId("commission-approve").click();
    });

    // The dialog is up and nothing has been sent.
    expect(screen.getByTestId("commission-approve-confirm")).toBeTruthy();
    expect(urls.length).toBe(before);
  });
});

// What is still OWED, which is the figure somebody running the programme opens
// this panel for.
describe("the outstanding figure", () => {
  it("counts accrued and approved, and neither paid nor reversed", () => {
    const owed = outstandingByCurrency([
      { ...accrued, status: "accrued", amount_minor: 20000 },
      { ...accrued, id: "c-2", status: "approved", amount_minor: 5000 },
      { ...accrued, id: "c-3", status: "paid", amount_minor: 99900 },
      { ...accrued, id: "c-4", status: "void", amount_minor: 99900 },
    ]);

    expect(owed).toEqual([{ currency: "EUR", amountMinor: 25000 }]);
  });

  it("keeps currencies apart rather than adding them", () => {
    const owed = outstandingByCurrency([
      { ...accrued, amount_minor: 20000, currency: "EUR" },
      { ...accrued, id: "c-2", amount_minor: 3000, currency: "USD" },
    ]);

    // Two slots, not one sum: EUR 200 plus USD 30 is not 230 of anything.
    expect(owed).toEqual([
      { currency: "EUR", amountMinor: 20000 },
      { currency: "USD", amountMinor: 3000 },
    ]);
  });

  it("draws nothing when every entry is settled", async () => {
    stubCommissions([{ ...accrued, status: "paid" }]);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");

    // A slot reading "0" spends a slot saying there is nothing to say.
    expect(screen.queryByTestId("commission-outstanding")).toBeNull();
  });

  it("shows what is owed when something is", async () => {
    stubCommissions([accrued]);

    render(<PartnerCommissions organizationId="o-1" />);
    const strip = await screen.findByTestId("commission-outstanding");

    expect(strip.textContent).toContain("€200.00");
  });
});

// A permission denial is WITHHELD, not absent (design-system README, "Absent,
// disabled, or withheld — decided by CAUSE"). An empty cell and a refused one
// make the same shape on screen and mean opposite things: nothing to decide
// here, versus not yours to decide.
describe("a reader who may not decide", () => {
  it("is told the decision is withheld rather than shown controls that 403", async () => {
    stubCommissions([accrued], false);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");
    await screen.findByTestId("commission-withheld");

    // The verbs are gone, and something stands in their place saying why.
    expect(screen.queryByTestId("commission-approve")).toBeNull();
    expect(screen.queryByTestId("commission-void")).toBeNull();
  });

  it("gets an empty cell on a row with nothing to decide, not a withheld note", async () => {
    // Void is terminal for everybody, so the reason this cell is empty is the
    // entry's state and not the reader's grant.
    stubCommissions([{ ...accrued, status: "void" }], false);

    render(<PartnerCommissions organizationId="o-1" />);
    await screen.findByTestId("commission-ledger");

    expect(screen.queryByTestId("commission-withheld")).toBeNull();
  });
});
