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
  ROSTER,
  render,
  roleSelect,
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

    expect(roleShown(rowFor("Ada Active"), "ada active")).toBe("Admin");
    expect(roleShown(rowFor("Otto Off"), "otto off")).toBe("Read-only");
  });

  it("offers the placeholder to a member holding no role", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    expect(roleShown(rowFor("Nora None"), "nora none")).toBe("Set role…");
    // It is a face and never an entry: picking "Set role…" back would set no
    // role, so the list this row opens is the assignable roles, in the server's
    // key order, and nothing else. The options exist only while the popup is open, hence the click.
    await user.click(roleSelect(rowFor("Nora None"), "nora none"));
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
  // not read as a blank "Set role…" — that would let an admin strip privileges
  // they were never shown.
  it("names the roles a multi-role member holds", async () => {
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

    // Both held roles are named, under their display labels, and the copy says
    // what picking one does.
    const shown = roleShown(rowFor("Nora None"), "nora none");
    expect(shown).toMatch(/holds/i);
    expect(shown).toContain("Team lead");
    expect(shown).toContain("Ops");
    expect(shown).toMatch(/replaces all of them/i);
  });

  it("sets a member's role through the role seam", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const active = rowFor("Ada Active");
    await pickOption(user, roleSelect(active, "ada active"), "Team lead");

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
    await pickOption(user, roleSelect(active, "ada active"), "User");
    await waitFor(() => expect(patches).toHaveLength(1));

    // The SAME target again — the retry the operator would make.
    await pickOption(user, roleSelect(active, "ada active"), "User");
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
    await pickOption(user, roleSelect(active, "ada active"), "Team lead");
    await waitFor(() => expect(rosterReads).toBe(2));

    // Mid-flight: the row reads the role being applied and stays locked. "Admin"
    // here would be the stale cache showing through.
    expect(roleShown(active, "ada active")).toBe("Team lead");
    expect(roleSelect(active, "ada active").disabled).toBe(true);

    releaseRoster();
    await waitFor(() =>
      expect(roleSelect(rowFor("Ada Active"), "ada active").disabled).toBe(
        false,
      ),
    );
    expect(roleShown(rowFor("Ada Active"), "ada active")).toBe("Team lead");
  });

  // A custom role reads under the name its maker gave it, and is offered like
  // any other. A role this reader may not hand out is a fact on the row rather
  // than a picker whose every choice the server refuses.
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

    expect(roleShown(rowFor("Ivy Invited"), "ivy invited")).toBe("Field sales");
    // Ada holds admin, which this reader may not hand out.
    expect(within(rowFor("Ada Active")).queryByRole("combobox")).toBeNull();
    expect(within(rowFor("Ada Active")).getByText("Admin")).toBeTruthy();

    await user.click(roleSelect(rowFor("Nora None"), "nora none"));
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toContain("Field sales");
    expect(offered).not.toContain("Admin");
  });
});
