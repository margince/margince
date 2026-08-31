/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { GoogleAppCard } from "./google-app";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CLIENT_ID = "111-abc.apps.googleusercontent.com";

type GoogleAppResponse = {
  configured: boolean;
  client_id: string;
  source: "stored" | "environment" | "none";
  redirect_uris: { purpose: string; url: string }[];
};

// The three sources the card has to tell apart. `configured` alone cannot:
// it is true for both a stored app and one the deployment supplies, and the
// card said "no app stored" for the second — telling an operator Gmail could
// not be connected on an installation where it demonstrably could.
const SIGN_IN_URI = "https://api.acme.test/v1/auth/oidc/google/callback";
const CONNECT_URI = "https://api.acme.test/v1/connectors/google/callback";

function stored(): GoogleAppResponse {
  return {
    configured: true,
    client_id: CLIENT_ID,
    source: "stored",
    redirect_uris: [
      { purpose: "sign_in", url: SIGN_IN_URI },
      { purpose: "mailbox_connect", url: CONNECT_URI },
    ],
  };
}

function fromEnvironment(): GoogleAppResponse {
  return {
    configured: true,
    client_id: CLIENT_ID,
    source: "environment",
    redirect_uris: [{ purpose: "mailbox_connect", url: CONNECT_URI }],
  };
}

function absent(): GoogleAppResponse {
  return {
    configured: false,
    client_id: "",
    source: "none",
    redirect_uris: [],
  };
}

function mount(app: GoogleAppResponse) {
  const calls: { method: string; url: string; body?: unknown }[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const url = new URL(request.url).pathname;
    if (request.method === "GET") {
      if (url.endsWith("/me")) {
        // Through the fixture rather than a hand-built body: the grant shape is
        // the capability layer's, and a test that spelled its own would prove
        // nothing about what the real /me returns.
        return jsonResponse(
          meFixture({ allow: { capture_settings: ["read", "update"] } }),
        );
      }
      return jsonResponse(app);
    }
    calls.push({
      method: request.method,
      url,
      body: request.body ? JSON.parse(await request.text()) : undefined,
    });
    return new Response(null, { status: 204 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(<GoogleAppCard />, { wrapper: Wrap });
  return { calls };
}

describe("the Google app card", () => {
  // The client id is NOT a secret — it travels in every authorization redirect,
  // and an operator has to see which app their installation uses to check it
  // against the Google console.
  it("names the app in use", async () => {
    mount(stored());
    expect(await screen.findByText(`In use: ${CLIENT_ID}`)).toBeTruthy();
  });

  // An installation with none has not failed at anything, but it cannot connect
  // a mailbox either, and the card says which.
  it("says no app is available from any source, and what that costs", async () => {
    mount(absent());
    expect(
      await screen.findByText(/No app is available from any source/),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove app" })).toBeNull();
  });

  // The bug this state exists to fix. With credentials in the environment the
  // card used to read "No app stored. Gmail and Calendar cannot be connected",
  // whose second sentence was simply false: the connector falls back to the
  // deployment's app, and GET /installation/setup reported the same
  // installation configured.
  it("reports an app the deployment supplies, rather than claiming there is none", async () => {
    mount(fromEnvironment());
    expect(
      await screen.findByText(
        new RegExp(`In use from this deployment.s configuration: ${CLIENT_ID}`),
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/No app is available/)).toBeNull();
    // Nothing is STORED, so there is nothing to remove.
    expect(screen.queryByRole("button", { name: "Remove app" })).toBeNull();
  });

  // An operator cannot guess these, and registering only one of them fails at
  // the consent screen with an error that does not say which was missing.
  it("lists every redirect URI this deployment serves, and only those", async () => {
    mount(stored());
    expect(await screen.findByText(SIGN_IN_URI)).toBeTruthy();
    expect(screen.getByText(CONNECT_URI)).toBeTruthy();
  });

  it("does not advertise a redirect URI for a flow this deployment does not serve", async () => {
    mount(fromEnvironment());
    expect(await screen.findByText(CONNECT_URI)).toBeTruthy();
    expect(screen.queryByText(SIGN_IN_URI)).toBeNull();
  });

  it("sends the pair together and then holds neither", async () => {
    const user = userEvent.setup();
    const { calls } = mount(absent());
    await screen.findByText(/No app is available/);
    const secret = screen.getByLabelText<HTMLInputElement>("Client secret");
    await user.type(screen.getByLabelText("Client ID"), CLIENT_ID);
    await user.type(secret, "GOCSPX-secret");
    await user.click(screen.getByRole("button", { name: "Store app" }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toMatchObject({
      method: "PUT",
      url: "/v1/installation/google-app",
      body: { client_id: CLIENT_ID, client_secret: "GOCSPX-secret" },
    });
    // Cleared on the way out: the field was the only copy this app held.
    await waitFor(() => expect(secret.value).toBe(""));
  });

  it("offers removal only for an app that is there", async () => {
    const user = userEvent.setup();
    const { calls } = mount(stored());
    await user.click(await screen.findByRole("button", { name: "Remove app" }));
    // The first press opens the question; it does not answer it.
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Remove app" }),
    );
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toMatchObject({
      method: "DELETE",
      url: "/v1/installation/google-app",
    });
  });

  // The secret cannot be read back, every mailbox is connected through this
  // app, and removing it re-opens the first-run gate — so a stray click must
  // not be enough.
  it("deletes nothing until the removal is confirmed", async () => {
    const user = userEvent.setup();
    const { calls } = mount(stored());
    await user.click(await screen.findByRole("button", { name: "Remove app" }));
    await screen.findByRole("dialog");
    expect(calls).toEqual([]);
  });
});
