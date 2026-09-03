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
// menu actually offers them, and withholds "change status" / "cancel" from a
// row whose status is terminal (no valid transition exists from one, so the
// control would only ever refuse — the reasoning #3573/#3700 apply elsewhere).

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

const GRANTED = meFixture({
  allow: { contract: ["read", "create", "update", "delete"] },
});

function stub(contracts: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/me")
        ? GRANTED
        : url.includes("/documents")
          ? { data: [] }
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
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
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
    expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
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
});
