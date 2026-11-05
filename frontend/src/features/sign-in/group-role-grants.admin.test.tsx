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
import { LocaleProvider } from "../../i18n";
import { SEEDED_ASSIGNABLE_ROLES } from "../../testing/roles.testkit";
import { GroupRoleGrants } from "./group-role-grants";

// A new Admin grant asks for the typed phrase before the map is saved whole.

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

function mount(initial: Record<string, string>) {
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
  render(
    <GroupRoleGrants initial={initial} canManage lock={{ disabled: false }} />,
    { wrapper: Wrap },
  );
  return { calls };
}

async function pickRole(
  user: ReturnType<typeof userEvent.setup>,
  picker: HTMLElement,
  role: string,
) {
  await user.click(picker);
  await user.click(await screen.findByRole("option", { name: role }));
}

describe("the group roles admin confirmation", () => {
  // A group mapped to Admin hands the highest role to everyone the directory
  // puts in it. So a new one asks for the typed phrase, naming only that group.
  it("asks for the typed phrase before saving a new Admin mapping", async () => {
    const user = userEvent.setup();
    const { calls } = mount({ admins: "admin", sales: "rep" });
    await pickRole(
      user,
      await screen.findByRole("combobox", { name: "Role for the sales group" }),
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
    await user.click(await screen.findByRole("button", { name: "Add group" }));
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
});
