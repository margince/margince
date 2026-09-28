/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { RolesSettings } from "./roles-settings";

// The role editor over a stubbed transport: the list and its archived toggle,
// a new role made by copying one, a grant flip carrying If-Match, the server's
// refusals in the catalog's words, and the access preview of the open role.

type Role = components["schemas"]["Role"];

const GRANT = { read: true, create: true, update: true, delete: true };
const NONE = { read: false, create: false, update: false, delete: false };

const ADMIN: Role = {
  key: "admin",
  name: "Admin",
  is_system: true,
  version: 4,
  row_scope: "all",
  objects: { contact: GRANT, deal: GRANT },
};
const FIELD: Role = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
  version: 7,
  row_scope: "own",
  objects: { contact: { ...NONE, read: true }, deal: NONE },
};
const OLD: Role = {
  key: "custom_old",
  name: "Old role",
  is_system: false,
  version: 2,
  row_scope: "team",
  archived_at: "2026-09-01T09:00:00Z",
  objects: { contact: NONE, deal: NONE },
};

type Call = { method: string; url: string; body?: unknown; ifMatch?: string };

const ROLE_ADMIN: GrantSpec = {
  role_admin: ["read", "create", "update", "delete"],
};

function backend(
  calls: Call[],
  opts: {
    allow?: GrantSpec;
    roles?: string[];
    refuse?: { status: number; code: string };
  } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url);
    if (url.pathname.endsWith("/v1/me")) {
      return json(
        meFixture({
          roles: opts.roles ?? ["admin"],
          allow: opts.allow ?? ROLE_ADMIN,
        }),
      );
    }
    if (url.pathname.endsWith("/v1/roles") && req.method === "GET") {
      const withArchived = url.searchParams.get("include_archived") === "true";
      return json({
        roles: withArchived ? [ADMIN, FIELD, OLD] : [ADMIN, FIELD],
      });
    }
    if (url.pathname.endsWith("/v1/users/access-preview")) {
      return json({
        role: url.searchParams.get("role"),
        row_scope: "own",
        objects: { contact: { ...NONE, read: true } },
        field_masks: [],
        teams: [],
      });
    }
    const body = await req
      .clone()
      .json()
      .catch(() => undefined);
    calls.push({
      method: req.method,
      url: url.pathname,
      body,
      ifMatch: req.headers.get("If-Match") ?? undefined,
    });
    if (opts.refuse) {
      return json(
        { title: "Refused", detail: "server words", code: opts.refuse.code },
        opts.refuse.status,
      );
    }
    if (req.method === "POST" && url.pathname.endsWith("/v1/roles")) {
      return json({ ...FIELD, key: "custom_closers", name: "Closers" }, 201);
    }
    return json({ ...FIELD, version: FIELD.version + 1 });
  });
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

async function openRole(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(await screen.findByRole("button", { name: `Open ${name}` }));
  return screen.findByRole("heading", { name });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("RolesSettings", () => {
  it("lists the live roles, and the archived ones only when asked", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<RolesSettings />);

    await screen.findByText("Field sales");
    expect(screen.getByText("Admin")).toBeTruthy();
    expect(screen.queryByText("Old role")).toBeNull();

    await user.click(screen.getByRole("switch", { name: /show archived/i }));
    const old = await screen.findByTestId("role-custom_old");
    expect(within(old).getByText(en["roles.archived"])).toBeTruthy();
  });

  it("makes a new role as a copy of the one picked, and opens it", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<RolesSettings />);

    await user.click(await screen.findByRole("button", { name: "New role" }));
    const dialog = within(await screen.findByRole("dialog"));
    await pickOption(
      user,
      dialog.getByRole("combobox", { name: /copy rights from/i }),
      "Field sales",
    );
    await user.type(dialog.getByRole("textbox", { name: /name/i }), "Closers");
    await user.click(dialog.getByRole("button", { name: "Create role" }));

    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: "POST",
          url: "/v1/roles",
          body: { copy_from: "custom_field_sales", name: "Closers" },
        }),
      ),
    );
  });

  it("offers no new role to a reader who is not an admin", async () => {
    vi.stubGlobal("fetch", backend([], { roles: ["ops"] }));
    render(<RolesSettings />);
    await screen.findByText("Field sales");
    expect(screen.queryByRole("button", { name: "New role" })).toBeNull();
  });

  it("writes a flipped grant with the version the reader saw", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<RolesSettings />);
    await openRole(user, "Field sales");

    await user.click(
      screen.getByRole("switch", { name: /allow field sales to create deal/i }),
    );
    await waitFor(() =>
      expect(calls).toContainEqual({
        method: "PATCH",
        url: "/v1/roles/custom_field_sales/objects/deal",
        body: { ...NONE, create: true },
        ifMatch: "7",
      }),
    );
  });

  it("moves the row scope with If-Match", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<RolesSettings />);
    await openRole(user, "Field sales");

    await user.click(screen.getByRole("radio", { name: /their teams/i }));
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: "PATCH",
          url: "/v1/roles/custom_field_sales",
          body: { row_scope: "team" },
          ifMatch: "7",
        }),
      ),
    );
  });

  it.each([
    [403, "widening_requires_admin", "roles.refusal.widening"],
    [409, "version_skew", "roles.refusal.versionSkew"],
    [409, "role_in_use", "roles.refusal.inUse"],
    [409, "archived_role_held", "roles.refusal.archivedHeld"],
    [409, "system_role", "roles.refusal.system"],
  ] as const)(
    "says a %i %s refusal in the catalog's words",
    async (status, code, key) => {
      const user = userEvent.setup();
      vi.stubGlobal("fetch", backend([], { refuse: { status, code } }));
      render(<RolesSettings />);
      await openRole(user, "Field sales");

      await user.click(
        screen.getByRole("switch", { name: /allow field sales to read deal/i }),
      );
      expect(await screen.findByText(en[key])).toBeTruthy();
      expect(screen.queryByText("server words")).toBeNull();
    },
  );

  it("archives a custom role and offers no archive on a built-in one", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<RolesSettings />);

    await openRole(user, "Admin");
    expect(screen.queryByRole("button", { name: "Archive role" })).toBeNull();

    await openRole(user, "Field sales");
    await user.click(screen.getByRole("button", { name: "Archive role" }));
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          method: "POST",
          url: "/v1/roles/custom_field_sales/archive",
        }),
      ),
    );
  });

  it("previews what the open role sees, from the server", async () => {
    const user = userEvent.setup();
    const fetch = backend([]);
    vi.stubGlobal("fetch", fetch);
    render(<RolesSettings />);
    await openRole(user, "Field sales");

    await user.click(screen.getByText(en["roles.preview"]));
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(([input]) =>
          String(input instanceof Request ? input.url : input).includes(
            "/users/access-preview?role=custom_field_sales",
          ),
        ),
      ).toBe(true),
    );
    expect(await screen.findByText(/contacts: read/i)).toBeTruthy();
  });

  it("disables every switch for a reader who may only read roles", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backend([], { roles: ["ops"], allow: { role_admin: ["read"] } }),
    );
    render(<RolesSettings />);
    await openRole(user, "Field sales");

    const switches = screen.getAllByRole("switch", {
      name: /allow field sales/i,
    });
    expect(switches.length).toBeGreaterThan(0);
    for (const control of switches) {
      expect(control.hasAttribute("disabled")).toBe(true);
    }
    expect(screen.queryByRole("button", { name: "Rename" })).toBeNull();
  });
});
