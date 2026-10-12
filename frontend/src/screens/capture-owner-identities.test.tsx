/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "./story-utils";

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
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
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

function showing(identities: Record<string, unknown>[], routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/owner-identities": () => jsonResponse({ data: identities }),
    ...routes,
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

it("shows when each address was declared, under the address", async () => {
  showing([identity({})]);

  const list = await screen.findByTestId("owner-identities-list");
  expect(
    within(list).getByText(en["ownerIdentities.kind.address"]),
  ).toBeTruthy();
  expect(within(list).getByText(/2026/)).toBeTruthy();
});

// Declaring a typed address again is the exact inverse, so it goes at once.
it("removes a typed address at once and declares it again through Undo", async () => {
  const user = userEvent.setup();
  const removed = vi.fn(() => new Response(null, { status: 204 }));
  // The restored row is the same address, so the reply need not echo the body.
  const declared = vi.fn((_body: unknown) =>
    jsonResponse(identity({ id: "oi-2" }), 201),
  );
  showing([identity({})], {
    "DELETE /capture/owner-identities/oi-1": removed,
    "POST /capture/owner-identities": declared,
  });

  await user.click(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  const said = await screen.findByRole("status");
  await waitFor(() =>
    expect(said.textContent).toContain(en["ownerIdentities.removedToast"]),
  );
  expect(removed).toHaveBeenCalledTimes(1);

  await user.click(
    await within(said).findByRole("button", { name: en["common.undo"] }),
  );
  await waitFor(() =>
    expect(declared).toHaveBeenCalledWith({
      kind: "address",
      value: "founder@previous-employer.example",
    }),
  );
});

// Declared again, a learned address would read as typed: it asks first instead.
it("asks before removing an address the product learned", async () => {
  const user = userEvent.setup();
  const removed = vi.fn(() => new Response(null, { status: 204 }));
  showing([identity({ source: "delivered_to" })], {
    "DELETE /capture/owner-identities/oi-1": removed,
  });

  await user.click(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(removed).not.toHaveBeenCalled();
  await user.click(
    within(dialog).getByRole("button", {
      name: en["ownerIdentities.removeConfirm"],
    }),
  );
  await waitFor(() => expect(removed).toHaveBeenCalledTimes(1));
});

// The removed row takes its verb with it, so focus moves on to the next one.
it("hands focus to the next address once a typed one is gone", async () => {
  const user = userEvent.setup();
  const rows = [
    identity({}),
    identity({ id: "oi-2", value: "me@side-project.example" }),
  ];
  showing(rows, {
    "DELETE /capture/owner-identities/oi-1": () => {
      rows.shift();
      return new Response(null, { status: 204 });
    },
  });

  await user.click(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  );
  const next = screen.getByRole("button", {
    name: "Remove me@side-project.example",
  });
  await waitFor(() => expect(document.activeElement).toBe(next));
});

// The confirm outlives its row, so it names where focus lands instead.
it("returns focus to the add verb after a confirmed removal empties the list", async () => {
  const user = userEvent.setup();
  const rows = [identity({ source: "delivered_to" })];
  showing(rows, {
    "DELETE /capture/owner-identities/oi-1": () => {
      rows.shift();
      return new Response(null, { status: 204 });
    },
  });

  await user.click(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", {
      name: en["ownerIdentities.removeConfirm"],
    }),
  );
  const add = screen.getByRole("button", { name: en["ownerIdentities.add"] });
  await waitFor(() => expect(document.activeElement).toBe(add));
  expect(await screen.findByTestId("owner-identities-empty")).toBeTruthy();
});

it("keeps the address in the confirm's title while it closes", async () => {
  const user = userEvent.setup();
  showing([identity({ source: "delivered_to" })]);

  await user.click(
    await screen.findByRole("button", {
      name: "Remove founder@previous-employer.example",
    }),
  );
  const dialog = await screen.findByRole("dialog");
  const title = within(dialog).getByRole("heading");
  await user.keyboard("{Escape}");
  expect(title.textContent).toContain("founder@previous-employer.example");
});
