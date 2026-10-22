/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

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
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { useMe } from "../../lib/session";
import { meFixture } from "../../testing/mefixture";
import { TelegramConnectorsPanel } from "./connectors.telegram";

type ChannelConnection = components["schemas"]["ChannelConnection"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type StubOpts = {
  channels?: ChannelConnection[];
  /** Answer with the 503 of a deployment that serves no messaging channels. */
  channelsNotConfigured?: boolean;
  /** The session GET /me answers; unset, an admin holding no object grants. */
  me?: components["schemas"]["MeResponse"];
};

function stubApi(opts: StubOpts) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/channel-connections") && request.method === "GET") {
        if (opts.channelsNotConfigured) {
          return jsonResponse(
            { code: "channel_connections_not_configured" },
            503,
          );
        }
        return jsonResponse({ data: opts.channels ?? [] });
      }
      if (path.endsWith("/me") && request.method === "GET") {
        return jsonResponse(opts.me ?? meFixture({}));
      }
      throw new Error(`unstubbed: ${request.method} ${path}`);
    }),
  );
}

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

// Shares the panel's cache entry, so it appears only once the grants it reads
// have arrived.
function SessionSettled() {
  return useMe().isSuccess ? <span>session settled</span> : null;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A workspace may hold more than one live bot, since nothing in the API forbids
// connecting a second one. A send refuses outright while it does. This panel is
// the only surface that can take one away again, so it has to show every one it
// is handed.
describe("the Telegram connector panel", () => {
  const salesBot: ChannelConnection = {
    id: "018f3a1b-0000-7000-8000-0000000000d1",
    provider: "telegram",
    channelId: "555000111",
    channelLabel: "acme_sales_bot",
    status: "connected",
    version: 1,
  };
  const supportBot: ChannelConnection = {
    ...salesBot,
    id: "018f3a1b-0000-7000-8000-0000000000d2",
    channelId: "555000222",
    channelLabel: "acme_support_bot",
    status: "pending",
  };
  const botManager = meFixture({
    allow: { channel_connection: ["read", "create", "update", "delete"] },
  });

  it("lists every connected bot, each with its own Disconnect", async () => {
    stubApi({ channels: [salesBot, supportBot], me: botManager });
    render(<TelegramConnectorsPanel />);

    // One SettingRow per bot. The roster is a list of decisions now. A bot is
    // identified by its own row rather than by an <li> from the list wrapper.
    const rows = await screen.findAllByTestId("telegram-connection");
    expect(rows.length).toBe(2);
    expect(within(rows[0]).getByText("@acme_sales_bot")).toBeTruthy();
    expect(within(rows[1]).getByText("@acme_support_bot")).toBeTruthy();
    // One shared Disconnect for two bots could only ever remove one of them,
    // which is the state an admin is here to escape.
    for (const row of rows) {
      expect(
        within(row).getByRole("button", { name: "Disconnect" }),
      ).toBeTruthy();
    }
  });

  it("opens the replace-token form on the bot whose row was clicked", async () => {
    stubApi({ channels: [salesBot, supportBot], me: botManager });
    render(<TelegramConnectorsPanel />);

    const rows = await screen.findAllByTestId("telegram-connection");
    await userEvent.click(
      within(rows[1]).getByRole("button", { name: "Replace token" }),
    );
    // The second bot is the pending one. The form showing its status proves it
    // bound to that row and not to the first.
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/^Pending/)).toBeTruthy();
  });

  const readOnlyNote =
    "Only an administrator or operations user can connect or change the bot.";

  it("offers the connect verb to a reader who holds channel_connection:create", async () => {
    stubApi({
      me: meFixture({ allow: { channel_connection: ["read", "create"] } }),
    });
    render(<TelegramConnectorsPanel />);
    expect(await screen.findByTestId("telegram-connect")).toBeTruthy();
    expect(screen.queryByText(readOnlyNote)).toBeNull();
  });

  // The seeded rep reads the bot and may change none of it. Three buttons the
  // server would refuse are withheld, and the line says who can act instead.
  it("gives a reader without write grants the roster and the reason, no verbs", async () => {
    stubApi({
      channels: [salesBot],
      me: meFixture({
        roles: ["rep"],
        allow: { channel_connection: ["read"] },
      }),
    });
    render(<TelegramConnectorsPanel />);
    expect(await screen.findByText(readOnlyNote)).toBeTruthy();
    const row = screen.getByTestId("telegram-connection");
    expect(within(row).getByText("@acme_sales_bot")).toBeTruthy();
    expect(within(row).queryByRole("button")).toBeNull();
    expect(row.querySelector(".provider-row-actions")).toBeNull();
  });

  it("draws nothing for a reader who has no bot to see and cannot connect one", async () => {
    stubApi({
      me: meFixture({
        roles: ["rep"],
        allow: { channel_connection: ["read"] },
      }),
    });
    render(
      <>
        <TelegramConnectorsPanel />
        <SessionSettled />
      </>,
    );
    // Absence proves nothing while /me is in flight, when every grant is false.
    await screen.findByText("session settled");
    await waitFor(() => expect(screen.queryByText("Telegram bot")).toBeNull());
    expect(screen.queryByTestId("telegram-connect")).toBeNull();
  });

  it("withholds every verb from a read seat even when the role grants them", async () => {
    stubApi({
      channels: [salesBot],
      me: meFixture({
        seat: "read",
        allow: { channel_connection: ["read", "create", "update", "delete"] },
      }),
    });
    render(<TelegramConnectorsPanel />);
    expect(await screen.findByText(readOnlyNote)).toBeTruthy();
    expect(
      within(screen.getByTestId("telegram-connection")).queryByRole("button"),
    ).toBeNull();
  });

  it("gates each row verb on its own grant", async () => {
    stubApi({
      channels: [salesBot],
      me: meFixture({ allow: { channel_connection: ["read", "update"] } }),
    });
    render(<TelegramConnectorsPanel />);
    const row = await screen.findByTestId("telegram-connection");
    expect(
      await within(row).findByRole("button", { name: "Replace token" }),
    ).toBeTruthy();
    expect(
      within(row).queryByRole("button", { name: "Disconnect" }),
    ).toBeNull();
    expect(screen.queryByText(readOnlyNote)).toBeNull();
  });

  // No channels on this deployment withholds the verbs from an admin too. The
  // seat is then not the reason, and the read-only line would misdirect a rep.
  it("lets the not-configured notice speak alone when neither applies", async () => {
    stubApi({
      channelsNotConfigured: true,
      me: meFixture({
        roles: ["rep"],
        allow: { channel_connection: ["read"] },
      }),
    });
    render(
      <>
        <TelegramConnectorsPanel />
        <SessionSettled />
      </>,
    );
    expect(
      await screen.findByText(
        "Messaging channels are not configured on this installation.",
      ),
    ).toBeTruthy();
    // Absence proves nothing while /me is in flight, when every grant is false.
    await screen.findByText("session settled");
    expect(screen.queryByText(readOnlyNote)).toBeNull();
  });
});
