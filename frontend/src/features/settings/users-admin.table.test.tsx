/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { browserZone } from "../../format/timezone";
import { en } from "../../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  jsonResponse,
  ROSTER,
  render,
  roleControl,
  roleShown,
  rowFor,
  rowMenu,
} from "./users-admin.testkit";
import type { User } from "./users-members";
import { roleRefusal } from "./users-rolecell";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const columnHeaders = () =>
  screen.getAllByRole("columnheader").map((cell) => cell.textContent);

describe("the members table", () => {
  it("names each member's teams from the team roster", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() =>
      expect(within(rowFor("Ada Active")).getByText(/Nordics/)).toBeTruthy(),
    );
    expect(within(rowFor("Ada Active")).getByText(/DACH Sales/)).toBeTruthy();
  });

  it("reads last activity against the clock and claims nothing for a missing one", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-08T12:00:00Z"));
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    const seen = within(rowFor("Ada Active")).getByText("3 hours ago");
    const added = within(rowFor("Ada Active")).getByText(/^Added /);
    expect(added.getAttribute("dateTime")).toBe("2026-01-15T10:00:00Z");
    expect(seen.getAttribute("dateTime")).toBe("2026-10-08T09:00:00Z");
    expect(seen.getAttribute("title")).toBeTruthy();
    // Null is "never signed in" or "withheld from this reader"; the cell says neither.
    const otto = rowFor("Otto Off");
    expect(within(otto).getByText(en["users.lastActiveUnknown"])).toBeTruthy();
    expect(within(otto).queryByText(/never/i)).toBeNull();
  });

  it("marks only the members who are not active, beside their name", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    expect(within(rowFor("Ada Active")).queryByText("Active")).toBeNull();
    expect(within(rowFor("Otto Off")).getByText("Deactivated")).toBeTruthy();
    expect(within(rowFor("Ivy Invited")).getByText("Invited")).toBeTruthy();
  });

  it("draws the administration columns only for a reader the roster serves them to", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(columnHeaders()).toEqual([
      "Name",
      "Role",
      "Teams",
      "Activity",
      "Actions",
    ]);
    cleanup();

    vi.stubGlobal("fetch", backend([], { roles: ["rep"] }));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(columnHeaders()).toEqual(["Name", "Activity"]);
    // Last activity is withheld from this reader, so the cell states only when
    // the member was added.
    expect(
      within(rowFor("Ada Active")).queryByText(en["users.lastActiveUnknown"]),
    ).toBeNull();
    expect(within(rowFor("Ada Active")).getByText(/^Added /)).toBeTruthy();
  });

  it("states roles as words under one card sentence when the reader may not change them", async () => {
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["read", "create", "delete"] },
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    expect(screen.getByText(en["users.role.withheld"])).toBeTruthy();
    for (const name of ["Ada Active", "Nora None", "Ivy Invited"]) {
      expect(roleControl(rowFor(name), name)).toBeNull();
    }
    expect(roleShown(rowFor("Ada Active"))).toBe("Admin");
  });

  // Deactivate is a verb on this page, so the page carries no read-only notice
  // and the card says which half is withheld.
  it("tells a seat that may only deactivate that roles are not theirs to change", async () => {
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["read", "delete"] },
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());
    expect(screen.getByText(en["users.role.withheld"])).toBeTruthy();
  });
});

describe("roleRefusal", () => {
  const member = (over: Partial<User>): User => ({
    ...ROSTER.data[0],
    timezone: browserZone(),
    ...over,
  });
  const context = (roster: User[], meId?: string) => ({
    roster,
    roles: SEEDED_ASSIGNABLE_ROLES,
    canChangeRole: true,
    meId,
    t: (key: string) => key,
  });

  it("offers the picker where the server lists the change", () => {
    const ada = member({ allowed_actions: ["change_role"] });
    expect(roleRefusal(ada, context([ada]))).toBeNull();
  });

  it("names the last admin, the reader's own row, and a member out of reach", () => {
    const sole = member({ id: "a", allowed_actions: [] });
    expect(roleRefusal(sole, context([sole]))).toEqual({
      reason: "users.role.lastAdmin",
    });
    const self = member({ id: "me", roles: ["rep"], allowed_actions: [] });
    expect(roleRefusal(self, context([sole, self], "me"))).toEqual({
      reason: "users.role.own",
    });
    const other = member({ id: "b", roles: ["rep"], allowed_actions: [] });
    expect(roleRefusal(other, context([sole, other], "me"))).toEqual({
      reason: "users.role.outside",
    });
  });

  it("names the reader's own row even when its role is one they may not hand out", () => {
    const self = member({
      id: "me",
      roles: ["role_not_assignable"],
      allowed_actions: [],
    });
    expect(roleRefusal(self, context([self], "me"))).toEqual({
      reason: "users.role.own",
    });
  });

  it("leaves the reason to the card's sentence when the reader holds no role change", () => {
    const ada = member({ allowed_actions: ["change_role"] });
    expect(
      roleRefusal(ada, { ...context([ada]), canChangeRole: false }),
    ).toEqual({});
  });
});

describe("a merge the server refuses", () => {
  const used: Omit<User, "timezone"> = {
    ...ROSTER.data[0],
    id: "u-used",
    email: "uma@acme.test",
    display_name: "Uma Used",
    allowed_actions: ["deactivate"],
    unavailable_actions: [{ action: "merge", reason: "password_set" }],
  };

  function rosterWith(rows: Omit<User, "timezone">[]) {
    const rest = backend([]);
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (
        new URL(req.url).pathname.endsWith("/v1/users") &&
        req.method === "GET"
      ) {
        return jsonResponse({ ...ROSTER, data: rows });
      }
      return rest(input, init);
    });
  }

  it("stays in the menu, disabled, with the reason and the way out", async () => {
    vi.stubGlobal("fetch", rosterWith([used]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Uma Used")).toBeTruthy());

    const verbs = await rowMenu(userEvent.setup(), "Uma Used");
    const merge = verbs.getByRole("button", { name: en["users.merge"] });
    expect(merge.hasAttribute("disabled")).toBe(true);
    const reason = document.getElementById(
      merge.getAttribute("aria-describedby") ?? "",
    );
    expect(reason?.textContent).toContain(
      en["users.mergeRefused.password_set"],
    );
    expect(reason?.textContent).toContain("hand over Uma Used");
  });

  it("offers a working Merge, and no reason, where the server offers it", async () => {
    const unused = {
      ...used,
      id: "u-unused",
      display_name: "Una Unused",
      allowed_actions: ["merge"] as User["allowed_actions"],
      unavailable_actions: [],
    };
    vi.stubGlobal("fetch", rosterWith([unused]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Una Unused")).toBeTruthy());

    const verbs = await rowMenu(userEvent.setup(), "Una Unused");
    const merge = verbs.getByRole("button", { name: en["users.merge"] });
    expect(merge.hasAttribute("disabled")).toBe(false);
    expect(merge.hasAttribute("aria-describedby")).toBe(false);
  });
});
