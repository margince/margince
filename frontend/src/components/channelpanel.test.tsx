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
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { ChannelPanel } from "./channelpanel";

type ChannelLinkState = components["schemas"]["ChannelLinkState"];

const DEAL_ID = "018f3a1b-0000-7000-8000-00000000c001";
const LINK_PATH = `/v1/channel-links/deal/${DEAL_ID}`;

const link: components["schemas"]["ChannelLink"] = {
  id: "018f3a1b-0000-7000-8000-00000000c101",
  provider: "dispact",
  channel_slug: "deal-acme-renewal",
  channel_name: "Acme renewal",
  channel_url: "https://chat.example.com/acme/channels/deal-acme-renewal",
  verbosity: "important",
  status: "active",
  deal_id: DEAL_ID,
  created_at: "2026-11-02T09:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Write = { method: string; body: unknown };

// The server as the panel meets it: GET answers `state`, and each write is
// recorded and answered by `answer`, which may move `state` on.
function stubServer(
  initial: ChannelLinkState,
  answer: (write: Write) => Response = () =>
    new Response(null, { status: 204 }),
) {
  let state = initial;
  const writes: Write[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path !== LINK_PATH) {
        throw new Error(`unstubbed: ${request.method} ${path}`);
      }
      if (request.method === "GET") return jsonResponse(state);
      const text = await request.text();
      const write = {
        method: request.method,
        body: text ? JSON.parse(text) : null,
      };
      writes.push(write);
      const response = answer(write);
      if (response.ok && response.status !== 204) {
        state = (await response.clone().json()) as ChannelLinkState;
      }
      return response;
    }),
  );
  return writes;
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

function renderPanel(canEdit = true) {
  return render(
    <ChannelPanel recordType="deal" recordId={DEAL_ID} canEdit={canEdit} />,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ChannelPanel", () => {
  it("sends a reader with no chat bot to the integrations settings", async () => {
    stubServer({ state: "off" });
    renderPanel();
    expect(
      await screen.findByText(
        "Connect a chat bot in Settings to link a channel.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open chat bot settings" }),
    ).toHaveAttribute(
      "href",
      expect.stringContaining("settings/integrations/telegram"),
    );
  });

  it("creates a channel and names the invited members without a chat account", async () => {
    const writes = stubServer({ state: "none" }, () =>
      jsonResponse(
        {
          state: "linked",
          link,
          invites: { invited: 3, not_in_chat: 2, skipped_over_cap: 0 },
        },
        201,
      ),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", { name: "Create a channel" }),
    );
    expect(
      await screen.findByText("2 invited members have no chat account."),
    ).toBeInTheDocument();
    expect(writes).toEqual([{ method: "POST", body: { mode: "create" } }]);
  });

  it("links an existing channel with the history summary asked for by default", async () => {
    const writes = stubServer({ state: "none" }, () =>
      jsonResponse({ state: "linked", link }, 201),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", { name: "Link an existing channel" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("checkbox", {
        name: "Summarize the talk so far (up to 5 summaries)",
      }),
    ).toBeChecked();
    await user.type(
      within(dialog).getByLabelText("Channel name or link *"),
      "deal-acme-renewal",
    );
    await pickOption(
      user,
      within(dialog).getByRole("combobox", { name: "What Margince posts" }),
      "Milestones only",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Link channel" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(writes).toEqual([
      {
        method: "POST",
        body: {
          mode: "existing",
          channel: "deal-acme-renewal",
          verbosity: "milestones",
          summarize_history: true,
        },
      },
    ]);
    expect(
      await screen.findByRole("link", {
        name: "Open Acme renewal in the team chat",
      }),
    ).toBeInTheDocument();
  });

  it("words a refused link in the reader's terms", async () => {
    stubServer({ state: "none" }, () =>
      jsonResponse(
        { code: "channel_bot_not_member", detail: "bot is not a member" },
        422,
      ),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", { name: "Link an existing channel" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByLabelText("Channel name or link *"),
      "deal-acme-renewal",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Link channel" }),
    );
    expect(
      await within(dialog).findByText(
        "The bot is not a member of this channel. Add it to the channel, then try again.",
      ),
    ).toBeInTheDocument();
  });

  it("changes how much Margince posts into a linked channel", async () => {
    const writes = stubServer({ state: "linked", link }, (write) =>
      jsonResponse({
        state: "linked",
        link: { ...link, ...(write.body as object) },
      }),
    );
    const user = userEvent.setup();
    renderPanel();
    const verbosity = await screen.findByRole("combobox", {
      name: "What Margince posts",
    });
    expect(verbosity).toHaveTextContent("Important only");
    await pickOption(user, verbosity, "Silent (card only)");
    await waitFor(() =>
      expect(writes).toEqual([
        { method: "PATCH", body: { verbosity: "silent" } },
      ]),
    );
    await waitFor(() =>
      expect(verbosity).toHaveTextContent("Silent (card only)"),
    );
  });

  it("unlinks only after the reader confirms", async () => {
    const writes = stubServer({ state: "linked", link });
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Unlink Acme renewal" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        "Margince stops posting and reading. The channel and its messages stay in the chat.",
      ),
    ).toBeInTheDocument();
    expect(writes).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Unlink" }));
    await waitFor(() =>
      expect(writes).toEqual([{ method: "DELETE", body: null }]),
    );
  });

  it("says a paused channel lost its chat and offers Unlink in view", async () => {
    stubServer({ state: "paused", link: { ...link, status: "channel_gone" } });
    renderPanel();
    expect(await screen.findByText("Channel paused")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Unlink Acme renewal" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("combobox", { name: "What Margince posts" }),
    ).toBeNull();
  });

  it("offers no verb to a reader who may not edit the record", async () => {
    stubServer({ state: "linked", link });
    renderPanel(false);
    const verbosity = await screen.findByRole("combobox", {
      name: "What Margince posts",
    });
    expect(verbosity).toBeDisabled();
    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
  });

  it("offers no link verbs on an unlinked record to a reader who may not edit", async () => {
    stubServer({ state: "none" });
    renderPanel(false);
    expect(await screen.findByText("No channel yet")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Create a channel" }),
    ).toBeNull();
  });
});
