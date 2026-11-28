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
import { en as englishCopy } from "../i18n/en";
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
      if (path === `${LINK_PATH}/channels` && request.method === "GET") {
        return jsonResponse({
          channels: [
            { slug: "deal-acme-renewal", name: "deal-acme-renewal" },
            { slug: "deal-globex", name: "deal-globex" },
          ],
        });
      }
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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = rtlRender(
    <QueryClientProvider client={queryClient}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
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
  it("hides the card when no chat bot is connected", async () => {
    stubServer({ state: "off" });
    const { container, queryClient } = renderPanel();
    await waitFor(() => {
      expect(
        queryClient.getQueryState(["channel-link", "deal", DEAL_ID])?.status,
      ).toBe("success");
    });
    expect(container).toBeEmptyDOMElement();
  });

  it("hides the card while checking whether a chat bot is connected", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const { container } = renderPanel();
    expect(container).toBeEmptyDOMElement();
  });

  it("creates a channel and names the invited members without a chat account", async () => {
    const writes = stubServer({ state: "none" }, () =>
      jsonResponse(
        {
          state: "linked",
          link,
          invites: {
            invited: 3,
            not_in_chat: 2,
            not_added: 1,
            skipped_over_cap: 0,
          },
        },
        201,
      ),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["channelLink.create"],
      }),
    );
    expect(
      await screen.findByText(
        "2 invited members have no chat account. 1 invited member was not added. Check the app’s audience in Dispact.",
      ),
    ).toBeInTheDocument();
    expect(writes).toEqual([
      { method: "POST", body: { mode: "create", provider: "dispact" } },
    ]);
  });

  it("links an existing channel picked from the ones the bot is in, with the history summary asked for by default", async () => {
    const writes = stubServer({ state: "none" }, () =>
      jsonResponse({ state: "linked", link }, 201),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["channelLink.linkExisting"],
      }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("checkbox", {
        name: englishCopy["channelLink.summarize"],
      }),
    ).toBeChecked();
    await user.type(
      within(dialog).getByLabelText("Channel name or link *"),
      "acme",
    );
    await user.click(
      await screen.findByRole("option", { name: /deal-acme-renewal/ }),
    );
    await pickOption(
      user,
      within(dialog).getByRole("combobox", {
        name: englishCopy["channelLink.verbosity"],
      }),
      "Milestones only",
    );
    await user.click(
      within(dialog).getByRole("button", {
        name: englishCopy["channelLink.linkCta"],
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(writes).toEqual([
      {
        method: "POST",
        body: {
          provider: "dispact",
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
      await screen.findByRole("button", {
        name: englishCopy["channelLink.linkExisting"],
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByLabelText("Channel name or link *"),
      "deal-acme-renewal",
    );
    await user.click(
      within(dialog).getByRole("button", {
        name: englishCopy["channelLink.linkCta"],
      }),
    );
    expect(
      await within(dialog).findByText(
        englishCopy["channelLink.refused.botNotMember"],
      ),
    ).toBeInTheDocument();
  });

  it("tells the rep to rename the record when the chat refuses the channel name", async () => {
    stubServer({ state: "none" }, () =>
      jsonResponse(
        { code: "channel_name_refused", detail: "name refused" },
        422,
      ),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["channelLink.create"],
      }),
    );
    expect(
      await screen.findByText(englishCopy["channelLink.refused.nameRefused"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(/try again/)).not.toBeInTheDocument();
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
    await user.click(await screen.findByRole("button", { name: "Manage" }));
    const verbosity = await screen.findByRole("combobox", {
      name: englishCopy["channelLink.verbosity"],
    });
    expect(verbosity).toHaveTextContent(
      englishCopy["channelLink.verbosity.important"],
    );
    await pickOption(user, verbosity, "Silent (card only)");
    await waitFor(() =>
      expect(writes).toEqual([
        { method: "PATCH", body: { verbosity: "silent" } },
      ]),
    );
    await waitFor(() =>
      expect(verbosity).toHaveTextContent(
        englishCopy["channelLink.verbosity.silent"],
      ),
    );
  });

  it("changes message detail without rewriting frequency or either sync direction", async () => {
    const writes = stubServer(
      { state: "linked", link: { ...link, message_detail: "brief" } },
      (write) =>
        jsonResponse({
          state: "linked",
          link: { ...link, message_detail: "brief", ...(write.body as object) },
        }),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(await screen.findByRole("button", { name: "Manage" }));
    const detail = screen.getByRole("combobox", {
      name: englishCopy["channelLink.messageDetail"],
    });
    await pickOption(user, detail, "Full");
    await waitFor(() =>
      expect(writes).toEqual([
        {
          method: "PATCH",
          body: { message_detail: "full", expected_link_id: link.id },
        },
      ]),
    );
    await waitFor(() => expect(detail).toHaveTextContent("Full"));
    expect(
      screen.getByRole("combobox", {
        name: englishCopy["channelLink.verbosity"],
      }),
    ).toHaveTextContent(englishCopy["channelLink.verbosity.important"]);
    expect(
      screen.getByRole("switch", {
        name: englishCopy["channelLink.readDirection"],
      }),
    ).toBeChecked();
    expect(
      screen.getByRole("switch", {
        name: englishCopy["channelLink.postDirection"],
      }),
    ).toBeChecked();
  });

  it("unlinks only after the reader confirms", async () => {
    const writes = stubServer({ state: "linked", link });
    const user = userEvent.setup();
    renderPanel();
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["record.moreActions"],
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Unlink Acme renewal" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(englishCopy["channelLink.unlinkBody"]),
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
    expect(
      await screen.findByText(englishCopy["channelLink.paused"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Unlink Acme renewal" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("combobox", {
        name: englishCopy["channelLink.verbosity"],
      }),
    ).toBeNull();
  });

  it("switches each sync direction without changing the other", async () => {
    const writes = stubServer({ state: "linked", link }, (write) =>
      jsonResponse({
        state: "linked",
        link: { ...link, ...(write.body as object) },
      }),
    );
    const user = userEvent.setup();
    renderPanel();
    await user.click(await screen.findByRole("button", { name: "Manage" }));
    await user.click(
      screen.getByRole("switch", {
        name: englishCopy["channelLink.readDirection"],
      }),
    );
    await waitFor(() =>
      expect(writes).toEqual([
        {
          method: "PATCH",
          body: { save_channel_updates: false, expected_link_id: link.id },
        },
      ]),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("switch", {
          name: englishCopy["channelLink.readDirection"],
        }),
      ).not.toBeChecked(),
    );
    expect(
      screen.getByRole("switch", {
        name: englishCopy["channelLink.postDirection"],
      }),
    ).toBeChecked();
  });

  it("offers no verb to a reader who may not edit the record", async () => {
    stubServer({ state: "linked", link });
    renderPanel(false);
    expect(
      await screen.findByText(/Recaps saved to History/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: englishCopy["record.moreActions"] }),
    ).toBeNull();
  });

  it("offers no link verbs on an unlinked record to a reader who may not edit", async () => {
    stubServer({ state: "none" });
    renderPanel(false);
    expect(
      await screen.findByText(englishCopy["channelLink.none"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: englishCopy["channelLink.create"] }),
    ).toBeNull();
  });
});
