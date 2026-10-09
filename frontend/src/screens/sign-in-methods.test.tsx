/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
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

// Which ways contacts may sign in. The list is the DEPLOYMENT's — an admin
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

// The provider switches take installation_settings:update; the group→role map
// takes the admin-only authentication_policy:update. The default holds both, so
// a caller who can work the whole card is the common case; a test that wants to
// prove the map is withheld passes an allow that omits the sign-in-policy grant.
const fullSignInGrants: GrantSpec = {
  installation_settings: ["read", "update"],
  authentication_policy: ["read", "update"],
};

function mount(
  providers: { key: string; label: string; enabled: boolean }[],
  groupRoleMap: Record<string, string> = {},
  allow: GrantSpec = fullSignInGrants,
) {
  const calls: unknown[] = [];
  const refusing = { writes: false, reads: false };
  const fetchMock = vi.fn(async (request: Request) => {
    if (request.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method === "PATCH" ? refusing.writes : refusing.reads) {
      return jsonResponse({ title: "Unavailable", status: 503 }, 503);
    }
    if (request.method === "PATCH") {
      calls.push(JSON.parse(await request.text()));
      return new Response(null, { status: 204 });
    }
    // The grant editor picks from the roles the server says this reader may
    // hand out, the same list the invite form reads.
    if (request.url.includes("/users/assignable-roles")) {
      return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
    }
    // The narrow authentication-policy projection this card reads — not the
    // installation aggregate, which answers a different route.
    return jsonResponse({
      sign_in_providers: providers,
      require_sso: false,
      require_mfa: false,
      oidc_group_role_map: groupRoleMap,
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
  return { calls, refusing, qc };
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
    // Waited on rather than read straight after the click: the request is only
    // recorded when the mutation's fetch resolves, so a bare assertion here
    // would pass on timing rather than on the body that was sent.
    await waitFor(() =>
      expect(calls).toEqual([{ enabled_oidc_providers: ["microsoft"] }]),
    );
  });

  it("sends the provider added back when one is switched on", async () => {
    const { calls } = mount([
      { key: "google", label: "Google", enabled: false },
      { key: "microsoft", label: "Microsoft", enabled: true },
    ]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("switch", { name: /google/i }));
    await waitFor(() =>
      expect(calls).toEqual([
        { enabled_oidc_providers: ["microsoft", "google"] },
      ]),
    );
  });

  // A failed refetch replaces the switches with the gate's error arm; a save
  // failure about those switches has nothing left to stand beside.
  it("drops a save failure once the settings themselves fail to load", async () => {
    const { refusing, qc } = mount([
      { key: "google", label: "Google", enabled: true },
    ]);
    const user = userEvent.setup();
    refusing.writes = true;
    await user.click(await screen.findByRole("switch", { name: /google/i }));
    expect(await screen.findByText("Change not saved")).toBeTruthy();

    refusing.reads = true;
    await act(() => qc.refetchQueries());

    expect(
      await screen.findByText("Could not load this view. Reload the page."),
    ).toBeTruthy();
    expect(screen.queryByText("Change not saved")).toBeNull();
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

describe("the group role grant editor", () => {
  it("renders each stored mapping with its group and granted role", async () => {
    mount([], { engineering: "manager" });
    expect(await screen.findByDisplayValue("engineering")).toBeTruthy();
    // Its own region, with headings that outlast whatever is typed in a row.
    const panel = screen.getByRole("region", { name: "Group role grants" });
    const headers = within(panel)
      .getAllByRole("columnheader")
      .map((header) => header.textContent);
    expect(headers).toEqual(["IdP group", "Granted role", "Actions"]);
    // The role reads under its product name, the same label the roster uses.
    const role = screen.getByRole("combobox", { name: /granted role/i });
    expect(role.textContent).toContain("Team lead");
  });

  // The whole map travels: the setting replaces rather than merges, so the
  // stored mapping must ride along with the added one.
  it("adds a mapping and saves the whole map", async () => {
    const { calls } = mount([], { sales: "rep" });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /add group/i }));
    const groups = screen.getAllByRole("textbox", { name: /idp group/i });
    const added = groups[groups.length - 1];
    if (!added) {
      throw new Error("the added row rendered no group input");
    }
    await user.type(added, "platform");
    const roles = screen.getAllByRole("combobox", { name: /granted role/i });
    const addedRole = roles[roles.length - 1];
    if (!addedRole) {
      throw new Error("the added row rendered no role picker");
    }
    await user.click(addedRole);
    await user.click(await screen.findByRole("option", { name: "Ops" }));
    await user.click(
      screen.getByRole("button", { name: /save group grants/i }),
    );
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { sales: "rep", platform: "ops" } },
      ]),
    );
  });

  it("removes a mapping and saves the map without it", async () => {
    const { calls } = mount([], { sales: "rep", engineering: "manager" });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Remove sales" }),
    );
    await user.click(
      screen.getByRole("button", { name: /save group grants/i }),
    );
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { engineering: "manager" } },
      ]),
    );
  });

  // Mirrors the server's refusal so it reaches the admin before a request
  // does: the save is barred and says why, and nothing is sent.
  it("refuses a blank group name before any request is made", async () => {
    const { calls } = mount([], {});
    const user = userEvent.setup();
    // The empty map says what it costs: nothing extra is granted.
    expect(await screen.findByText(/no groups are mapped/i)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /add group/i }));
    const save = screen.getByRole("button", { name: /save group grants/i });
    expect(save.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(/every mapping needs a group name/i)).toBeTruthy();
    await user.click(save);
    expect(calls).toEqual([]);
  });

  // The setting's honest cost stands beside the editor, not in a tooltip: the
  // directory only ever grants here, and mapping a group onto Admin is a grant
  // of admin.
  it("carries the grant-only warning where the map is edited", async () => {
    mount([], {});
    expect(
      await screen.findByText(/grants the mapped roles and never removes any/i),
    ).toBeTruthy();
    expect(
      screen.getByText(/removing a member from an idp group does not take/i),
    ).toBeTruthy();
    expect(
      screen.getByText(/mapping a group onto admin grants admin/i),
    ).toBeTruthy();
  });

  // Writing the map grants roles, so it is admin-only
  // (authentication_policy:update) — a caller who administers installation
  // settings but not the sign-in policy sees the map read-only rather than an
  // editor that would 403 on save. The provider switches, which ARE
  // installation_settings:update, stay theirs to work; this asserts only that
  // the grant editor withholds itself.
  it("shows the map read-only to a caller without the sign-in-policy grant", async () => {
    mount([], { sales: "rep" }, { installation_settings: ["read", "update"] });
    const group = await screen.findByRole("textbox", { name: /idp group/i });
    expect(group.hasAttribute("disabled")).toBe(true);
    expect(
      screen
        .getByRole("button", { name: /add group/i })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(
      screen
        .getByRole("button", { name: /save group grants/i })
        .hasAttribute("disabled"),
    ).toBe(true);
  });
});
