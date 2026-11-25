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
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { TelegramBusinessPanel } from "./connectors.telegrambusiness";

type ChannelConnection = components["schemas"]["ChannelConnection"];
type BusinessConnection = components["schemas"]["TelegramBusinessConnection"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const bot: ChannelConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000d1",
  provider: "telegram",
  channelId: "555000111",
  channelLabel: "acme_sales_bot",
  status: "connected",
  version: 1,
};

const team = { id: "018f3a1b-0000-7000-8000-0000000000e1", name: "Sales desk" };
const otherTeam = {
  id: "018f3a1b-0000-7000-8000-0000000000e2",
  name: "Support",
};

const pending: BusinessConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000b1",
  accountId: "9001",
  ownerLabel: "Mira Keller",
  status: "pending_approval",
  connectedAt: "2026-03-02T10:00:00Z",
};
const live: BusinessConnection = {
  ...pending,
  id: "018f3a1b-0000-7000-8000-0000000000b2",
  accountId: "9002",
  ownerLabel: "Jonas Weiss",
  status: "connected",
  approvedAt: "2026-03-03T10:00:00Z",
  seatId: "018f3a1b-0000-7000-8000-0000000000f1",
  readerTeamId: team.id,
  mailVisibility: "held",
};
const off: BusinessConnection = {
  ...pending,
  id: "018f3a1b-0000-7000-8000-0000000000b3",
  accountId: "9003",
  ownerLabel: "Lena Brandt",
  status: "disconnected",
};

const admin = meFixture({
  allow: { channel_connection: ["read", "create", "update", "delete"] },
});

type StubOpts = {
  bots?: ChannelConnection[];
  accounts?: BusinessConnection[];
  me?: components["schemas"]["MeResponse"];
  approveStatus?: number;
  visibilityStatus?: number;
  visibilityDetail?: string;
};

function stubApi(opts: StubOpts) {
  const calls: string[] = [];
  const bodies: Record<string, unknown> = {};
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const key = `${request.method} ${pathname.replace(/^.*\/channel-connections/, "")}`;
      if (key === "GET ") {
        return jsonResponse({ data: opts.bots ?? [bot] });
      }
      if (pathname.endsWith("/me")) {
        return jsonResponse(opts.me ?? admin);
      }
      if (pathname.endsWith("/teams")) {
        return jsonResponse({ data: [team, otherTeam], page: {} });
      }
      if (request.method !== "GET" && request.method !== "DELETE") {
        bodies[key] = await request.clone().json();
      }
      calls.push(key);
      if (key === "GET /telegram-business") {
        return jsonResponse({ data: opts.accounts ?? [] });
      }
      if (key.endsWith("/approve")) {
        if (opts.approveStatus === 409) {
          return jsonResponse(
            { status: 409, code: "conflict", title: "Conflict" },
            409,
          );
        }
        if (opts.approveStatus === 422) {
          return jsonResponse(
            {
              status: 422,
              code: "reader_team_required",
              title: "Unprocessable Entity",
            },
            422,
          );
        }
        return jsonResponse({ ...pending, status: "connected" });
      }
      if (key.endsWith("/visibility")) {
        if (opts.visibilityStatus === 422) {
          return jsonResponse(
            {
              status: 422,
              code: "shared_posture_not_allowed",
              title: "Unprocessable Entity",
              detail: opts.visibilityDetail,
            },
            422,
          );
        }
        return jsonResponse(live);
      }
      if (key.startsWith("DELETE")) {
        return new Response(null, { status: 204 });
      }
      throw new Error(`unstubbed: ${key}`);
    }),
  );
  return Object.assign(calls, { bodies });
}

