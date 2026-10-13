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
  openRoleChange,
  ROSTER,
  render,
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
    expect(screen.getByRole("columnheader", { name: "Member" })).toBeTruthy();

    // No menu the verbs would live behind, and no role or team COLUMN: this
    // reader is not sent `roles` or `team_ids` at all (handlers_roster.go
    // withholds the privileged projection), so there is no fact to state,
    // which is why the fixture above answers them the narrow roster the server
    // would.
    expect(screen.queryByRole("columnheader", { name: "Role" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Teams" })).toBeNull();
    expect(screen.queryByText("Read-only")).toBeNull();
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    // The deactivated member is absent for the same reason: `include_inactive`
    // is honoured only for a caller who passes that read.
    expect(screen.queryByText("Otto Off")).toBeNull();

    // Inviting IS the admin's, and the card SAYS it is withheld rather than
    // simply dropping the verb: the page opens for every seat, so a roster with
    // no explanation reads as "this installation cannot add contacts".
    expect(screen.queryByRole("button", { name: /invite member/i })).toBeNull();
    // Matched on the KEY's text, not on the words "admins only": the string
    // stopped saying that when these became delegatable grants.
    expect(
      screen.getByText(new RegExp(en["users.roster.readOnly"], "i")),
    ).toBeTruthy();
    expect(screen.queryByLabelText(/^Email/)).toBeNull();
  });

  // The escalation Codex found: `user_admin:update` WITHOUT the read. The
  // roster omits every member's roles for this caller, the change-role dialog
  // would read the missing field as [], and `ChangeUserRole` REPLACES the whole
  // role set, so picking a role would silently drop every other role the
  // target holds, none of which this reader can see. The verb is withheld until
  // the read is there too.
  it("offers no role change to a write holder who cannot read the roster", async () => {
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
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /invite member/i })).toBeNull();
  });

  // And the same holder WITH the read gets all of it, so the case above is not
  // passing because the fixture refuses everyone.
  it("offers the role change once the write holder can read the roster", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["read", "create", "update", "delete"] },
      }),
    );
    render(<UsersAdminCard />);

    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(
      (await rowMenu(user, "Nora None")).getByText("Change role"),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /invite member/i })).toBeTruthy();
  });

  it("carries the invite verb on the roster's own card and counts the roster per status", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    // ONE untitled card, because there is one subject and the page head
    // already names it: inviting is a verb on the roster, not a card of its own.
    expect(screen.queryByRole("heading", { name: /^Members$/ })).toBeNull();
    expect(document.querySelectorAll("section")).toHaveLength(1);
    const members = screen.getByText("Ada Active").closest("section");
    if (!(members instanceof HTMLElement)) {
      throw new Error("the roster is not a card of its own");
    }
    expect(
      within(members).getByRole("button", { name: /invite member/i }),
    ).toBeTruthy();
    // All counts what the roster holds, the deactivated member and the
    // workspace's own agent seat included, because the read opts into both and a
    // count that skipped either would disagree with the rows beneath it.
    expect(
      within(members).getByRole("button", { name: /^All\W+5$/ }),
    ).toBeTruthy();
    expect(
      within(members).getByRole("button", { name: /^Active\W+3$/ }),
    ).toBeTruthy();
    expect(
      within(members).getByRole("button", { name: /^Invited\W+1$/ }),
    ).toBeTruthy();
    expect(
      within(members).getByRole("button", { name: /^Deactivated\W+1$/ }),
    ).toBeTruthy();
    // And the invite fields are not on the page until the dialog carries them.
    expect(
      within(members).queryByPlaceholderText("name@company.com"),
    ).toBeNull();
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
    const user = userEvent.setup();
    // The reader holds every user_admin verb, and Ada is an admin they may not
    // touch: the server lists nothing on her, so her row draws no menu and
    // says her role cannot be changed here.
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
    expect(roleShown(ada)).toBe("Admin");
    expect(within(ada).getByText("You cannot change this role")).toBeTruthy();
    // The same reader keeps every verb on a member the server lists them on.
    expect(
      (await rowMenu(user, "Nora None")).getByText("Change role"),
    ).toBeTruthy();
  });

  // The roster is one table: every row draws its role, teams, status and the
  // menu trigger in the same columns, so nothing shifts with a row's state and
  // a status badge is never squeezed into "Ac…" beside a picker.
  it("lays every member out in the same columns", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const headers = screen
      .getAllByRole("columnheader")
      .map((header) => header.textContent);
    expect(headers).toEqual(["Member", "Role", "Teams", "Status", "Actions"]);
    const row = rowFor("Ada Active");
    const cells = within(row).getAllByRole("cell");
    const cell = (index: number) => {
      const found = cells[index];
      if (found === undefined) {
        throw new Error(`the member row has no column ${index}`);
      }
      return within(found);
    };
    expect(cell(0).getByText("ada@acme.test")).toBeTruthy();
    expect(roleShown(row)).toBe("Admin");
    expect(cell(2).getByText("No team")).toBeTruthy();
    expect(cell(3).getByText("Active")).toBeTruthy();
    expect(
      cell(4).getByRole("button", {
        name: /actions for ada active/i,
      }),
    ).toBeTruthy();
    // The verbs are BEHIND the menu, so the closed row draws none of them, and
    // the role is read as text on every row rather than a picker on some.
    expect(within(row).queryByRole("combobox")).toBeNull();
    expect(within(row).queryByText("Deactivate")).toBeNull();
    expect(within(row).queryByText(/set-password link/i)).toBeNull();
  });

  // The search and the status filter narrow the roster already read.
  it("narrows the roster by status and by name or email", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: /^Deactivated\W+1$/ }));
    expect(screen.getByText("Otto Off")).toBeTruthy();
    expect(screen.queryByText("Ada Active")).toBeNull();

    await user.click(screen.getByRole("button", { name: /^All\W+5$/ }));
    await user.type(
      screen.getByRole("searchbox", { name: "Search members" }),
      "IVY@",
    );
    expect(screen.getByText("Ivy Invited")).toBeTruthy();
    expect(screen.queryByText("Ada Active")).toBeNull();

    await user.type(
      screen.getByRole("searchbox", { name: "Search members" }),
      "nobody",
    );
    expect(screen.getByText("No members match this search.")).toBeTruthy();
  });

  // The Teams column names the teams `team_ids` lists, from the team roster.
  it("names each member's teams", async () => {
    const routed = backend([]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.method === "GET" && req.url.includes("/v1/teams")) {
          return jsonResponse({
            data: [{ id: "t-dach", name: "DACH Sales" }],
            page: { has_more: false },
          });
        }
        if (req.method === "GET" && /\/v1\/users\?/.test(req.url)) {
          return jsonResponse({
            ...ROSTER,
            data: ROSTER.data.map((member) =>
              member.id === "u-active"
                ? { ...member, team_ids: ["t-dach"] }
                : member,
            ),
          });
        }
        return routed(req);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(within(rowFor("Ada Active")).getByText("DACH Sales")).toBeTruthy(),
    );
    expect(within(rowFor("Nora None")).getByText("No team")).toBeTruthy();
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
    // No role change at all: the seat's authority comes from a passport and the
    // contact it names, never from a role of its own, and the role column says
    // so as the row's ANSWER.
    expect(roleShown(agent)).toMatch(/agent passports, has no role/i);
    // No set-password link: the seat holds no password by construction, which
    // is what makes it a thing that signs in nowhere. Its menu still opens —
    // deactivating the seat is a posture an operator is entitled to take.
    const agentVerbs = await rowMenu(user, "Margince Agent");
    expect(agentVerbs.queryByText(/set-password link/i)).toBeNull();
    expect(agentVerbs.queryByText("Change role")).toBeNull();
    expect(agentVerbs.getByText("Deactivate")).toBeTruthy();

    // A contact's row is untouched by any of that — and the link's absence above
    // has to be about the AGENT rather than about an installation that mints no
    // links at all, so the same verb is asserted PRESENT here.
    const contact = rowFor("Nora None");
    expect(within(contact).queryByText("Agent")).toBeNull();
    const contactVerbs = await rowMenu(user, "Nora None");
    expect(contactVerbs.getByText(/set-password link/i)).toBeTruthy();
    expect(contactVerbs.getByText("Change role")).toBeTruthy();
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
    await userEvent.click(
      screen.getByRole("button", { name: /invite member/i }),
    );
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
    // Focus lands on the member's own cell, which reads back the outcome.
    expect(document.activeElement).toBe(
      row.querySelector(".users-roster-member"),
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
    const change = await openRoleChange(user, "Ada Active");
    await pickOption(user, change.select, "User");
    await user.click(change.confirm);

    // The refusal reads in the dialog AND under the member, so closing the
    // dialog does not take the explanation with it.
    await waitFor(() => expect(within(active).getByRole("alert")).toBeTruthy());
    expect(within(active).getByText(/leave no admin/i)).toBeTruthy();
    expect(change.dialog.getByText(/leave no admin/i)).toBeTruthy();
    // The refused change left the role untouched, so the row must read the
    // role the member still holds — anything else would claim a change the
    // server rejected.
    expect(roleShown(active)).toBe("Admin");
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

    const row = await screen.findByTestId("member-u-invited");
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
