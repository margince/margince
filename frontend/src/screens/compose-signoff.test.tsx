/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// The sign-off under the body is the server's answer, drawn as given: the
// composer shows what the send appends and never assembles it itself.

type SignOff = components["schemas"]["EmailSignOff"];

const REGION_NAME = "Added when you send";
const CLOSING_HINT = "You have no signature, so this closing is added.";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function meAs(id: string) {
  const me = meFixture({});
  return { ...me, user: { ...me.user, id } };
}

const COMPANY_VIEW = {
  company: { id: "company-1", name: "Acme" },
  contacts: { data: [{ contact_id: "per-1", full_name: "Dieter Klein" }] },
  deals: { data: [] },
  projects: [],
};

// The server's answer is a function of who asks, so a test can change the
// signed-in member and see whose sign-off the composer draws.
function stubRoutes(signOffFor: (userId: string) => SignOff) {
  const asked: string[] = [];
  const session = { userId: "user-a" };
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
      asked.push(key);
      if (key === "GET /me") return jsonResponse(meAs(session.userId));
      if (key === "POST /emails:sign-off")
        return jsonResponse(signOffFor(session.userId));
      if (key === "GET /consent-purposes") return jsonResponse({ data: [] });
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (key === "GET /companies/company-1/360")
        return jsonResponse(COMPANY_VIEW);
      if (isPreviewDoor(url.pathname)) {
        const body = request ? await request.clone().json() : null;
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({});
    }),
  );
  return { asked, session };
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

function renderComposer() {
  return render(
    <ComposeModal
      entityType="company"
      entityId="company-1"
      contactId="per-1"
      open
      onClose={vi.fn()}
    />,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the sign-off under the composer's body", () => {
  it("shows the sender's signature as the send will append it", async () => {
    const { asked } = stubRoutes(() => ({
      text: "Marek Janetzke\nGradion",
      kind: "signature",
    }));
    renderComposer();

    const block = await screen.findByRole("region", { name: REGION_NAME });
    expect(block.textContent).toContain("Marek Janetzke\nGradion");
    expect(screen.queryByText(CLOSING_HINT)).toBeNull();
    expect(asked).toContain("POST /emails:sign-off");
  });

  it("says when no signature is set and links to where one is written", async () => {
    stubRoutes(() => ({
      text: "Best regards,\nLars Jankowfsky",
      kind: "closing",
    }));
    renderComposer();

    const block = await screen.findByRole("region", { name: REGION_NAME });
    expect(block.textContent).toContain("Best regards,\nLars Jankowfsky");
    expect(screen.getByText(CLOSING_HINT)).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Set your signature" })
        .getAttribute("href"),
    ).toBe("#/settings/account");
  });

  // The answer holds one member's signature and name. A different member
  // signed in on the same tab must never be shown it, not even while their
  // own is still being asked for.
  it("never shows one member's sign-off to the next", async () => {
    // The second member's answer is held back, so the window in which only
    // the first member's answer exists is one the test can look into.
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const { session } = stubRoutes((userId) => ({
      text: userId === "user-a" ? "Ada Alpha" : "Ben Beta",
      kind: "signature",
    }));
    const answer = vi.mocked(fetch).getMockImplementation();
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      if (
        session.userId === "user-b" &&
        String(input instanceof Request ? input.url : input).includes(
          "sign-off",
        )
      ) {
        await held;
      }
      return answer ? answer(input, init) : new Response(null, { status: 500 });
    });
    const client = renderComposer();
    expect(
      (await screen.findByRole("region", { name: REGION_NAME })).textContent,
    ).toContain("Ada Alpha");

    session.userId = "user-b";
    act(() => {
      client.setQueryData(["me"], meAs("user-b"));
    });
    await waitFor(() => expect(screen.queryByText("Ada Alpha")).toBeNull());
    release();
    await screen.findByText("Ben Beta");
    expect(screen.queryByText("Ada Alpha")).toBeNull();
  });

  // A reply to a captured channel message posts to send-message, which
  // appends no sign-off — so the composer must not promise one.
  it("shows no sign-off on a channel reply", async () => {
    const { asked } = stubRoutes(() => ({
      text: "Best regards,\nLars Jankowfsky",
      kind: "closing",
    }));
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByRole("combobox", { name: "Reason for contact" });
    await waitFor(() => expect(asked).toContain("GET /me"));
    expect(screen.queryByRole("region", { name: REGION_NAME })).toBeNull();
    expect(asked).not.toContain("POST /emails:sign-off");
  });
});
