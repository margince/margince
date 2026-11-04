/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  jsonResponse,
  openRoleDialog,
  ROSTER,
  render,
  roleControl,
  roleShown,
  rowFor,
} from "./users-admin.testkit";

// The member card's role picker: what each row reads back and offers, and the
// role seam it drives. The server stays the authority on every change.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UsersAdminCard role picker", () => {
  it("reads back each member's current role", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(roleShown(rowFor("Ada Active"))).toBe("Admin");
    expect(roleShown(rowFor("Otto Off"))).toBe("Read-only");
  });

  it("names a member holding no role as unset", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    expect(roleShown(rowFor("Nora None"))).toBe(en["field.unset"]);
    // "Not set" is never an entry. The list offers the assignable roles in the
    // server's key order.
    const { chooser } = await openRoleDialog(
      user,
      rowFor("Nora None"),
      "nora none",
    );
    await user.click(chooser);
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual([
      "Admin",
      "Management",
      "Team lead",
      "Ops",
      "Read-only",
      "User",
    ]);
  });

  // Any choice replaces the whole set. A multi-role member read as one role
  // would let an admin strip privileges they never saw.
  it("names the roles a multi-role member holds, and warns in the chooser", async () => {
    const user = userEvent.setup();
    const twoRoles = {
      ...ROSTER,
      data: ROSTER.data.map((u) =>
        u.id === "u-none" ? { ...u, roles: ["manager", "ops"] } : u,
      ),
    };
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
        return jsonResponse(twoRoles);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    const nora = rowFor("Nora None");
    expect(roleShown(nora)).toMatch(/^Team lead, Ops/);
    expect(within(nora).getByText(en["users.rolesReplace"])).toBeTruthy();
    expect(within(nora).getByRole("status").textContent).toBe("");

    const { chooser } = await openRoleDialog(user, nora, "Nora None");
    expect(chooser.textContent).toMatch(/^Team lead, Ops/);
    await user.click(chooser);
    const set = await screen.findByRole("option", { name: /^Team lead, Ops/ });
    expect(set.getAttribute("aria-selected")).toBe("true");
    expect(roleControl(nora, "Nora None")?.textContent).toMatch(/Team lead/);
    expect(within(nora).getByRole("status").textContent).toBe(
      en["users.rolesReplace"],
    );
  });

  it("warns before Save only when the picked role cannot capture and the member has connections", async () => {
    const user = userEvent.setup();
    const server = backend([]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/users/assignable-roles")) {
          return jsonResponse({
            roles: SEEDED_ASSIGNABLE_ROLES.map((role) => ({
              ...role,
              captures_mail: role.key !== "read_only",
            })),
          });
        }
        const answer = await server(req);
        const rosterRead =
          req.method === "GET" &&
          new URL(req.url).pathname.endsWith("/v1/users");
        if (!rosterRead) {
          return answer;
        }
        const page: typeof ROSTER = await answer.json();
        return jsonResponse({
          ...page,
          data: page.data.map((member) =>
            member.id === "u-active"
              ? { ...member, connected_mailboxes: 2 }
              : member,
          ),
        });
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    const warning =
      "Their 2 connected mailboxes and calendars will stop capturing.";

    const ada = await openRoleDialog(user, rowFor("Ada Active"), "ada active");
    await pickOption(user, ada.chooser, "Read-only");
    expect(within(ada.dialog).getByText(warning)).toBeTruthy();
    await pickOption(user, ada.chooser, "Team lead");
    expect(within(ada.dialog).queryByText(warning)).toBeNull();
    await user.click(
      within(ada.dialog).getByRole("button", { name: "Cancel" }),
    );

    const nora = await openRoleDialog(user, rowFor("Nora None"), "nora none");
    await pickOption(user, nora.chooser, "Read-only");
    expect(within(nora.dialog).queryByText(/will stop capturing/)).toBeNull();
  });

  it("sets a member's role through the role seam", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const active = rowFor("Ada Active");
    const { chooser, save } = await openRoleDialog(user, active, "ada active");
    await pickOption(user, chooser, "Team lead");
    expect(calls.some((c) => c.method === "PATCH")).toBe(false);
    await user.click(save);

    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "PATCH" &&
            c.url.includes("/users/u-active/role") &&
            (c.body as { role?: string })?.role === "manager",
        ),
      ).toBe(true),
    );
  });

  it("lets the same role be re-picked after a refusal", async () => {
    const user = userEvent.setup();
    const patches: string[] = [];
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
        patches.push(req.url);
        return jsonResponse({ title: "Conflict", detail: "Try again." }, 409);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const active = rowFor("Ada Active");
    const { chooser, save } = await openRoleDialog(user, active, "ada active");
    await pickOption(user, chooser, "User");
    await user.click(save);
    await waitFor(() => expect(patches).toHaveLength(1));

    // The SAME target again — the retry the operator would make.
    await pickOption(user, chooser, "User");
    await user.click(save);
    await waitFor(() => expect(patches).toHaveLength(2));
  });

  // A settled mutation whose roster refetch is still outstanding would render
  // the member's replaced role from the stale cache — the operator would watch
  // their change appear and then undo itself.
  it("stays pending until the refreshed roster lands", async () => {
    const user = userEvent.setup();
    let rosterReads = 0;
    // Built up front rather than captured lazily: the refetch has to be held
    // open from the moment it starts, and a deferred that only exists once the
    // request arrives would race the assertions below.
    let releaseRoster = () => {};
    const rosterHeld = new Promise<void>((resolve) => {
      releaseRoster = resolve;
    });
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
          rosterReads += 1;
          if (rosterReads === 1) {
            return jsonResponse(ROSTER);
          }
          // Hold the refetch open so the window between "mutation done" and
          // "new roster in hand" is observable rather than a race.
          await rosterHeld;
          return jsonResponse({
            ...ROSTER,
            data: ROSTER.data.map((u) =>
              u.id === "u-active" ? { ...u, roles: ["manager"] } : u,
            ),
          });
        }
        return jsonResponse({}, 200);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const active = rowFor("Ada Active");
    const { dialog, chooser, save } = await openRoleDialog(
      user,
      active,
      "ada active",
    );
    await pickOption(user, chooser, "Team lead");
    await user.click(save);
    await waitFor(() => expect(rosterReads).toBe(2));

    // Mid-flight: the dialog holds the role being applied and stays locked. A
    // closed dialog here would show the stale cache's "Admin" through the row.
    expect(dialog.isConnected).toBe(true);
    expect(chooser.textContent).toBe("Team lead");
    expect(chooser).toHaveProperty("disabled", true);

    releaseRoster();
    await waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(roleShown(rowFor("Ada Active"))).toBe("Team lead");
  });

  // A custom role reads under the name its maker gave it, and is offered like
  // any other. A role this reader may not hand out stays on the face of a
  // refused picker rather than reading as unset.
  it("names a custom role and offers only the roles the reader may assign", async () => {
    const user = userEvent.setup();
    const fieldSales = {
      key: "custom_field_sales",
      name: "Field sales",
      is_system: false,
    };
    const routed = backend([]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/users/assignable-roles")) {
          return jsonResponse({
            roles: [
              ...SEEDED_ASSIGNABLE_ROLES.filter((r) => r.key !== "admin"),
              fieldSales,
            ],
          });
        }
        if (req.url.includes("/users?") && req.method === "GET") {
          return jsonResponse({
            ...ROSTER,
            data: ROSTER.data.map((u) =>
              u.id === "u-invited" ? { ...u, roles: [fieldSales.key] } : u,
            ),
          });
        }
        return routed(req);
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ivy Invited")).toBeTruthy());

    expect(roleShown(rowFor("Ivy Invited"))).toBe("Field sales");
    // Ada holds admin, which this reader may not hand out: the row still names
    // it, offers no chooser, and says why.
    const ada = rowFor("Ada Active");
    expect(roleControl(ada, "ada active")).toBeNull();
    expect(roleShown(ada)).toBe("Admin");
    expect(within(ada).getByText(en["users.role.outside"])).toBeTruthy();

    const { chooser } = await openRoleDialog(
      user,
      rowFor("Nora None"),
      "nora none",
    );
    await user.click(chooser);
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toContain("Field sales");
    expect(offered).not.toContain("Admin");
  });
});
