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
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { MAX_GROUP_GRANTS } from "./group-role-grants";
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
  capture_settings: ["read"],
};

type Provider = { key: string; label: string; enabled: boolean };

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
  groupRoleMap: Record<string, string> = {},
  allow: GrantSpec = fullSignInGrants,
  server: {
    required?: boolean;
    ownConfirmed?: boolean;
    ownAnswer?: Promise<Response>;
    app?: "none" | "stored" | "environment";
    microsoftTenant?: string;
    onScreen?: string[];
  } = {},
) {
  const calls: unknown[] = [];
  const appReads: string[] = [];
  const refusing = { writes: false, reads: false };
  let storedMap = groupRoleMap;
  const fetchMock = vi.fn(async (request: Request) => {
    if (request.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.url.endsWith("/v1/me/mfa")) {
      if (server.ownAnswer) {
        return server.ownAnswer;
      }
      const confirmed = server.ownConfirmed ?? true;
      return jsonResponse({
        enrolled: confirmed,
        confirmed,
        recovery_codes_left: confirmed ? 10 : 0,
      });
    }
    if (request.method === "PATCH" ? refusing.writes : refusing.reads) {
      return jsonResponse(
        { title: "Unavailable", status: 503, code: "service_unavailable" },
        503,
      );
    }
    if (request.method === "PATCH") {
      const change = JSON.parse(await request.text());
      calls.push(change);
      storedMap = change.oidc_group_role_map ?? storedMap;
      return new Response(null, { status: 204 });
    }
    // The grant editor picks from the roles the server says this reader may
    // hand out, the same list the invite form reads.
    if (request.url.includes("/users/assignable-roles")) {
      return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
    }
    if (request.url.includes("/installation/oauth-apps/")) {
      appReads.push(request.url);
      const vendor = /\/oauth-apps\/(google|microsoft)$/.exec(request.url)?.[1];
      return server.app
        ? jsonResponse({
            provider: vendor,
            configured: server.app !== "none",
            source: server.app,
            client_id: server.app === "none" ? "" : "client-1",
            ...(vendor === "microsoft" && server.microsoftTenant
              ? { tenant: server.microsoftTenant }
              : {}),
            redirect_uris: [],
          })
        : jsonResponse(
            { title: "Forbidden", status: 403, code: "permission_denied" },
            403,
          );
    }
    // The narrow authentication-policy projection this card reads — not the
    // installation aggregate, which answers a different route.
    return jsonResponse({
      sign_in_providers: providers,
      sign_in_screen: server.onScreen ?? [],
      require_sso: false,
      require_mfa: server.required ?? false,
      oidc_group_role_map: storedMap,
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
  return { calls, appReads, refusing, qc };
}

describe("the sign-in methods card", () => {
  // There is no value of the setting that removes password, so the row says
  // so in words and offers no switch to flip.
  it("shows password as always on, with no switch", async () => {
    mount([GOOGLE], {}, undefined, { onScreen: ["google"], app: "stored" });
    expect(await screen.findByText("Always on")).toBeTruthy();
    expect(
      screen.queryByRole("switch", { name: /email and password/i }),
    ).toBeNull();
  });

  // A failed refetch replaces the switches with the gate's error arm; a save
  // failure about those switches has nothing left to stand beside.
  it("drops a save failure once the settings themselves fail to load", async () => {
    const { refusing, qc } = mount([{ ...GOOGLE, enabled: false }]);
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

  // The verdict is the server's sign_in_screen, never worked out here.
  it("says a provider is on the sign-in screen only when the server draws it", async () => {
    mount([GOOGLE, MICROSOFT], {}, undefined, {
      onScreen: ["google"],
      app: "stored",
      microsoftTenant: "dir",
    });
    expect(await screen.findAllByText("On the sign-in screen")).toHaveLength(1);
    expect(
      await screen.findByText("The Microsoft app cannot be used right now."),
    ).toBeTruthy();
    expect(screen.getAllByText("Not on the sign-in screen")).toHaveLength(1);
  });

  it("says a chosen provider is missing because its app is not set up", async () => {
    mount([GOOGLE], {}, undefined, { app: "none" });
    expect(
      await screen.findByText("The Google app is not set up."),
    ).toBeTruthy();
    expect(screen.getByText("Not on the sign-in screen")).toBeTruthy();
  });

  it("says a Microsoft app with no directory keeps its button off the screen", async () => {
    mount([MICROSOFT], {}, undefined, { app: "environment" });
    expect(
      await screen.findByText(/Microsoft app is not limited to your/),
    ).toBeTruthy();
  });

  it("asks for no app when the reader may not read capture settings", async () => {
    const { appReads } = mount(
      [GOOGLE],
      {},
      {
        installation_settings: ["read", "update"],
        authentication_policy: ["read", "update"],
      },
      { app: "none" },
    );
    expect(
      await screen.findByText(
        "The Continue with Google app cannot be used right now.",
      ),
    ).toBeTruthy();
    expect(appReads).toEqual([]);
  });

  it("says a provider switched off is turned off", async () => {
    mount([{ ...GOOGLE, enabled: false }]);
    expect(await screen.findByText("Turned off")).toBeTruthy();
  });

  // Turning a provider off takes a way in from everyone who uses it, so the
  // switch only opens the question; the typed phrase answers it.
  it("sends nothing on switch-off until the phrase is typed and confirmed", async () => {
    const { calls } = mount([GOOGLE, MICROSOFT], {}, undefined, {
      onScreen: ["google", "microsoft"],
      app: "stored",
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
    const { calls } = mount([GOOGLE], {}, undefined, {
      onScreen: ["google"],
      app: "stored",
    });
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

// Turning the requirement on sends members without a factor to set one up, so
// it asks first. Turning it off takes nothing from anyone, so it saves at once.
describe("the second-factor requirement", () => {
  const requireSwitch = () =>
    screen.findByRole("switch", { name: /require second factor/i });
  const confirmButton = (dialog: HTMLElement) =>
    within(dialog).getByRole("button", { name: /require second factor/i });
  // Settled means the confirm is live: it is held while the admin's own factor
  // is still being asked for.
  const settledDialog = async () => {
    const dialog = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(confirmButton(dialog).hasAttribute("disabled")).toBe(false),
    );
    return dialog;
  };

  it("asks before requiring a second factor, then saves it", async () => {
    const { calls } = mount([]);
    const user = userEvent.setup();
    await user.click(await requireSwitch());

    const dialog = await settledDialog();
    expect(
      within(dialog).getByText(/asked to set it up before they can continue/i),
    ).toBeTruthy();
    expect(calls).toEqual([]);
    await user.click(confirmButton(dialog));
    await waitFor(() => expect(calls).toEqual([{ require_mfa: true }]));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("sends nothing when the confirm is cancelled", async () => {
    const { calls } = mount([]);
    const user = userEvent.setup();
    await user.click(await requireSwitch());
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls).toEqual([]);
  });

  it("tells an admin with no factor of their own that they are asked to set one up too", async () => {
    mount([], {}, fullSignInGrants, { ownConfirmed: false });
    const user = userEvent.setup();
    await user.click(await requireSwitch());
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(/you have not set up two-step sign-in/i),
    ).toBeTruthy();
  });

  it("leaves the self warning out for an admin who holds a factor", async () => {
    mount([], {}, fullSignInGrants, { ownConfirmed: true });
    const user = userEvent.setup();
    await user.click(await requireSwitch());
    const dialog = await settledDialog();
    expect(
      within(dialog).queryByText(/you have not set up two-step sign-in/i),
    ).toBeNull();
  });

  it("holds the confirm while the admin's own factor is unknown", async () => {
    let answer: (response: Response) => void = () => undefined;
    const ownAnswer = new Promise<Response>((resolve) => {
      answer = resolve;
    });
    const { calls } = mount([], {}, fullSignInGrants, { ownAnswer });
    const user = userEvent.setup();
    await user.click(await requireSwitch());
    const dialog = await screen.findByRole("dialog");

    expect(confirmButton(dialog).hasAttribute("disabled")).toBe(true);
    await user.click(confirmButton(dialog));
    expect(calls).toEqual([]);

    answer(
      jsonResponse({
        enrolled: true,
        confirmed: true,
        recovery_codes_left: 10,
      }),
    );
    await settledDialog();
  });

  // A failed read is no proof the admin holds a factor.
  it("warns the admin when their own factor cannot be read", async () => {
    mount([], {}, fullSignInGrants, {
      ownAnswer: Promise.resolve(
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
      ),
    });
    const user = userEvent.setup();
    await user.click(await requireSwitch());
    const dialog = await settledDialog();
    expect(
      within(dialog).getByText(/you have not set up two-step sign-in/i),
    ).toBeTruthy();
  });

  it("turns the requirement off without asking", async () => {
    const { calls } = mount([], {}, fullSignInGrants, { required: true });
    const user = userEvent.setup();
    const control = await requireSwitch();
    expect(control.getAttribute("aria-checked")).toBe("true");
    await user.click(control);
    await waitFor(() => expect(calls).toEqual([{ require_mfa: false }]));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("is read-only for a caller who cannot update the installation", async () => {
    mount(
      [],
      {},
      { installation_settings: ["read"], authentication_policy: ["read"] },
    );
    expect((await requireSwitch()).hasAttribute("disabled")).toBe(true);
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
    expect(headers).toEqual(["Directory group", "Role they get", "Actions"]);
    // The role reads under its product name, the same label the roster uses.
    const role = screen.getByRole("combobox", {
      name: /role for the .* group/i,
    });
    expect(role.textContent).toContain("Team lead");
  });

  // The whole map travels: the setting replaces rather than merges, so the
  // stored mapping must ride along with the added one.
  it("adds a mapping and saves the whole map", async () => {
    const { calls } = mount([], { sales: "rep" });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /add group/i }));
    const groups = screen.getAllByRole("textbox", { name: /directory group/i });
    const added = groups[groups.length - 1];
    if (!added) {
      throw new Error("the added row rendered no group input");
    }
    await user.type(added, "platform");
    const roles = screen.getAllByRole("combobox", {
      name: /role for the .* group/i,
    });
    const addedRole = roles[roles.length - 1];
    if (!addedRole) {
      throw new Error("the added row rendered no role picker");
    }
    await user.click(addedRole);
    await user.click(await screen.findByRole("option", { name: "Ops" }));
    await user.click(screen.getByRole("button", { name: /save group roles/i }));
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
      await screen.findByRole("button", { name: "Remove the sales mapping" }),
    );
    await user.click(screen.getByRole("button", { name: /save group roles/i }));
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { engineering: "manager" } },
      ]),
    );
  });

  it("hands focus to Add group once the saved map clears Save away", async () => {
    mount([], { sales: "rep", engineering: "manager" });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Remove the sales mapping" }),
    );
    await user.click(screen.getByRole("button", { name: /save group roles/i }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /save group roles/i }),
      ).toBeNull(),
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: /add group/i }),
      ),
    );
  });

  it("hands focus to the first group when a full map leaves Add disabled", async () => {
    const full = Object.fromEntries(
      Array.from({ length: MAX_GROUP_GRANTS }, (_, index) => [
        `team-${index}`,
        "rep",
      ]),
    );
    mount([], full);
    const user = userEvent.setup();
    const groups = await screen.findAllByRole("textbox", {
      name: /directory group/i,
    });
    const last = groups.at(-1);
    if (!last) {
      throw new Error("the full map rendered no group input");
    }
    await user.type(last, "x");
    await user.click(screen.getByRole("button", { name: /save group roles/i }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: /save group roles/i }),
      ).toBeNull(),
    );
    await waitFor(() => expect(document.activeElement).toBe(groups[0]));
    const add = screen.getByRole("button", { name: /add group/i });
    expect(add.hasAttribute("disabled")).toBe(true);
  });

  // Mirrors the server's refusal so it reaches the admin before a request
  // does: the save is barred and says why, and nothing is sent.
  it("refuses a blank group name before any request is made", async () => {
    const { calls } = mount([], {});
    const user = userEvent.setup();
    // The empty map says what it costs: nothing extra is granted.
    expect(await screen.findByText(/no groups are mapped/i)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /add group/i }));
    const save = screen.getByRole("button", { name: /save group roles/i });
    expect(save.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(/every mapping needs a group name/i)).toBeTruthy();
    await user.click(save);
    expect(calls).toEqual([]);
  });

  // The directory only ever grants, and mapping a group onto Admin grants
  // admin. That cost stands beside the editor once a row exists to weigh.
  it("carries the grant-only warning once a mapping is drafted", async () => {
    mount([], {});
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /add group/i }));
    expect(screen.getByText(/groups only ever add a role/i)).toBeTruthy();
    expect(
      screen.getByText(
        /taking someone out of a directory group does not take/i,
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(/a group mapped to admin makes every invited member/i),
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
    const group = await screen.findByRole("textbox", {
      name: /directory group/i,
    });
    expect(group.hasAttribute("disabled")).toBe(true);
    expect(screen.queryByRole("button", { name: /add group/i })).toBeNull();
    expect(
      screen.queryByRole("button", { name: /save group roles/i }),
    ).toBeNull();
  });

  // Save stands only while the draft differs from what is stored.
  it("offers save only once the draft changes", async () => {
    mount([], { sales: "rep" });
    const user = userEvent.setup();
    expect(await screen.findByDisplayValue("sales")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /save group roles/i }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: /add group/i }));
    expect(
      screen.getByRole("button", { name: /save group roles/i }),
    ).toBeTruthy();
  });
});
