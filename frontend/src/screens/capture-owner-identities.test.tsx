/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// Where a claim came from is said on every row, so a seat can tell an address
// they typed from one Margince learned, and the empty card offers the one verb.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function identity(over: Record<string, unknown>) {
  return {
    id: "oi-1",
    kind: "address",
    value: "founder@previous-employer.example",
    source: "user",
    created_at: "2026-09-01T09:00:00Z",
    ...over,
  };
}

function showing(identities: Record<string, unknown>[]) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/owner-identities": () => jsonResponse({ data: identities }),
  });
  render(<OwnerIdentitiesCard />);
}

it("says a discovered alias was found rather than typed", async () => {
  showing([identity({ source: "delivered_to" })]);

  expect(
    await screen.findByText(en["ownerIdentities.source.deliveredTo"]),
  ).toBeTruthy();
  expect(screen.queryByText(en["ownerIdentities.source.user"])).toBeNull();
});

it("marks a row the seat declared themselves as theirs", async () => {
  showing([identity({ source: "user" })]);

  await screen.findByText("founder@previous-employer.example");
  expect(screen.getByText(en["ownerIdentities.source.user"])).toBeTruthy();
});

// The two learned sources are different facts: one is the receiving server's
// delivery, the other the provider's own send-as list.
it("tells a provider-attested address from a discovered one", async () => {
  showing([identity({ source: "provider" })]);

  expect(
    await screen.findByText(en["ownerIdentities.source.provider"]),
  ).toBeTruthy();
  expect(
    screen.queryByText(en["ownerIdentities.source.deliveredTo"]),
  ).toBeNull();
});

it("offers Add address from the empty state, and nowhere else", async () => {
  showing([]);

  expect(await screen.findByTestId("owner-identities-empty")).toBeTruthy();
  expect(
    screen.getAllByRole("button", { name: en["ownerIdentities.add"] }),
  ).toHaveLength(1);
  expect(screen.queryByRole("table")).toBeNull();
});

it("names the address on its remove verb", async () => {
  showing([identity({})]);

  expect(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  ).toBeTruthy();
  expect(
    screen.getAllByRole("button", { name: en["ownerIdentities.add"] }),
  ).toHaveLength(1);
});
