/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { CF_OBJECTS } from "../../lib/customfields.logic";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { FieldChangeTrail } from "./customfields.trail";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type CustomField = components["schemas"]["CustomField"];

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

const budget = (object: CustomField["object"]): CustomField => ({
  id: `budget-${object}`,
  object,
  label: "Budget",
  slug: "budget",
  type: "number",
  status: object === "deal" ? "retired" : "active",
  column_name: "cf_budget",
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
});

const change = (
  object: CustomField["object"],
  hour: number,
  before: object,
  after: object,
): AuditLogEntry => ({
  id: `a-${object}`,
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Test User",
  action: "update",
  entity_type: "custom_field",
  entity_id: `budget-${object}`,
  occurred_at: `2026-06-22T${hour}:00:00Z`,
  before: { ...before },
  after: { ...after },
});

// Three edits that keep the label, one per record type, newest first.
const EDITS: AuditLogEntry[] = [
  change("deal", 16, { status: "active" }, { status: "retired" }),
  change("company", 15, { show_in_create: true }, { show_in_create: false }),
  change(
    "contact",
    14,
    { label_translations: {} },
    { label_translations: { de: "Etat" } },
  ),
];

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

function backend(
  allow: GrantSpec,
  catalogReads: string[],
  failCatalogs = { times: 0 },
  pagedObject?: CustomField["object"],
) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) return json(meFixture({ allow }));
    if (url.includes("/audit-log")) {
      return json({ data: EDITS, page: { next_cursor: null } });
    }
    const object = new URL(url).searchParams.get("object") ?? "";
    catalogReads.push(object);
    if (object === "company" && failCatalogs.times > 0) {
      failCatalogs.times -= 1;
      return new Response(JSON.stringify({ title: "Unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/problem+json" },
      });
    }
    const known = (["contact", "company", "deal"] as const).find(
      (one) => one === object,
    );
    // The paged record type holds its field on a second page only.
    if (known && known === pagedObject) {
      const cursor = new URL(url).searchParams.get("cursor");
      return json({
        data: cursor === "p2" ? [budget(known)] : [],
        page: { next_cursor: cursor === "p2" ? null : "p2", has_more: !cursor },
      });
    }
    return json({
      data: known ? [budget(known)] : [],
      page: { next_cursor: null },
    });
  });
}

const mount = () =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <FieldChangeTrail meUserId="u1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );

describe("FieldChangeTrail", () => {
  it("names the record type, the field and the change on an edit that kept the label", async () => {
    const catalogReads: string[] = [];
    vi.stubGlobal("fetch", backend({ audit_log: ["read"] }, catalogReads));
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByText(en["cf.audit.title"]));

    const rows = await screen.findAllByRole("listitem");
    expect(rows.map((row) => within(row).getByText(/ · /).textContent)).toEqual(
      ["Deal · Budget (retired)", "Company · Budget", "Contact · Budget"],
    );
    expect(within(rows[0] as HTMLElement).getByText("Archived")).toBeTruthy();
    expect(
      within(rows[1] as HTMLElement).getByText(en["cf.audit.hiddenFromCreate"]),
    ).toBeTruthy();
    expect(
      within(rows[2] as HTMLElement).getByText(en["cf.audit.translations"]),
    ).toBeTruthy();
    expect(screen.queryByText("Updated")).toBeNull();
  });

  it("reads the other record types' fields only once the trail is opened", async () => {
    const catalogReads: string[] = [];
    const fetch = backend({ audit_log: ["read"] }, catalogReads);
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    mount();

    const summary = await screen.findByText(en["cf.audit.title"]);
    await vi.waitFor(() =>
      expect(
        fetch.mock.calls.some(([input]) =>
          String(input instanceof Request ? input.url : input).includes(
            "/audit-log",
          ),
        ),
      ).toBe(true),
    );
    expect(catalogReads).toEqual([]);

    await user.click(summary);
    await screen.findAllByRole("listitem");
    expect(new Set(catalogReads)).toEqual(new Set(CF_OBJECTS));
  });

  it("says a field catalog it could not read failed, and reads it again on retry", async () => {
    const catalogReads: string[] = [];
    const failCatalogs = { times: 1 };
    vi.stubGlobal(
      "fetch",
      backend({ audit_log: ["read"] }, catalogReads, failCatalogs),
    );
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByText(en["cf.audit.title"]));
    const retry = await screen.findByRole("button", {
      name: en["state.retry"],
    });
    expect(screen.queryAllByRole("listitem")).toEqual([]);

    await user.click(retry);
    const rows = await screen.findAllByRole("listitem");
    expect(within(rows[1] as HTMLElement).getByText(/ · /).textContent).toBe(
      "Company · Budget",
    );
    expect(catalogReads.filter((one) => one === "company")).toHaveLength(2);
  });

  it("names a field its catalog lists on a later page", async () => {
    const catalogReads: string[] = [];
    vi.stubGlobal(
      "fetch",
      backend({ audit_log: ["read"] }, catalogReads, { times: 0 }, "company"),
    );
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByText(en["cf.audit.title"]));
    const rows = await screen.findAllByRole("listitem");
    expect(within(rows[1] as HTMLElement).getByText(/ · /).textContent).toBe(
      "Company · Budget",
    );
    expect(catalogReads.filter((one) => one === "company")).toHaveLength(2);
  });

  it("asks for nothing when the role may not read the trail", async () => {
    const catalogReads: string[] = [];
    const fetch = backend({}, catalogReads);
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByText(en["cf.audit.title"]));

    expect(await screen.findByText(/Hidden for your role/i)).toBeTruthy();
    expect(catalogReads).toEqual([]);
  });
});
