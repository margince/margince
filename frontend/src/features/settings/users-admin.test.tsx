/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  jsonResponse,
  ROSTER,
  render,
  roleControl,
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
    expect(
      screen.getByRole("table", { name: en["users.membersTitle"] }),
    ).toBeTruthy();

    // No picker, and no menu the verbs would live behind. Nor any role TEXT:
    // this reader is not sent `roles` at all (handlers_roster.go withholds the
    // privileged projection), so there is no fact to state — which is why the
    // fixture above answers them the narrow roster the server would.
    expect(roleControl(rowFor("Ada Active"), "Ada Active")).toBeNull();
    expect(screen.queryByText("Read-only")).toBeNull();
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    // The deactivated member is absent for the same reason: `include_inactive`
    // is honoured only for a caller who passes that read.
    expect(screen.queryByText("Otto Off")).toBeNull();

    // Inviting is the admin's. The read-only notice is the page's, not this
    // card's (settings-reach.test.tsx, "says once that it is theirs to read").
    expect(screen.queryByRole("button", { name: /invite member/i })).toBeNull();
    expect(screen.queryByText(en["users.role.withheld"])).toBeNull();
    expect(screen.queryByLabelText(/^Email/)).toBeNull();
  });

  // `user_admin:update` without the read: the roster omits roles, and a role
  // change replaces the whole set, so a pick would drop roles nobody saw.
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
    expect(roleControl(rowFor("Ada Active"), "Ada Active")).toBeNull();
    expect(screen.queryByRole("button", { name: /actions for/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /invite member/i })).toBeNull();
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
    expect(roleSelect(rowFor("Ada Active"), "Ada Active")).toBeTruthy();
    expect(screen.getByRole("button", { name: /invite member/i })).toBeTruthy();
  });

  // The page heading names the roster, so the card carries no title. With no
  // shell around it, the invite verb draws outside the panel (PageHeadAction).
  it("puts the invite verb outside the panel, with no title, count or intro", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(screen.queryByRole("heading")).toBeNull();
    const head = document.querySelector(".pagehead-action");
    if (!(head instanceof HTMLElement)) {
      throw new Error("the members card declares no page-head verb");
    }
    expect(
      within(head).getByRole("button", { name: en["users.inviteOpen"] }),
    ).toBeTruthy();
    expect(head.closest(".panel")).toBeNull();
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
    // The reader holds every user_admin verb, and Ada is the only admin. The
    // server lists nothing on her, so her row draws no menu. Her role reads as
    // words, with the reason for a screen reader instead of a chooser.
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
    expect(roleControl(ada, "Ada Active")).toBeNull();
    expect(roleShown(ada)).toBe("Admin");
    expect(within(ada).getByText(en["users.role.lastAdmin"])).toBeTruthy();
    // The same reader keeps every verb on a member the server lists them on.
    expect(roleSelect(rowFor("Nora None"), "Nora None")).toBeTruthy();
  });

  it("draws a member as one table row with their verbs behind the menu", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const row = rowFor("Ada Active");
    expect(within(row).getByText("ada@acme.test")).toBeTruthy();
    expect(roleSelect(row, "Ada Active")).toBeTruthy();
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
    // line stands where the role would be, as the row's ANSWER.
    expect(roleControl(agent, "Margince Agent")).toBeNull();
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

  // A service member is an agent row too. The screen has to say which kind
  // it is looking at and which extension's feed the seat runs.
  describe("a service member", () => {
    const SERVICE = {
      id: "u-feed",
      email: "service-feed@members.invalid",
      display_name: "Openchannel feed",
      status: "active" as const,
      is_agent: true,
      service_extension: "openchannel",
      roles: ["service_ingest"],
      allowed_actions: ["deactivate" as const],
    };
    beforeEach(() => {
      ROSTER.data.push(SERVICE);
    });
    afterEach(() => {
      ROSTER.data.splice(ROSTER.data.indexOf(SERVICE), 1);
    });

    it("is labelled as a service member of its extension, not as an agent", async () => {
      vi.stubGlobal("fetch", backend([]));
      render(<UsersAdminCard />);
      await waitFor(() =>
        expect(screen.getByText("Openchannel feed")).toBeTruthy(),
      );

      const row = rowFor("Openchannel feed");
      expect(within(row).getByText("Service member")).toBeTruthy();
      expect(within(row).queryByText("Agent")).toBeNull();
      expect(within(row).getByText("Feed of openchannel")).toBeTruthy();
      expect(within(row).queryByText(/members\.invalid/)).toBeNull();
      expect(within(row).getByText(/fixed role/i)).toBeTruthy();
    });

    it("deactivates through the deactivate seam and says the feed stops", async () => {
      const user = userEvent.setup();
      const calls: { method: string; url: string; body?: unknown }[] = [];
      vi.stubGlobal("fetch", backend(calls));
      render(<UsersAdminCard />);
      await waitFor(() =>
        expect(screen.getByText("Openchannel feed")).toBeTruthy(),
      );

      await user.click(
        (await rowMenu(user, "Openchannel feed")).getByText("Deactivate"),
      );
      const dialog = await screen.findByRole("dialog");
      expect(within(dialog).getByText(/stops that feed/i)).toBeTruthy();
      await user.click(
        within(dialog).getByRole("button", { name: /deactivate/i }),
      );

      await waitFor(() =>
        expect(
          calls.some(
            (c) =>
              c.method === "POST" && c.url.includes("/users/u-feed/deactivate"),
          ),
        ).toBe(true),
      );
    });
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

  it("removes a locked-out member's second factor after a confirm", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());
    expect(
      (await rowMenu(user, "Ada Active")).queryByText(en["users.removeMfa"]),
    ).toBeNull();

    await user.click(
      (await rowMenu(user, "Nora None")).getByText(en["users.removeMfa"]),
    );
    const dialog = await screen.findByRole("dialog");
    expect(calls.some((c) => c.url.includes("/mfa"))).toBe(false);
    await user.click(
      within(dialog).getByRole("button", { name: en["users.removeMfa"] }),
    );

    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.method === "DELETE" && c.url.includes("/users/u-none/mfa"),
        ),
      ).toBe(true),
    );
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

    // The closed menu keeps its items mounted, and now offers the opposite verb.
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
    // The refusal keeps the operator's pick open beside the reason; backing out
    // puts back the role the member still holds.
    expect(roleShown(active)).toBe("User");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(roleShown(active)).toBe("Admin"));
  });

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
