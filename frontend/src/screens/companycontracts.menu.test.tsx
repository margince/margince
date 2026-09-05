/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { CompanyContractsCard } from "./companycontracts";

// margince#3286: the row menu offered Edit and Archive only. Renew, change
// status and record-cancellation reach the same POST /contracts/{id}/renewal
// /status /cancellation the backend has carried since #3230 — this proves the
// menu actually offers them, and withholds "change status" from a row whose
// status is terminal (refuseInvalidTransition admits no transition out of one
// but a same-status no-op, so the control would only ever refuse — the
// reasoning #3573/#3700 apply elsewhere). Cancel is NOT withheld there:
// Store.Cancel is a plain column patch with no status check at all.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVE = {
  id: "c-1",
  organization_id: "o-1",
  title: "Framework agreement 2024",
  source: "manual",
  captured_by: "human:u-1",
  status: "active",
  under_contract: true,
  auto_renew: false,
  value_basis: "total",
  version: 3,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const EXPIRED = { ...ACTIVE, id: "c-2", status: "expired" };
const RENEWED_DEAL = { id: "d-1", name: "2025 renewal" };

const GRANTED = meFixture({
  allow: { contract: ["read", "create", "update", "delete"] },
});

function stub(contracts: unknown[], me: unknown = GRANTED) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/me")
        ? me
        : url.includes("/documents")
          ? { data: [] }
          : url.includes("/deals/")
            ? RENEWED_DEAL
            : { data: contracts };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function show(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the contract row menu", () => {
  it("offers renew, change status and cancel on a live agreement", async () => {
    stub([ACTIVE]);
    const user = userEvent.setup();
    show(<CompanyContractsCard orgId="o-1" />);

    await user.click(
      await screen.findByRole("button", { name: "Contract actions" }),
    );

    expect(screen.getByRole("button", { name: "Renew" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Change status" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Cancel agreement" }),
    ).toBeTruthy();
  });

  it("withholds change-status and cancel from a terminal agreement, but not renew", async () => {
    stub([EXPIRED]);
    const user = userEvent.setup();
    show(<CompanyContractsCard orgId="o-1" />);

    await user.click(
      await screen.findByRole("button", { name: "Contract actions" }),
    );

    // A terminal status has no valid transition out of it but superseded, and
    // renewing a non-superseded terminal agreement (expired, cancelled) is
    // still the normal way a lapsed agreement gets a successor.
    expect(screen.getByRole("button", { name: "Renew" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Change status" })).toBeNull();
    // Cancel is offered even here: Store.Cancel (contract_lifecycle.go) is a
    // two-column patch with NO status check at all — unlike ChangeStatus, it
    // never refuses on a terminal row, so a form withholding it here would
    // refuse more than the server does.
    expect(
      screen.getByRole("button", { name: "Cancel agreement" }),
    ).toBeTruthy();
  });

  it("withholds renew from an already-superseded agreement", async () => {
    stub([{ ...ACTIVE, status: "superseded" }]);
    const user = userEvent.setup();
    show(<CompanyContractsCard orgId="o-1" />);

    await user.click(
      await screen.findByRole("button", { name: "Contract actions" }),
    );

    // refuseRenewalOfTerminal (contract_lifecycle.go): the ONE status renewal
    // itself refuses is superseded — a chain must stay single-headed.
    expect(screen.queryByRole("button", { name: "Renew" })).toBeNull();
  });

  it("withholds renew from a seat holding only update, not create", async () => {
    // Store.Renew (contract_lifecycle.go) asserts BOTH contract:create and
    // contract:update in one call — it both creates the successor and
    // supersedes the predecessor. A seat holding update alone would see every
    // press of Renew refused by the server.
    stub([ACTIVE], meFixture({ allow: { contract: ["read", "update"] } }));
    const user = userEvent.setup();
    show(<CompanyContractsCard orgId="o-1" />);

    await user.click(
      await screen.findByRole("button", { name: "Contract actions" }),
    );

    expect(screen.queryByRole("button", { name: "Renew" })).toBeNull();
    expect(screen.getByRole("button", { name: "Change status" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Cancel agreement" }),
    ).toBeTruthy();
  });

  it("offers no menu at all to a seat with no write grants", async () => {
    stub([ACTIVE], meFixture({ allow: { contract: ["read"] } }));
    show(<CompanyContractsCard orgId="o-1" />);

    await screen.findByText("Framework agreement 2024");
    expect(
      screen.queryByRole("button", { name: "Contract actions" }),
    ).toBeNull();
  });
});

// margince#3286 (re-measured): a renewal made through the screen could not
// name the deal that won it, and no row said which deal a contract WAS
// linked to even where one existed. These prove the row's own display half.
describe("the contract row's linked deal", () => {
  it("names the deal a contract carries", async () => {
    stub([{ ...ACTIVE, deal_id: RENEWED_DEAL.id }]);
    show(<CompanyContractsCard orgId="o-1" />);

    await screen.findByText("Framework agreement 2024");
    expect(await screen.findByText(RENEWED_DEAL.name)).toBeTruthy();
  });

  it("shows nothing where a contract carries no deal", async () => {
    stub([ACTIVE]);
    show(<CompanyContractsCard orgId="o-1" />);

    await screen.findByText("Framework agreement 2024");
    expect(screen.queryByText(RENEWED_DEAL.name)).toBeNull();
    expect(screen.queryByText("Deal")).toBeNull();
  });
});
