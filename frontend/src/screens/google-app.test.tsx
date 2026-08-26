/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
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

function mount(stored: { configured: boolean; client_id: string }) {
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
      return jsonResponse(stored);
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
    mount({ configured: true, client_id: CLIENT_ID });
    expect(await screen.findByText(`In use: ${CLIENT_ID}`)).toBeTruthy();
  });

  // An installation with none has not failed at anything, but it cannot connect
  // a mailbox either, and the card says which.
  it("says no app is stored, and what that costs", async () => {
    mount({ configured: false, client_id: "" });
    expect(
      await screen.findByText(/No app stored\. Gmail and Calendar cannot/),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove app" })).toBeNull();
  });

  it("sends the pair together and then holds neither", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ configured: false, client_id: "" });
    await screen.findByText(/No app stored/);
    const secret = screen.getByLabelText("Client secret") as HTMLInputElement;
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
    const { calls } = mount({ configured: true, client_id: CLIENT_ID });
    await user.click(await screen.findByRole("button", { name: "Remove app" }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toMatchObject({
      method: "DELETE",
      url: "/v1/installation/google-app",
    });
  });
});