function render() {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <TelegramBusinessPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the Telegram Business panel", () => {
  it("names the connected bot in the setup steps and shows the empty state", async () => {
    stubApi({});
    render();
    expect(
      await screen.findByText(/Enters @acme_sales_bot and chooses/),
    ).toBeVisible();
    expect(await screen.findByText("No business account linked")).toBeVisible();
  });

  it("shows one row per status with its own badge and verbs", async () => {
    stubApi({ accounts: [pending, live, off] });
    render();
    const rows = await screen.findAllByTestId("telegram-business-connection");
    expect(rows).toHaveLength(3);

    expect(within(rows[0]).getByText("Mira Keller")).toBeVisible();
    expect(within(rows[0]).getByText("Waiting for approval")).toBeVisible();
    expect(
      within(rows[0]).getByRole("button", {
        name: "Approve as company account",
      }),
    ).toBeVisible();

    expect(within(rows[1]).getByText("Connected")).toBeVisible();
    expect(
      within(rows[1]).queryByRole("button", {
        name: "Approve as company account",
      }),
    ).toBeNull();
    expect(
      within(rows[1]).getByRole("button", { name: "Change who reads" }),
    ).toBeVisible();
    expect(
      within(rows[1]).getByRole("button", { name: "Actions for Jonas Weiss" }),
    ).toBeVisible();

    expect(within(rows[2]).getByText("Switched off in Telegram")).toBeVisible();
    expect(within(rows[2]).getByText(/switched this link off/)).toBeVisible();
    expect(
      within(rows[2]).queryByRole("button", {
        name: "Approve as company account",
      }),
    ).toBeNull();
  });

  async function openApprove(user: ReturnType<typeof userEvent.setup>) {
    await user.click(
      await screen.findByRole("button", { name: "Approve as company account" }),
    );
    return screen.findByRole("dialog");
  }

  it("needs a reader team, then posts the approval with it and reloads the list", async () => {
    const calls = stubApi({ accounts: [pending] });
    const user = userEvent.setup();
    render();
    const dialog = await openApprove(user);
    expect(within(dialog).getByText(/only the team you choose/)).toBeVisible();
    const confirm = within(dialog).getByRole("button", {
      name: "Approve as company account",
    });
    await user.click(confirm);
    await waitFor(() =>
      expect(within(dialog).getByRole("combobox")).toHaveAttribute(
        "aria-invalid",
        "true",
      ),
    );
    expect(calls.some((c) => c.endsWith("/approve"))).toBe(false);

    await user.click(within(dialog).getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Sales desk" }));
    await user.click(confirm);
    const key = `POST /telegram-business/${pending.id}/approve`;
    await waitFor(() => expect(calls).toContain(key));
    expect(calls.bodies[key]).toEqual({ readerTeamId: team.id });
    await waitFor(() =>
      expect(calls.filter((c) => c === "GET /telegram-business")).toHaveLength(
        2,
      ),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  async function approveWithTeam(user: ReturnType<typeof userEvent.setup>) {
    const dialog = await openApprove(user);
    await user.click(within(dialog).getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Sales desk" }));
    await user.click(
      within(dialog).getByRole("button", {
        name: "Approve as company account",
      }),
    );
    return dialog;
  }

  it("names both causes when approving answers 409", async () => {
    const calls = stubApi({ accounts: [pending], approveStatus: 409 });
    const user = userEvent.setup();
    render();
    const dialog = await approveWithTeam(user);
    const message = await within(dialog).findByText(/cannot be approved/);
    expect(message).toHaveTextContent(/switched the link off/);
    expect(message).toHaveTextContent(
      /differs from the one that already reads/,
    );
    await waitFor(() =>
      expect(calls.filter((c) => c === "GET /telegram-business")).toHaveLength(
        2,
      ),
    );
  });

  it("asks for a team when approving answers 422 reader_team_required", async () => {
    stubApi({
      accounts: [pending],
      approveStatus: 422,
    });
    const user = userEvent.setup();
    render();
    const dialog = await approveWithTeam(user);
    expect(
      await within(dialog).findByText(
        "Choose the team that reads this account’s messages.",
      ),
    ).toBeVisible();
  });

  it("shows each account's reader team and visibility", async () => {
    stubApi({
      accounts: [
        live,
        { ...live, id: "x2", ownerLabel: "Ada", mailVisibility: "shared" },
        {
          ...live,
          id: "x3",
          ownerLabel: "Own",
          readerTeamId: null,
          mailVisibility: "classified",
        },
      ],
    });
    render();
    const rows = await screen.findAllByTestId("telegram-business-connection");
    expect(
      await within(rows[0]).findByText(/Read by Sales desk · Held/),
    ).toBeVisible();
    expect(
      within(rows[1]).getByText(/Read by Sales desk · Shared/),
    ).toBeVisible();
    expect(within(rows[2]).getByText(/each chat is held/)).toBeVisible();
    expect(
      within(rows[2]).queryByRole("button", { name: "Change who reads" }),
    ).toBeNull();
  });

  async function openVisibility(user: ReturnType<typeof userEvent.setup>) {
    await user.click(
      await screen.findByRole("button", { name: "Change who reads" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: /Shared/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "Change visibility" }),
    );
    return dialog;
  }

  it("sends the visibility change with confirm and reloads the list", async () => {
    const calls = stubApi({ accounts: [live] });
    const user = userEvent.setup();
    render();
    await openVisibility(user);
    const key = `PATCH /telegram-business/${live.id}/visibility`;
    await waitFor(() => expect(calls).toContain(key));
    expect(calls.bodies[key]).toEqual({
      mail_visibility: "shared",
      confirm: true,
    });
    await waitFor(() =>
      expect(calls.filter((c) => c === "GET /telegram-business")).toHaveLength(
        2,
      ),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("shows the refusal when the company has not opted in to shared", async () => {
    stubApi({
      accounts: [live],
      visibilityStatus: 422,
      visibilityDetail:
        "Shared mail visibility is not enabled for this company.",
    });
    const user = userEvent.setup();
    render();
    const dialog = await openVisibility(user);
    expect(
      await within(dialog).findByText(/not enabled for this company/),
    ).toBeVisible();
  });

  it("confirms before disconnecting and sends the delete", async () => {
    const calls = stubApi({ accounts: [live] });
    const user = userEvent.setup();
    render();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Jonas Weiss" }),
    );
    await user.click(await screen.findByRole("button", { name: "Disconnect" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(/stops capturing from this account/),
    ).toBeVisible();
    expect(calls.some((c) => c.startsWith("DELETE"))).toBe(false);

    await user.click(
      within(dialog).getByRole("button", { name: "Disconnect" }),
    );
    await waitFor(() =>
      expect(calls).toContain(`DELETE /telegram-business/${live.id}`),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("withdraws when no Telegram bot is connected", async () => {
    const calls = stubApi({ bots: [] });
    const { container } = render();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(calls).not.toContain("GET /telegram-business");
  });

  it("withdraws from a reader who may delete but not update", async () => {
    const calls = stubApi({
      me: meFixture({
        roles: ["rep"],
        allow: { channel_connection: ["read", "delete"] },
      }),
    });
    const { container } = render();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(calls).not.toContain("GET /telegram-business");
  });

  it("withdraws from a reader who holds neither verb", async () => {
    const calls = stubApi({
      me: meFixture({
        roles: ["rep"],
        allow: { channel_connection: ["read"] },
      }),
    });
    const { container } = render();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(calls).not.toContain("GET /telegram-business");
  });
});
