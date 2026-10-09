/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// Where a claim came from is only worth saying when the seat did not make it.
//
// An address somebody typed needs no explanation. One the product LEARNED does:
// a seat scanning this card would otherwise find an address they never entered
// and have no way to tell whether they forgot adding it or something else did —
// and they are the colleague who decides whether it stays.

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
    await screen.findByText(en["ownerIdentities.learned.deliveredTo"]),
  ).toBeTruthy();
});

it("says nothing about a row the seat declared themselves", async () => {
  showing([identity({ source: "user" })]);

  await screen.findByText("founder@previous-employer.example");
  expect(
    screen.queryByText(en["ownerIdentities.learned.deliveredTo"]),
  ).toBeNull();
  expect(screen.queryByText(en["ownerIdentities.learned.provider"])).toBeNull();
});

// The two learned sources are different facts and say different things: one is
// the receiving server's delivery, the other the provider's own send-as list.
it("tells a provider-attested address from a discovered one", async () => {
  showing([identity({ source: "provider" })]);

  expect(
    await screen.findByText(en["ownerIdentities.learned.provider"]),
  ).toBeTruthy();
  expect(
    screen.queryByText(en["ownerIdentities.learned.deliveredTo"]),
  ).toBeNull();
});

// Every row's verb is the same glyph, so its name says which address it takes back.
it("names the remove verb after the address it withdraws", async () => {
  showing([identity({})]);

  expect(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  ).toBeTruthy();
});

// The server's refusal is a developer's English; the reader gets the catalog's.
it("says what an address must look like when the server refuses it", async () => {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/owner-identities": () => jsonResponse({ data: [] }),
    "POST /capture/owner-identities": () =>
      jsonResponse(
        {
          code: "validation_error",
          status: 422,
          detail: "give one email address of at most 320 characters",
          details: {
            errors: [
              {
                field: "value",
                code: "invalid_exclusion",
                message: "give one email address of at most 320 characters",
              },
            ],
          },
        },
        422,
      ),
  });
  render(<OwnerIdentitiesCard />);
  const user = userEvent.setup();

  await user.click(
    await screen.findByRole("button", { name: en["ownerIdentities.add"] }),
  );
  await user.type(
    screen.getByRole("textbox", { name: en["ownerIdentities.valueLabel"] }),
    "not an address",
  );
  await user.click(
    screen.getByRole("button", { name: en["ownerIdentities.confirm"] }),
  );

  expect(
    await screen.findByText(en["captureValue.refusedAddress"]),
  ).toBeTruthy();
  expect(screen.queryByText(/give one email address/)).toBeNull();
});
