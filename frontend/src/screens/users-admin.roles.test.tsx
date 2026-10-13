/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
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

// The member card's role column and its change-role dialog: what each row reads
// back and offers, and the role seam the dialog drives. The server stays the
// authority on every change.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UsersAdminCard role change", () => {
  it("reads back each member's current role", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(roleShown(rowFor("Ada Active"))).toBe("Admin");
    expect(roleShown(rowFor("Otto Off"))).toBe("Read-only");
  });

  it("reads a member holding no role as such, and confirms nothing until a pick", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    expect(roleShown(rowFor("Nora None"))).toBe("No role");
    const change = await openRoleChange(user, "Nora None");
    // "Set role…" is the picker's face and never an entry: picking it back would
    // set no role, so the confirm stays shut and the list is the assignable
    // roles, in the server's key order, and nothing else.
    expect(change.select.textContent).toBe("Set role…");
    expect(change.confirm.disabled).toBe(true);
    await user.click(change.select);
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

  // Any choice replaces the whole set, so a member holding several roles must
  // read as every one of them, and the dialog says a pick replaces them all;
  // otherwise an admin could strip privileges they were never shown.
  it("names the roles a multi-role member holds", async () => {
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

    // Both held roles are named, under their display labels, on the row and in
    // the dialog, and the dialog says what picking one does.
    expect(roleShown(rowFor("Nora None"))).toBe("Team lead, Ops");
    const change = await openRoleChange(user, "Nora None");
    expect(change.dialog.getByText("Role today: Team lead, Ops")).toBeTruthy();
    expect(change.dialog.getByText(/replaces every role/i)).toBeTruthy();
  });

  it("sets a member's role through the role seam", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const change = await openRoleChange(user, "Ada Active");
    // Nothing is written by the pick itself: the dialog's confirm is the write.
    await pickOption(user, change.select, "Team lead");
    expect(calls.some((c) => c.method === "PATCH")).toBe(false);
    await user.click(change.confirm);

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

    const change = await openRoleChange(user, "Ada Active");
    await pickOption(user, change.select, "User");
    await user.click(change.confirm);
    await waitFor(() => expect(patches).toHaveLength(1));
    // The refusal reads in the dialog and the dialog stays, on the same pick.
    await waitFor(() =>
      expect(change.dialog.getByText(/try again/i)).toBeTruthy(),
    );

    // The SAME target again — the retry the operator would make.
    await user.click(change.confirm);
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

    const change = await openRoleChange(user, "Ada Active");
    await pickOption(user, change.select, "Team lead");
    await user.click(change.confirm);
    await waitFor(() => expect(rosterReads).toBe(2));

    // Mid-flight: the dialog stays open and locked over the role being applied,
    // so the row's stale "Admin" is never presented as the outcome.
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(change.select.disabled).toBe(true);

    releaseRoster();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(roleShown(rowFor("Ada Active"))).toBe("Team lead");
  });

  // A custom role reads under the name its maker gave it, and is offered like
  // any other. A member holding a role this reader may not hand out offers no
  // change at all, and the row says so.
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
    // Ada holds admin, which this reader may not hand out.
    expect(roleShown(rowFor("Ada Active"))).toBe("Admin");
    expect(
      within(rowFor("Ada Active")).getByText("You cannot change this role"),
    ).toBeTruthy();
    expect(
      (await rowMenu(user, "Ada Active")).queryByText("Change role"),
    ).toBeNull();

    const change = await openRoleChange(user, "Nora None");
    await user.click(change.select);
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toContain("Field sales");
    expect(offered).not.toContain("Admin");
  });
});
