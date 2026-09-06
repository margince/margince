/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import type { Transport } from "./persontransports";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// WHICH WAY a message leaves, asked inside the one composer.
//
// The contact page used to answer this with a whole second drawer: a contact
// reachable only by mail got the shared composer and one with a chat channel got
// an older one, so the same button gave two surfaces and half the traffic lost
// the thread pane, the conversation offers and the permission preview. The dial
// lives in the shared drawer now.
//
// The rules about WHICH transports exist are `persontransports.test.ts` — they
// are statements about reachability and anchors, and asserting them through a
// rendered composer meant mounting a drawer to find out whether a list had two
// entries. What is here is what the DRAWER does with the answer.

type Sent = { key: string; body: unknown };

const MAIL: Transport = { id: "email", label: "Email" };
const CHAT: Transport = { id: "dispact", label: "Dispact", anchorId: "a-chat" };

const ACTIVITY = {
  id: "a-chat",
  kind: "message",
  channel_provider: "dispact",
  // Inbound: the contact wrote first, which is what makes a reply a reply and
  // spares the reader the "why are you writing?" the composer asks of a message
  // with no anchor to derive from.
  direction: "inbound",
  occurred_at: "2026-08-15T08:00:00Z",
  is_done: false,
  source: "ext:dispact-connector:dispact",
  captured_by: "human:u1",
  created_at: "2026-08-15T08:00:00Z",
  updated_at: "2026-08-15T08:00:00Z",
  version: 1,
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, body });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /activities/a-chat") return jsonResponse(ACTIVITY);
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({ data: [] });
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function drawer(transports: readonly Transport[], initial?: string) {
  return (
    <ComposeModal
      entityType="person"
      entityId="p-1"
      personId="p-1"
      recordAddress="dana@brandt.example"
      transports={transports}
      initialTransportId={initial}
      open
      onClose={vi.fn()}
    />
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the composer's transport dial", () => {
  // One transport is not a decision. A dial holding a single option asks a
  // reader to confirm something they were never offered an alternative to.
  it("asks nothing when there is only one way to reach them", async () => {
    render(drawer([MAIL]));
    expect(await screen.findByLabelText("Subject")).toBeTruthy();
    expect(screen.queryByLabelText("How to send")).toBeNull();
  });

  it("offers the choice when there is one to make", async () => {
    render(drawer([MAIL, CHAT]));
    expect(await screen.findByLabelText("How to send")).toBeTruthy();
    // Mail leads, because it is the transport that can open a conversation.
    expect(screen.getByLabelText("Subject")).toBeTruthy();
  });

  // A channel carries no subject line and names no addressee — the server
  // resolves the recipient from the conversation being answered — so a composer
  // still drawing those fields would offer what the send cannot carry.
  it("withholds the mail-only fields once a channel is chosen", async () => {
    const user = userEvent.setup();
    render(drawer([MAIL, CHAT]));

    await pickOption(
      user,
      await screen.findByLabelText("How to send"),
      "Dispact",
    );

    await waitFor(() => expect(screen.queryByLabelText("Subject")).toBeNull());
    expect(screen.queryByLabelText("To")).toBeNull();
    expect(screen.queryByLabelText("Cc")).toBeNull();
  });

  // Opening ON the channel is the worklist's case: the row is about one message,
  // and the transport that message is on is the one to answer it from.
  it("opens on the transport a caller named", async () => {
    render(drawer([MAIL, CHAT], "dispact"));
    await screen.findByLabelText("How to send");
    expect(screen.queryByLabelText("Subject")).toBeNull();
  });

  // The words do not travel between transports: a subject typed for mail has
  // nowhere to go on a channel, and a body written for one conversation must not
  // arrive in another because the rep changed their mind about the medium.
  it("clears what was written when the dial moves", async () => {
    const user = userEvent.setup();
    render(drawer([MAIL, CHAT]));

    await user.type(await screen.findByLabelText("Subject"), "About the offer");
    writeMessage("Body", "Here it is.");

    await pickOption(user, screen.getByLabelText("How to send"), "Dispact");
    await pickOption(user, screen.getByLabelText("How to send"), "Email");

    await waitFor(() =>
      expect((screen.getByLabelText("Subject") as HTMLInputElement).value).toBe(
        "",
      ),
    );
  });

  // The whole point of the dial: a chosen channel answers ITS conversation, so
  // the send is the anchored channel reply and not an account-started email.
  it("sends a chosen channel through the conversation it continues", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "POST /activities/a-chat/send-message": () => jsonResponse(ACTIVITY, 202),
    });
    render(drawer([MAIL, CHAT], "dispact"));

    await screen.findByLabelText("How to send");
    writeMessage("Body", "On my way.");
    await user.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/a-chat/send-message"),
      ).toBe(true),
    );
    // And never the mail door, which would send a channel message as an
    // account email — wrong transport, wrong body shape.
    expect(sent.some((r) => r.key === "POST /emails")).toBe(false);
  });

  // The named conversation cannot be answered any more — disconnected, removed,
  // or off the end of the record's own window. Falling back silently is the
  // reader writing into a conversation they did not choose.
  it("says so when the conversation a caller named is gone", async () => {
    render(
      <ComposeModal
        entityType="person"
        entityId="p-1"
        personId="p-1"
        transports={[MAIL]}
        staleThread
        open
        onClose={vi.fn()}
      />,
    );
    expect(await screen.findByText(/can no longer be answered/i)).toBeTruthy();
  });
});
