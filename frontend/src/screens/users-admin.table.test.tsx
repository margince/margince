/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { viewerZone } from "../format/timezone";
import { en } from "../i18n/en";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  ROSTER,
  render,
  roleSelect,
  rowFor,
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
      expect(within(rowFor("Ada Active")).getByText("Nordics")).toBeTruthy(),
    );
    expect(within(rowFor("Ada Active")).getByText("DACH Sales")).toBeTruthy();
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

  it("refuses every picker against one card sentence when the reader may not change roles", async () => {
    vi.stubGlobal(
      "fetch",
      backend([], {
        roles: ["custom"],
        allow: { user_admin: ["read", "create", "delete"] },
      }),
    );
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Nora None")).toBeTruthy());

    const sentence = screen.getByText(en["users.role.withheld"]);
    for (const name of ["Ada Active", "Nora None", "Ivy Invited"]) {
      const picker = roleSelect(rowFor(name), name);
      expect(picker.disabled).toBe(true);
      expect(picker.getAttribute("aria-describedby")).toBe(sentence.id);
    }
  });
});

describe("roleRefusal", () => {
  const member = (over: Partial<User>): User => ({
    ...ROSTER.data[0],
    timezone: viewerZone(),
    ...over,
  });
  const context = (roster: User[], meId?: string) => ({
    roster,
    roles: SEEDED_ASSIGNABLE_ROLES,
    canChangeRole: true,
    cardReasonId: "posture",
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
      text: "users.role.lastAdmin",
    });
    const self = member({ id: "me", roles: ["rep"], allowed_actions: [] });
    expect(roleRefusal(self, context([sole, self], "me"))).toEqual({
      text: "users.role.own",
    });
    const other = member({ id: "b", roles: ["rep"], allowed_actions: [] });
    expect(roleRefusal(other, context([sole, other], "me"))).toEqual({
      text: "users.role.outside",
    });
  });

  it("names the reader's own row even when its role is one they may not hand out", () => {
    const self = member({
      id: "me",
      roles: ["role_not_assignable"],
      allowed_actions: [],
    });
    expect(roleRefusal(self, context([self], "me"))).toEqual({
      text: "users.role.own",
    });
  });

  it("points at the card's sentence when the reader holds no role change", () => {
    const ada = member({ allowed_actions: ["change_role"] });
    expect(
      roleRefusal(ada, { ...context([ada]), canChangeRole: false }),
    ).toEqual({ id: "posture" });
  });
});
