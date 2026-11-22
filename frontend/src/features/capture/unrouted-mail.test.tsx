/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
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
import { UnroutedMailCard } from "./unrouted-mail";

// The reader answers per address: "I receive this address" is the ordinary
// own-address claim, and "Not mine" keeps the mail hidden. Both can be undone.

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

const INFO = "info@colleague.example";
const REFUSAL = "/capture/unrouted-mail/info%40colleague.example/refusal";

function showing(routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/unrouted-mail": () =>
      jsonResponse({
        messages: 3,
        data: [
          { address: INFO, messages: 2 },
          { address: "security@colleague.example", messages: 1 },
        ],
      }),
    ...routes,
  });
  render(<UnroutedMailCard />);
}

it("lists the mail per address with its message count", async () => {
  showing();

  expect(await screen.findByText(INFO)).toBeTruthy();
  expect(screen.getByText("2 messages")).toBeTruthy();
  expect(screen.getByText("1 message")).toBeTruthy();
  expect(screen.getByTestId("unrouted-mail-summary").textContent).toBe(
    "3 messages in your mailbox were sent to addresses that are not yours.",
  );
});

it("shows nothing when the mailbox holds no such mail", async () => {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/unrouted-mail": () => jsonResponse({ messages: 0, data: [] }),
  });
  const { container } = rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <UnroutedMailCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  await waitFor(() => expect(container.textContent).toBe(""));
});

it("claims the address through the own-address writer and offers an undo", async () => {
  const claimed = vi.fn();
  const withdrawn = vi.fn();
  showing({
    "POST /capture/owner-identities": (body) => {
      claimed(body);
      return jsonResponse(
        {
          id: "oi-9",
          kind: "address",
          value: INFO,
          source: "user",
          created_at: "2026-11-22T09:00:00Z",
        },
        201,
      );
    },
    "DELETE /capture/owner-identities/oi-9": () => {
      withdrawn();
      return new Response(null, { status: 204 });
    },
  });

  await userEvent.click(
    await screen.findByRole("button", {
      name: `${en["unroutedMail.receive"]}: ${INFO}`,
    }),
  );
  await waitFor(() =>
    expect(claimed).toHaveBeenCalledWith({ kind: "address", value: INFO }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: en["common.undo"] }),
  );
  await waitFor(() => expect(withdrawn).toHaveBeenCalledOnce());
});

it("records not mine for the address and takes it back on undo", async () => {
  const refused = vi.fn();
  const restored = vi.fn();
  showing({
    [`PUT ${REFUSAL}`]: () => {
      refused();
      return new Response(null, { status: 204 });
    },
    [`DELETE ${REFUSAL}`]: () => {
      restored();
      return new Response(null, { status: 204 });
    },
  });

  await userEvent.click(
    await screen.findByRole("button", {
      name: `${en["unroutedMail.notMine"]}: ${INFO}`,
    }),
  );
  await waitFor(() => expect(refused).toHaveBeenCalledOnce());
  await userEvent.click(
    await screen.findByRole("button", { name: en["common.undo"] }),
  );
  await waitFor(() => expect(restored).toHaveBeenCalledOnce());
});

it("tells the reader to confirm an address that is already on file", async () => {
  showing({
    "POST /capture/owner-identities": () =>
      jsonResponse(
        {
          code: "domain_needs_confirmation",
          title: "Needs confirmation",
          status: 409,
        },
        409,
      ),
  });

  await userEvent.click(
    await screen.findByRole("button", {
      name: `${en["unroutedMail.receive"]}: ${INFO}`,
    }),
  );
  expect(
    await screen.findByText(en["unroutedMail.needsConfirmation"]),
  ).toBeTruthy();
});
