/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { en } from "../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  jsonResponse,
  ROSTER,
  render,
  roleSelect,
  roleShown,
  rowFor,
  rowMenu,
} from "./users-admin.testkit";

// The admin member-management card renders the include-inactive roster and drives
// the invite / role / deactivate / reactivate seams; the server stays the RBAC
// authority (this suite asserts the wire calls, not the gate).

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UsersAdminCard", () => {
  it("gives a non-admin the roster and withholds only the controls that change it", async () => {
    vi.stubGlobal("fetch", backend([], { roles: ["rep"] }));
    render(<UsersAdminCard />);

    // The roster itself is NOT admin surface: `GET /users` answers 200 to any
    // authenticated principal, and who is on the team is not an admin's private
    // question. So a rep reads the list.
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(screen.getByRole("heading", { name: /^Users$/ })).toBeTruthy();

    // No picker, and no menu the verbs would live behind. Nor any role TEXT:
    // this reader is not sent `roles` at all (handlers_roster.go withholds the
    // privileged projection), so there is no fact to state — which is why the
    // fixture above answers them the narrow roster the server would.
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByText("Read-only")).toBeNull();
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    // The deactivated member is absent for the same reason: `include_inactive`
    // is honoured only for a caller who passes that read.
    expect(screen.queryByText("Otto Off")).toBeNull();

    // Inviting IS the admin's, and the card SAYS it is withheld rather than
    // simply dropping the verb: the page opens for every seat, so a roster with
    // no explanation reads as "this installation cannot add contacts".
    expect(screen.queryByRole("button", { name: /invite user/i })).toBeNull();
    // Matched on the KEY's text, not on the words "admins only": the string
    // stopped saying that when these became delegatable grants.
    expect(
      screen.getByText(new RegExp(en["users.adminOnly"], "i")),
    ).toBeTruthy();
    expect(screen.queryByLabelText(/^Email/)).toBeNull();
  });

  // The escalation Codex found: `user_admin:update` WITHOUT the read. The
  // roster omits every member's roles for this caller, `RoleCell` would read the
  // missing field as [], and `ChangeUserRole` REPLACES the whole role set — so
  // picking a role would silently drop every other role the target holds, none
  // of which this reader can see. The picker is withheld until the read is
  // there too.
  it("offers no role picker to a write holder who cannot read the roster", async () => {
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["create", "update", "delete"] },
      }),
    );
    render(<UsersAdminCard />);

    // The positive control first: a member row only a resolved snapshot draws.
    // Without it every assertion below would run against the loading render,
    // where each predicate reads false and nothing is offered anyway.
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /invite user/i })).toBeNull();
  });

  // And the same holder WITH the read gets all of it, so the case above is not
  // passing because the fixture refuses everyone.
  it("offers the picker once the write holder can read the roster", async () => {
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["read", "create", "update", "delete"] },
      }),
    );
    render(<UsersAdminCard />);

    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(screen.getAllByRole("combobox").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: /invite user/i })).toBeTruthy();
  });

  it("carries the invite verb in the card's header and no count or intro", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(screen.getAllByRole("heading", { name: /^Users$/ })).toHaveLength(1);
    const header = screen
      .getByRole("heading", { name: /^Users$/ })
      .closest(".panel-head");
    if (!(header instanceof HTMLElement)) {
      throw new Error("the members card has no header band");
    }
    expect(
      within(header).getByRole("button", { name: /invite user/i }),
    ).toBeTruthy();
    // The page subtitle already says who is listed; a count pill and an intro
    // repeating it are gone.
    expect(screen.queryByText(/^\d+ users$/)).toBeNull();
    expect(screen.queryByText(/including deactivated/i)).toBeNull();
    expect(screen.queryByPlaceholderText("name@company.com")).toBeNull();
  });

  it("renders the include-inactive roster with per-status actions", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);

    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(screen.getByText("Otto Off")).toBeTruthy();
    // The roster request opts into the inactive members.
    // (asserted indirectly: the deactivated member is present at all.)
    expect(
      (await rowMenu(user, "Ada Active")).getByText("Deactivate"),
    ).toBeTruthy();
    expect(
      (await rowMenu(user, "Otto Off")).getByText("Reactivate"),
    ).toBeTruthy();
  });

  it("offers on a member only the verbs the server lists for them", async () => {
    // The reader holds every user_admin verb, and Ada is the only admin: the
    // server lists nothing on her, so her row draws no menu and her picker is
    // the same control, refused with the reason beside it.
    const routed = backend([]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.method === "GET" && /\/v1\/users\?/.test(req.url)) {
          return jsonResponse({
            ...ROSTER,
            data: ROSTER.data.map((member) =>
              member.id === "u-active"
                ? { ...member, allowed_actions: [] }
                : member,
            ),
          });
        }
        return routed(input, init);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const ada = rowFor("Ada Active");
    expect(
      within(ada).queryByRole("button", { name: /actions for/i }),
    ).toBeNull();
    const picker = roleSelect(ada, "Ada Active");
    expect(picker.disabled).toBe(true);
    expect(roleShown(ada, "Ada Active")).toBe("Admin");
    const reason = within(ada).getByText(en["users.role.lastAdmin"]);
    expect(picker.getAttribute("aria-describedby")).toContain(reason.id);
    // The same reader keeps every verb on a member the server lists them on.
    expect(roleSelect(rowFor("Nora None"), "Nora None")).toBeTruthy();
  });

  it("draws a member as one table row with their verbs behind the menu", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const row = rowFor("Ada Active");
    expect(within(row).getByText("ada@acme.test")).toBeTruthy();
    expect(
      within(row).getByRole("combobox", { name: /set role for ada/i }),
    ).toBeTruthy();
    expect(
      within(row).getByRole("button", { name: /actions for ada active/i }),
    ).toBeTruthy();
    expect(within(row).queryByText("Deactivate")).toBeNull();
    expect(within(row).queryByText(/set-password link/i)).toBeNull();
  });

  // The agent seat is listed — a client resolving the owner of a record it owns
  // has to find it — but it is not a colleague, and the row has to say so. Each
  // absence below is a control the server refuses anyway, so offering it could
  // only produce a 409 an admin cannot act on.
  it("marks the agent seat and offers it no control meant for a colleague", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(screen.getByText("Margince Agent")).toBeTruthy(),
    );

    const agent = rowFor("Margince Agent");
    expect(within(agent).getByText("Agent")).toBeTruthy();
    // No role control at all, not a disabled one: the seat's authority comes
    // from a passport and the contact it names, never from a role of its own. The
    // line stands where the picker would be, as the row's ANSWER.
    expect(
      within(agent).queryByRole("combobox", { name: /set role for/i }),
    ).toBeNull();
    expect(within(agent).getByText(/acts through a passport/i)).toBeTruthy();
    // No set-password link: the seat holds no password by construction, which
    // is what makes it a thing that signs in nowhere. Its menu still opens —
    // deactivating the seat is a posture an operator is entitled to take.
    const agentVerbs = await rowMenu(user, "Margince Agent");
    expect(agentVerbs.queryByText(/set-password link/i)).toBeNull();
    expect(agentVerbs.getByText("Deactivate")).toBeTruthy();

    // A contact's row is untouched by any of that — and the link's absence above
    // has to be about the AGENT rather than about an installation that mints no
    // links at all, so the same verb is asserted PRESENT here.
    const contact = rowFor("Nora None");
    expect(roleSelect(contact, "Nora None")).toBeTruthy();
    expect(within(contact).queryByText("Agent")).toBeNull();
    expect(
      (await rowMenu(user, "Nora None")).getByText(/set-password link/i),
    ).toBeTruthy();
  });

  // Deactivating the seat stays offered — an operator is entitled to that — and
  // the body written for a colleague describes sessions and sign-ins that the seat
  // has none of. What the agent body must NOT say is that scheduled extension
  // jobs stop: a tick acts as the job it is and reads no identity, so that
  // warning would talk an operator out of a safe action for a reason that is
  // no longer true.
  it("says what deactivating the agent seat does and does not stop", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(screen.getByText("Margince Agent")).toBeTruthy(),
    );

    await user.click(
      (await rowMenu(user, "Margince Agent")).getByRole("button", {
        name: /deactivate/i,
      }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(/scheduled extension jobs keep running/i),
    ).toBeTruthy();
    expect(within(dialog).queryByText(/signed out everywhere/i)).toBeNull();
    // The claim this screen used to make, and the one an operator would act on.
    expect(within(dialog).queryByText(/stops every job/i)).toBeNull();
  });

  // The invite form is a dialog the row's verb opens — three inputs, a team
  // fieldset and an access preview are not an answer that fits in a row's right
  // column. So every invite case opens it first, and the row's verb names the
  // whole act ("Invite a member") while the dialog's submit carries the bare
  // one ("Invite").
  const openInvite = async () => {
    await userEvent.click(screen.getByRole("button", { name: /invite user/i }));
    return screen.findByRole("dialog");
  };

  it("invites nobody until the dialog behind the header verb is open", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(screen.queryByLabelText(/^Email/)).toBeNull();
    const dialog = await openInvite();
    // The dialog's own submit reads the plain form of the verb, so the two are
    // tellable apart — for a reader and for `getByRole`.
    expect(
      within(dialog).getByRole("button", { name: /^invite$/i }),
    ).toBeTruthy();
    expect(within(dialog).getByLabelText(/^Email/)).toBeTruthy();
    expect(within(dialog).getByLabelText(/^Full name/)).toBeTruthy();
    expect(
      within(dialog).getByRole("combobox", {
        name: /^Role/,
      }),
    ).toBeTruthy();
  });

  it("invites a member with the entered email, name, and role", async () => {
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const dialog = await openInvite();
    await userEvent.type(
      within(dialog).getByPlaceholderText("name@company.com"),
      "new@acme.test",
    );
    await userEvent.type(
      within(dialog).getByPlaceholderText("Full name"),
      "New Contact",
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: /^invite$/i }),
    );

    await waitFor(() => {
      const post = calls.find(
        (c) => c.method === "POST" && c.url.endsWith("/users"),
      );
      expect(post).toBeTruthy();
      expect(post?.body).toEqual({
        email: "new@acme.test",
        display_name: "New Contact",
        role: "rep",
        team_ids: [],
      });
    });
  });

  it("deactivates an active member through the deactivate seam", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    await user.click(
      (await rowMenu(user, "Ada Active")).getByText("Deactivate"),
    );
    // Deactivation is destructive (revokes sessions/passports): confirm first.
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: /deactivate/i }),
    );

    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "POST" && c.url.includes("/users/u-active/deactivate"),
        ),
      ).toBe(true),
    );
  });

  // The confirm's own trigger does not survive the action it confirms: a
  // deactivated row offers Reactivate in its place. Handing focus back to the
  // removed button is a silent no-op that leaves focus on <body>, from where the
  // operator's next Tab restarts at the top of the page.
  it("returns focus to the member's row after deactivating, never to the document", async () => {
    const user = userEvent.setup();
    let deactivated = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/users/assignable-roles")) {
          return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
        }
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { email: "admin@acme.test" },
            roles: ["admin"],
            teams: [],
            // The card asks `user_admin` verb by verb and folds the seat, so a
            // snapshot carrying a role alone refuses every control this case is
            // about — and the case would pass with nothing rendered.
            authorization: {
              objects: {
                user_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: true,
                },
              },
              seat_type: "full",
              row_scope: "all",
            },
          });
        }
        if (req.url.includes("/teams") && req.method === "GET") {
          return jsonResponse({ data: [], page: { has_more: false } });
        }
        if (req.url.includes("/users/access-preview")) {
          return jsonResponse({
            role: "rep",
            row_scope: "team",
            objects: {},
            field_masks: [],
            teams: [],
          });
        }
        if (req.url.includes("/users") && req.method === "GET") {
          // The roster the server would really answer with once the seat is off,
          // which is what removes the Deactivate button. A stub that kept
          // reporting the member as active would leave the opener in place and
          // prove nothing about the case.
          return jsonResponse({
            ...ROSTER,
            data: ROSTER.data.map((member) =>
              member.id === "u-active" && deactivated
                ? {
                    ...member,
                    status: "deactivated",
                    allowed_actions: ["change_role", "reactivate"],
                  }
                : member,
            ),
          });
        }
        deactivated = true;
        return jsonResponse({});
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const row = rowFor("Ada Active");
    const verbs = await rowMenu(user, "Ada Active");
    await user.click(verbs.getByText("Deactivate"));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: /deactivate/i,
      }),
    );

    // The menu the verb was pressed in stays open — a dialog restores focus to
    // whatever opened it, so hiding that item first would send focus, on close,
    // to a node that is gone. What it now offers is the opposite verb.
    await waitFor(() => expect(verbs.getByText("Reactivate")).toBeTruthy());
    expect(within(row).getByText("Deactivated")).toBeTruthy();
    expect(document.activeElement).toBe(
      document.getElementById("member-u-active"),
    );
    expect(document.activeElement).not.toBe(document.body);
  });

  it("reactivates a deactivated member", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Otto Off")).toBeTruthy());

    await user.click((await rowMenu(user, "Otto Off")).getByText("Reactivate"));

    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "POST" && c.url.includes("/users/u-off/reactivate"),
        ),
      ).toBe(true),
    );
  });

  it("surfaces a failed member action as an inline alert on the row", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/users/assignable-roles")) {
          return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
        }
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { email: "admin@acme.test" },
            roles: ["admin"],
            teams: [],
            // The card asks `user_admin` verb by verb and folds the seat, so a
            // snapshot carrying a role alone refuses every control this case is
            // about — and the case would pass with nothing rendered.
            authorization: {
              objects: {
                user_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: true,
                },
              },
              seat_type: "full",
              row_scope: "all",
            },
          });
        }
        if (req.url.includes("/teams") && req.method === "GET") {
          return jsonResponse({ data: [], page: { has_more: false } });
        }
        if (req.url.includes("/users/access-preview")) {
          return jsonResponse({
            role: "rep",
            row_scope: "team",
            objects: {},
            field_masks: [],
            teams: [],
          });
        }
        if (req.url.includes("/users") && req.method === "GET") {
          return jsonResponse(ROSTER);
        }
        return jsonResponse(
          { title: "Conflict", detail: "That would leave no admin." },
          409,
        );
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const active = rowFor("Ada Active");
    await pickOption(user, roleSelect(active, "ada active"), "User");

    await waitFor(() => expect(within(active).getByRole("alert")).toBeTruthy());
    expect(screen.getByText(/leave no admin/i)).toBeTruthy();
    // The refused change left the role untouched, so the select must read the
    // role the member still holds — anything else would claim a change the
    // server rejected.
    expect(roleShown(active, "ada active")).toBe("Admin");
  });

  // The whole reason the select returns to the held role after a refusal: a
  // select left showing the refused target would make re-picking it a no-op,
  // and the operator's retry would silently never reach the server.
  it("surfaces a failed invite as an inline error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/users/assignable-roles")) {
          return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
        }
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { email: "admin@acme.test" },
            roles: ["admin"],
            teams: [],
            // The card asks `user_admin` verb by verb and folds the seat, so a
            // snapshot carrying a role alone refuses every control this case is
            // about — and the case would pass with nothing rendered.
            authorization: {
              objects: {
                user_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: true,
                },
              },
              seat_type: "full",
              row_scope: "all",
            },
          });
        }
        if (req.url.includes("/teams") && req.method === "GET") {
          return jsonResponse({ data: [], page: { has_more: false } });
        }
        if (req.url.includes("/users/access-preview")) {
          return jsonResponse({
            role: "rep",
            row_scope: "team",
            objects: {},
            field_masks: [],
            teams: [],
          });
        }
        if (req.url.includes("/users") && req.method === "GET") {
          return jsonResponse(ROSTER);
        }
        return jsonResponse(
          { title: "Conflict", detail: "That email already exists." },
          409,
        );
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const dialog = await openInvite();
    await userEvent.type(
      within(dialog).getByPlaceholderText("name@company.com"),
      "dupe@acme.test",
    );
    await userEvent.type(
      within(dialog).getByPlaceholderText("Full name"),
      "Dupe",
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: /^invite$/i }),
    );

    await waitFor(() =>
      expect(screen.getByText(/already exists/i)).toBeTruthy(),
    );
  });

  it("shows an unredeemed invitation as invited, and keeps both ways out of it open", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);

    await screen.findByText("Ivy Invited");
    const row = rowFor("Ivy Invited");
    expect(within(row).getByText("Invited")).toBeTruthy();

    // Both verbs stay reachable, and each closes a different hole. The link is
    // the ONLY route back for an invitation whose token expired — that member
    // has no password, so the self-service reset refuses them. Deactivating is
    // what releases the licensed seat an invitation sent to the wrong address
    // is still holding.
    const user = userEvent.setup();
    await user.click(
      within(row).getByRole("button", { name: /actions for Ivy Invited/i }),
    );
    expect(
      await screen.findByRole("button", { name: /get set-password link/i }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /^deactivate$/i })).toBeTruthy();
  });
});
