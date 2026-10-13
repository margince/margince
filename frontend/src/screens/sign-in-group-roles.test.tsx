/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { GroupRoleGrants } from "./sign-in-group-roles";

// Which directory groups hand which role to the members invited from them. The
// map is saved whole, and a new Admin grant asks for the typed phrase first.

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

function mount(initial: Record<string, string>, canManage = true) {
  const calls: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "PATCH") {
        calls.push(JSON.parse(await request.text()));
        return new Response(null, { status: 204 });
      }
      return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(<GroupRoleGrants initial={initial} canManage={canManage} />, {
    wrapper: Wrap,
  });
  return { calls };
}

// The editor sits behind a closed disclosure; a test opens it as a reader does.
async function open(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByText("Advanced: roles from directory groups"));
}

async function pickRole(
  user: ReturnType<typeof userEvent.setup>,
  picker: HTMLElement,
  role: string,
) {
  await user.click(picker);
  await user.click(await screen.findByRole("option", { name: role }));
}

describe("the group roles editor", () => {
  // Few installations map any group, so the section stays closed and its
  // summary says how many are mapped.
  it("stays closed by default and counts the mapped groups", () => {
    mount({ sales: "rep", engineering: "manager" });
    const details = screen
      .getByText("Advanced: roles from directory groups")
      .closest("details");
    expect(details?.open).toBe(false);
    expect(screen.getByText("2 groups mapped")).toBeTruthy();
  });

  it("says None mapped when no group is mapped", () => {
    mount({});
    expect(screen.getByText("None mapped")).toBeTruthy();
  });

  it("renders each stored mapping with its group and role", async () => {
    const user = userEvent.setup();
    mount({ engineering: "manager" });
    await open(user);
    expect(screen.getByDisplayValue("engineering")).toBeTruthy();
    const role = screen.getByRole("combobox", {
      name: "Role for the engineering group",
    });
    expect(role.textContent).toContain("Team lead");
  });

  // The whole map travels: the setting replaces rather than merges, so the
  // stored mapping rides along with the added one.
  it("adds a mapping and saves the whole map", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ sales: "rep" });
    await open(user);
    await user.click(screen.getByRole("button", { name: "Add group" }));
    const groups = screen.getAllByRole("textbox", { name: "Directory group" });
    const added = groups[groups.length - 1];
    if (!added) {
      throw new Error("the added row rendered no group input");
    }
    await user.type(added, "platform");
    await pickRole(
      user,
      screen.getByRole("combobox", { name: "Role for the platform group" }),
      "Ops",
    );
    await user.click(screen.getByRole("button", { name: "Save group roles" }));
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { sales: "rep", platform: "ops" } },
      ]),
    );
  });

  it("removes a mapping and saves the map without it", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ sales: "rep", engineering: "manager" });
    await open(user);
    await user.click(
      screen.getByRole("button", { name: "Remove the sales mapping" }),
    );
    await user.click(screen.getByRole("button", { name: "Save group roles" }));
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { engineering: "manager" } },
      ]),
    );
  });

  // Mirrors the server's refusal so it reaches the admin before a request does.
  it("refuses a blank group name before any request is made", async () => {
    const user = userEvent.setup();
    const { calls } = mount({});
    await open(user);
    expect(screen.getByText(/no groups are mapped/i)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Add group" }));
    const save = screen.getByRole("button", { name: "Save group roles" });
    expect(save).toHaveProperty("disabled", true);
    expect(screen.getByText("Every mapping needs a group name.")).toBeTruthy();
    await user.click(save);
    expect(calls).toEqual([]);
  });

  // A group mapped to Admin hands the highest role to everyone the directory
  // puts in it, so a NEW one asks for the typed phrase, naming only that group.
  it("asks for the typed phrase before saving a new Admin mapping", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ admins: "admin", sales: "rep" });
    await open(user);
    await pickRole(
      user,
      screen.getByRole("combobox", { name: "Role for the sales group" }),
      "Admin",
    );
    await user.click(screen.getByRole("button", { name: "Save group roles" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Make every member of sales an admin?",
    });
    const confirm = within(dialog).getByRole("button", {
      name: "Save and grant admin",
    });
    expect(confirm).toHaveProperty("disabled", true);
    expect(calls).toEqual([]);

    await user.type(
      within(dialog).getByRole("textbox"),
      "I am aware of the risk",
    );
    await user.click(confirm);
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { admins: "admin", sales: "admin" } },
      ]),
    );
  });

  // Re-saving an Admin mapping the map already held changes nobody's role.
  it("saves directly when the draft adds no Admin mapping", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ admins: "admin" });
    await open(user);
    await user.click(screen.getByRole("button", { name: "Add group" }));
    const groups = screen.getAllByRole("textbox", { name: "Directory group" });
    const added = groups[groups.length - 1];
    if (!added) {
      throw new Error("the added row rendered no group input");
    }
    await user.type(added, "support");
    await user.click(screen.getByRole("button", { name: "Save group roles" }));
    await waitFor(() =>
      expect(calls).toEqual([
        { oidc_group_role_map: { admins: "admin", support: "read_only" } },
      ]),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("carries the grant-only warning where the map is edited", async () => {
    const user = userEvent.setup();
    mount({});
    await open(user);
    expect(screen.getByText("Groups only ever add a role")).toBeTruthy();
    expect(screen.getByText(/does not take the role away here/)).toBeTruthy();
  });

  // Writing the map grants roles, so it is admin-only: a reader without that
  // grant sees the map read-only, with the reason on Save.
  it("shows the map read-only, with the reason, to a reader who cannot change it", async () => {
    const user = userEvent.setup();
    mount({ sales: "rep" }, false);
    await open(user);
    expect(
      screen.getByRole("textbox", { name: "Directory group" }),
    ).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "Add group" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(
      screen.getByRole("button", { name: "Save group roles" }),
    ).toHaveProperty("disabled", true);
    expect(
      screen.getByText("Only admins can change these mappings"),
    ).toBeTruthy();
  });
});
