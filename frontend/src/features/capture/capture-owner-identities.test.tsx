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
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { OwnerIdentitiesCard } from "./capture-owner-identities";

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

  // An empty list is said on the add row itself, not as a line of its own.
  expect(
    (await screen.findByTestId("owner-identities-empty")).closest(
      ".settingrow",
    ),
  ).toBe(
    screen
      .getByRole("button", { name: en["ownerIdentities.add"] })
      .closest(".settingrow"),
  );
  await user.click(
    screen.getByRole("button", { name: en["ownerIdentities.add"] }),
  );
  // That a declaration is not retroactive is said where it is made.
  expect(
    within(screen.getByRole("dialog")).getByText(
      en["ownerIdentities.notRetroactive"],
    ),
  ).toBeTruthy();
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

it("offers the add verb while the addresses are still loading", async () => {
  showing([], {
    "GET /capture/owner-identities": () => new Promise<Response>(() => {}),
  });

  const list = await screen.findByTestId("owner-identities-list");
  expect(
    within(list).getByRole("button", { name: en["ownerIdentities.add"] }),
  ).toBeTruthy();
});

it("shows when each address was declared, under the address", async () => {
  showing([identity({})]);

  const list = await screen.findByTestId("owner-identities-list");
  expect(
    await within(list).findByText(en["ownerIdentities.kind.address"]),
  ).toBeTruthy();
  expect(within(list).getByText(/2026/)).toBeTruthy();
});

function removeVerb() {
  return screen.findByRole("button", {
    name: "Remove founder@previous-employer.example",
  });
}

// Restoring the withdrawn row is the exact inverse, learned or typed alike. A
// learned alias is refused from then on, and its notice says so.
it.each([
  ["user", "ownerIdentities.removedToast"],
  ["delivered_to", "ownerIdentities.removedLearnedToast"],
] as const)(
  "removes a %s address at once and puts that same row back through Undo",
  async (source, notice) => {
    const user = userEvent.setup();
    const removed = vi.fn(() => new Response(null, { status: 204 }));
    const restored = vi.fn(() => jsonResponse(identity({ source })));
    showing([identity({ source })], {
      "DELETE /capture/owner-identities/oi-1": removed,
      "POST /capture/owner-identities/oi-1/restore": restored,
    });

    await user.click(await removeVerb());
    expect(screen.queryByRole("dialog")).toBeNull();
    const said = await screen.findByRole("status");
    await waitFor(() => expect(said.textContent).toContain(en[notice]));
    expect(removed).toHaveBeenCalledTimes(1);

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );
    await waitFor(() => expect(restored).toHaveBeenCalledTimes(1));
    expect(
      await screen.findByText(en["ownerIdentities.restoredToast"]),
    ).toBeTruthy();
  },
);

// Each refusal says what became of the address, in the reader's language.
it.each([
  [404, "not_found", "ownerIdentities.restoreGone"],
  [409, "removal_moved_on", "ownerIdentities.restoreMovedOn"],
  [409, "identity_declared_again", "ownerIdentities.restoreDeclaredAgain"],
] as const)(
  "words a %i %s refusal of the Undo for the reader",
  async (status, code, copy) => {
    const user = userEvent.setup();
    const listed = vi.fn(() => jsonResponse({ data: [identity({})] }));
    showing([identity({})], {
      "GET /capture/owner-identities": listed,
      "DELETE /capture/owner-identities/oi-1": () =>
        new Response(null, { status: 204 }),
      "POST /capture/owner-identities/oi-1/restore": () =>
        jsonResponse(
          { code, status, detail: "An English sentence for the API." },
          status,
        ),
    });

    await user.click(await removeVerb());
    const undo = await screen.findByRole("button", { name: en["common.undo"] });
    await waitFor(() => expect(listed).toHaveBeenCalledTimes(2));
    await user.click(undo);

    expect(await screen.findByText(en[copy])).toBeTruthy();
    expect(screen.queryByText(/An English sentence/)).toBeNull();
    // The list reads again, so a row already back shows as back.
    await waitFor(() => expect(listed).toHaveBeenCalledTimes(3));
  },
);

// The removed row takes its verb with it, so focus moves on to the next one.
it("hands focus to the next address once one is gone", async () => {
  const user = userEvent.setup();
  const rows = [
    identity({ source: "delivered_to" }),
    identity({ id: "oi-2", value: "me@side-project.example" }),
  ];
  showing(rows, {
    "DELETE /capture/owner-identities/oi-1": () => {
      rows.shift();
      return new Response(null, { status: 204 });
    },
  });

  await user.click(await removeVerb());
  const next = screen.getByRole("button", {
    name: "Remove me@side-project.example",
  });
  await waitFor(() => expect(document.activeElement).toBe(next));
});

it("returns focus to the add verb once the last address is gone", async () => {
  const user = userEvent.setup();
  const rows = [identity({})];
  showing(rows, {
    "DELETE /capture/owner-identities/oi-1": () => {
      rows.shift();
      return new Response(null, { status: 204 });
    },
  });

  await user.click(await removeVerb());
  expect(await screen.findByTestId("owner-identities-empty")).toBeTruthy();
  const add = screen.getByRole("button", { name: en["ownerIdentities.add"] });
  await waitFor(() => expect(document.activeElement).toBe(add));
});
