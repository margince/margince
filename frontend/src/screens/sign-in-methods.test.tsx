/** @vitest-environment happy-dom */
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
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { SignInMethodsCard } from "./sign-in-methods";

// Which ways contacts may sign in. The list is the DEPLOYMENT's: an admin
// narrows it and can never widen it, and password is not in it at all.

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

// The provider switches take installation_settings:update; the group→role map
// takes the admin-only authentication_policy:update. The default holds both, so
// a caller who can work the whole card is the common case; a test that wants to
// prove the map is withheld passes an allow that omits the sign-in-policy grant.
const fullSignInGrants: GrantSpec = {
  installation_settings: ["read", "update"],
  authentication_policy: ["read", "update"],
};

type Provider = { key: string; label: string; enabled: boolean };
type AppSource = "stored" | "environment" | "none";

const GOOGLE: Provider = {
  key: "google",
  label: "Continue with Google",
  enabled: true,
};
const MICROSOFT: Provider = {
  key: "microsoft",
  label: "Continue with Microsoft",
  enabled: true,
};

function mount(
  providers: Provider[],
  options: Readonly<{
    onScreen?: string[];
    apps?: AppSource;
    microsoftTenant?: string;
    allow?: GrantSpec;
  }> = {},
) {
  const allow = options.allow ?? fullSignInGrants;
  const calls: unknown[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    if (request.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method === "PATCH") {
      calls.push(JSON.parse(await request.text()));
      return new Response(null, { status: 204 });
    }
    if (request.url.includes("/users/assignable-roles")) {
      return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
    }
    // Each vendor's app, which says why a chosen provider is not drawn.
    const vendor = /\/oauth-apps\/(google|microsoft)$/.exec(request.url)?.[1];
    if (vendor) {
      const source = options.apps ?? "stored";
      return jsonResponse({
        provider: vendor,
        configured: source !== "none",
        client_id: source === "none" ? "" : "client",
        ...(vendor === "microsoft" && options.microsoftTenant
          ? { tenant: options.microsoftTenant }
          : {}),
        source,
        redirect_uris: [],
      });
    }
    // The narrow authentication-policy projection this card reads, not the
    // installation aggregate, which answers a different route.
    return jsonResponse({
      sign_in_providers: providers,
      sign_in_screen: options.onScreen ?? [],
      require_sso: false,
      require_mfa: false,
      oidc_group_role_map: {},
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
  // There is no value of the setting that removes password, so the row says
  // so in words and offers no switch to flip.
  it("shows password as always on, with no switch", async () => {
    mount([GOOGLE], { onScreen: ["google"] });
    expect(await screen.findByText("Always on")).toBeTruthy();
    expect(
      screen.queryByRole("switch", { name: /email and password/i }),
    ).toBeNull();
  });

  // The verdict is the server's sign_in_screen, never worked out here.
  it("says a provider is on the sign-in screen only when the server draws it", async () => {
    mount([GOOGLE, MICROSOFT], {
      onScreen: ["google"],
      microsoftTenant: "dir",
    });
    expect(await screen.findAllByText("On the sign-in screen")).toHaveLength(1);
    expect(
      await screen.findByText(
        "Not on the sign-in screen: the Microsoft app cannot be used right now.",
      ),
    ).toBeTruthy();
  });

  it("says a chosen provider is missing because its app is not set up", async () => {
    mount([GOOGLE], { apps: "none" });
    expect(
      await screen.findByText(
        "Not on the sign-in screen yet: the Google app is not set up.",
      ),
    ).toBeTruthy();
  });

  it("says a Microsoft app with no directory keeps its button off the screen", async () => {
    mount([MICROSOFT], { apps: "environment" });
    expect(
      await screen.findByText(/the Microsoft app is not limited to your/),
    ).toBeTruthy();
  });

  it("says a provider switched off is turned off", async () => {
    mount([{ ...GOOGLE, enabled: false }]);
    expect(await screen.findByText("Turned off")).toBeTruthy();
  });

  // Turning a provider off takes a way in from everyone who uses it, so the
  // switch only opens the question; the typed phrase answers it.
  it("sends nothing on switch-off until the phrase is typed and confirmed", async () => {
    const { calls } = mount([GOOGLE, MICROSOFT], {
      onScreen: ["google", "microsoft"],
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("switch", { name: "Continue with Google" }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Turn off Continue with Google?",
    });
    const confirm = within(dialog).getByRole("button", {
      name: "Turn off Google sign-in",
    });
    expect(confirm).toHaveProperty("disabled", true);
    expect(calls).toEqual([]);

    await user.type(
      within(dialog).getByRole("textbox"),
      "I am aware of the risk",
    );
    await user.click(confirm);
    // The whole remaining list travels: the setting replaces rather than
    // merges, so sending only the flipped key would turn every other one off.
    await waitFor(() =>
      expect(calls).toEqual([{ enabled_oidc_providers: ["microsoft"] }]),
    );
  });

  it("keeps the provider on, and sends nothing, when the admin backs out", async () => {
    const { calls } = mount([GOOGLE], { onScreen: ["google"] });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("switch", { name: "Continue with Google" }),
    );
    await user.click(await screen.findByRole("button", { name: "Keep it on" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls).toEqual([]);
  });

  // Turning one back on gives a way in rather than taking one, so it asks
  // nothing and saves at once.
  it("saves at once, with the provider added back, when one is switched on", async () => {
    const { calls } = mount([{ ...GOOGLE, enabled: false }, MICROSOFT]);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("switch", { name: "Continue with Google" }),
    );
    await waitFor(() =>
      expect(calls).toEqual([
        { enabled_oidc_providers: ["microsoft", "google"] },
      ]),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("refuses the switches, with the reason, to a reader who cannot change them", async () => {
    mount([GOOGLE], {
      onScreen: ["google"],
      allow: { authentication_policy: ["read"] },
    });
    expect(
      await screen.findByText(
        "Only an admin or ops can change sign-in methods",
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("switch", { name: "Continue with Google" }),
    ).toHaveProperty("disabled", true);
  });

  // An admin cannot add a provider here, so a deployment with none says so
  // rather than rendering an empty list.
  it("says so when the deployment configured no provider", async () => {
    mount([]);
    expect(
      await screen.findByText(/no external provider configured/i),
    ).toBeTruthy();
    expect(screen.queryByRole("switch")).toBeNull();
  });
});
