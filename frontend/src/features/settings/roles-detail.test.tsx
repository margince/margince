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
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { ExtensionUnit } from "../../lib/extensions.queries";
import type { Role } from "../../lib/roles.queries";
import { RoleDetail, useRoleWrites } from "./roles-detail";

const NONE = { read: false, create: false, update: false, delete: false };
const READ = { ...NONE, read: true };

const NOTES: ExtensionUnit = {
  name: "notes",
  description: "Notes on records.",
  version: "1.0.0",
  status: "ready",
  rbac_objects: ["ext_notes_note", "ext_notes_signing_key"],
  routes: [],
  jobs: [],
};

// `pricing_rule` stands for an object a newer server added, and
// `ext_gone_thing` for one a removed unit left behind.
const FIELD: Role = {
  key: "custom_field_sales",
  name: "Field sales",
  is_system: false,
  version: 7,
  row_scope: "team",
  objects: {
    deal: READ,
    contact: NONE,
    audit_log: NONE,
    ext_notes_note: READ,
    pricing_rule: NONE,
    ext_gone_thing: READ,
  },
};

type Seat = Readonly<{ role?: Role; canUpdate?: boolean }>;

function Body({
  units,
  role = FIELD,
  canUpdate = true,
}: Readonly<{ units: readonly ExtensionUnit[] }> & Seat) {
  const writes = useRoleWrites();
  return (
    <RoleDetail
      role={role}
      directory={[role]}
      units={units}
      canUpdate={canUpdate}
      canWiden
      writes={writes}
    />
  );
}

function render(units: readonly ExtensionUnit[] = [NOTES], seat: Seat = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Body units={units} {...seat} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// What a screen reader hears after a control's name.
function description(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}

// The names of the rows under each run, in the order the grid draws them.
function runs(): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const body of screen.getByRole("table").querySelectorAll("tbody")) {
    const [head, ...rows] = within(body).getAllByRole("rowheader");
    out[head?.textContent ?? ""] = rows.map((row) => row.textContent ?? "");
  }
  return out;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("RoleDetail permission grid", () => {
  it("draws each object by its name, under its area, in one table", () => {
    render();
    expect(screen.getAllByRole("table")).toHaveLength(1);
    expect(runs()).toEqual({
      Records: ["Contacts", "Deals"],
      Governance: ["Audit log"],
      "notes extension": ["Note", "Signing key"],
      Other: ["ext_gone_thing", "pricing_rule"],
    });
  });

  it("names every switch by the object's label", () => {
    render();
    expect(
      screen.getByRole("switch", { name: "Allow Field sales to Read Deals" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("switch", {
        name: "Allow Field sales to Delete Signing key",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("switch", {
        name: "Allow Field sales to Update pricing_rule",
      }),
    ).toBeTruthy();
  });

  it("leaves out an extension run when no unit is composed", () => {
    render([]);
    expect(Object.keys(runs())).toEqual(["Records", "Governance", "Other"]);
    expect(runs().Other).toEqual([
      "ext_gone_thing",
      "ext_notes_note",
      "pricing_rule",
    ]);
  });

  it("shows each raw key under its name only once asked", async () => {
    const user = userEvent.setup();
    render();
    const deals = screen.getByRole("rowheader", { name: "Deals" });
    expect(deals.querySelector("code")).toBeNull();

    await user.click(
      screen.getByRole("switch", { name: en["roles.showKeys"] }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("rowheader", { name: /Deals/ }).textContent,
      ).toContain("deal"),
    );
    expect(
      screen.getByRole("rowheader", { name: /Signing key/ }).textContent,
    ).toContain("ext_notes_signing_key");
  });

  it("writes the object's key, not its label", async () => {
    const user = userEvent.setup();
    const calls: { url: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        calls.push({ url: new URL(req.url).pathname, body: await req.json() });
        return new Response(JSON.stringify({ ...FIELD, version: 8 }), {
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render();
    await user.click(
      screen.getByRole("switch", { name: "Allow Field sales to Create Deals" }),
    );
    await waitFor(() =>
      expect(calls).toContainEqual({
        url: "/v1/roles/custom_field_sales/objects/deal",
        body: { ...READ, create: true },
      }),
    );
  });

  it("says once above the grid why a reader who may only read changes nothing, and on every switch", () => {
    render([NOTES], { canUpdate: false });
    const said = screen
      .getAllByText(en["roles.readOnly"])
      .filter((line) => !line.closest(".grant-cell"));
    expect(said).toHaveLength(1);
    const switches = screen.getAllByRole("switch", { name: /^Allow / });
    expect(switches.length).toBeGreaterThan(0);
    for (const control of switches) {
      expect(description(control)).toContain(en["roles.readOnly"]);
    }
  });

  it("gives an archived role's switches the archive as their reason, not the reader's role", () => {
    render([NOTES], {
      role: { ...FIELD, archived_at: "2026-09-01T09:00:00Z" },
    });
    expect(screen.queryByText(en["roles.readOnly"])).toBeNull();
    for (const control of screen.getAllByRole("switch", { name: /^Allow / })) {
      expect(description(control)).toContain(en["roles.archivedReadOnly"]);
    }
  });
});
