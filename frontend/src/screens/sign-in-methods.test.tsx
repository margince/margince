/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { SignInMethodsCard } from "./sign-in-methods";

// Which ways people may sign in. The list is the DEPLOYMENT's — an admin
// narrows it and can never widen it — and password is not in it at all.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function mount(providers: { key: string; label: string; enabled: boolean }[]) {
  const calls: unknown[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    if (request.url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({ allow: { installation_settings: ["read", "update"] } }),
      );
    }
    if (request.method === "PATCH") {
      calls.push(JSON.parse(await request.text()));
      return new Response(null, { status: 204 });
    }
    return jsonResponse({
      name: "Acme",
      // Not a zone name: this card never reads the field, and zone literals are
      // reserved to the module that owns them so a screen cannot grow a second
      // opinion about which clock a date is in.
      timezone: "",
      base_currency: "EUR",
      base_language: "en",
      fiscal_year_start_month: 1,
      base_currency_locked: false,
      max_upload_bytes: 1,
      sign_in_providers: providers,
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(<SignInMethodsCard />, { wrapper: Wrap });
  return { calls };
}

describe("the sign-in methods card", () => {
  // Not merely "on": there is no value of the setting that removes password, so
  // the control has to exist and refuse rather than be absent — an admin who
  // cannot find the row cannot tell whether password sign-in is configured.
  it("shows password as a method that cannot be switched off", async () => {
    mount([{ key: "google", label: "Google", enabled: true }]);
    const control = await screen.findByRole("switch", {
      name: /email and password/i,
    });
    expect(control.getAttribute("aria-checked")).toBe("true");
    // Natively disabled, which is what `reason` does — aria-disabled is
    // reserved for a write in flight, and a control that is merely busy would
    // become flippable again a moment later. This one never does.
    expect(control.hasAttribute("disabled")).toBe(true);
  });

  // The whole list travels. Sending only the flipped key would silently turn
  // every other provider off, because the setting replaces rather than merges.
  it("sends the whole remaining list when a provider is switched off", async () => {
    const { calls } = mount([
      { key: "google", label: "Google", enabled: true },
      { key: "microsoft", label: "Microsoft", enabled: true },
    ]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("switch", { name: /google/i }));
    expect(calls).toEqual([{ enabled_oidc_providers: ["microsoft"] }]);
  });

  it("sends the provider added back when one is switched on", async () => {
    const { calls } = mount([
      { key: "google", label: "Google", enabled: false },
      { key: "microsoft", label: "Microsoft", enabled: true },
    ]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("switch", { name: /google/i }));
    expect(calls).toEqual([
      { enabled_oidc_providers: ["microsoft", "google"] },
    ]);
  });

  // An admin cannot add a provider here, so a deployment with none has nothing
  // to offer and the card says so rather than rendering an empty list.
  it("says so when the deployment configured no provider", async () => {
    mount([]);
    expect(
      await screen.findByText(/no external provider configured/i),
    ).toBeTruthy();
    expect(screen.queryByRole("switch", { name: /google/i })).toBeNull();
  });
});
