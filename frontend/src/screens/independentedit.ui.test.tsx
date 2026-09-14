/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import {
  company,
  emptyPage,
  jsonResponse,
  stubFetch,
} from "./company.fixtures";
import { ContactDetails } from "./contactdetails";
import { DealScreen } from "./deals";

const anna: components["schemas"]["Contact"] = {
  id: "p-1",
  full_name: "Anna Weber",
  title: "Head of Procurement",
  version: 1,
  writable: true,
  emails: [
    {
      id: "e-1",
      email: "anna@example.test",
      is_primary: true,
      email_type: "work",
      source: "manual",
      captured_by: "human:u1",
      position: 0,
    },
  ],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};
function deal(
  overrides: Partial<components["schemas"]["Deal"]>,
): components["schemas"]["Deal"] {
  return {
    id: "x",
    version: 4,
    name: "Original deal",
    currency: "EUR",
    amount_minor: 10000,
    pipeline_id: "pl",
    stage_id: "s1",
    status: "open",
    writable: true,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}
function dealBackstop(request: Request) {
  if (request.url.endsWith("/me"))
    return jsonResponse(
      meFixture({ allow: { deal: ["read", "update"], company: ["read"] } }),
    );
  if (request.url.includes("/pipelines"))
    return jsonResponse({
      data: [
        {
          id: "pl",
          name: "Sales",
          stages: [
            { id: "s1", name: "Qualification", semantic: "open", position: 1 },
          ],
        },
      ],
    });
  if (request.url.includes("/context"))
    return jsonResponse({ anchor: { type: "deal", id: "x" }, sections: [] });
  return emptyPage();
}
function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("saves Customer tier when a background logo update changed the company version", async () => {
  const original = { ...company, cf_customer_tier: null };
  let current: Record<string, unknown> = original;
  const patches: { body: unknown; version: string | null }[] = [];
  stubFetch(async (url, method, request) => {
    if (url.includes("/custom-fields"))
      return jsonResponse({
        data: [
          {
            id: "tier",
            object: "company",
            label: "Customer tier",
            column_name: "cf_customer_tier",
            type: "picklist",
            status: "active",
            options: ["Strategic Client", "Growth Client"],
          },
        ],
      });
    if (method === "PATCH") {
      const body = await request.json();
      const version = request.headers.get("If-Match");
      patches.push({ body, version });
      if (version !== String(current.version))
        return jsonResponse({ code: "version_skew" }, 409);
      current = { ...current, ...body, version: 3 };
      return jsonResponse(current);
    }
    if (url.endsWith("/companies/o-1")) return jsonResponse(current);
    return emptyPage();
  });
  render(<CompanyScreen id="o-1" />);
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: "Change Customer tier" }),
  );
  current = { ...original, version: 2, logo_url: "new-logo" };
  await user.click(
    await screen.findByRole("option", { name: "Growth Client" }),
  );
  await waitFor(() => expect(patches).toHaveLength(2));
  expect(patches).toEqual([
    { body: { cf_customer_tier: "Growth Client" }, version: "1" },
    { body: { cf_customer_tier: "Growth Client" }, version: "2" },
  ]);
  expect(current).toMatchObject({
    logo_url: "new-logo",
    cf_customer_tier: "Growth Client",
  });
});

it("saves a contact's title across an unrelated update without resending their email list", async () => {
  let current = anna;
  const patches: { body: unknown; version: string | null }[] = [];
  stubFetch(async (url, method, request) => {
    if (method === "PATCH") {
      const body = await request.json();
      const version = request.headers.get("If-Match");
      patches.push({ body, version });
      if (version !== String(current.version))
        return jsonResponse({ code: "version_skew" }, 409);
      current = { ...current, ...body, version: 3 };
      return jsonResponse(current);
    }
    if (url.endsWith("/me"))
      return jsonResponse(
        meFixture({ allow: { contact: ["read", "update"] } }),
      );
    if (url.includes("/custom-fields") || url.includes("/activities"))
      return jsonResponse({ data: [] });
    return jsonResponse(current);
  });
  render(<ContactDetails contact={anna} />);
  fireEvent.click(await screen.findByRole("button", { name: "Change Title" }));
  fireEvent.change(await screen.findByLabelText("Title"), {
    target: { value: "New title" },
  });
  current = { ...anna, version: 2, full_name: "Updated name" };
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
  await waitFor(() => expect(patches).toHaveLength(2));
  expect(patches).toEqual([
    { body: { title: "New title" }, version: "1" },
    { body: { title: "New title" }, version: "2" },
  ]);
  expect(current).toMatchObject({
    full_name: "Updated name",
    title: "New title",
    emails: anna.emails,
  });
});

it("saves a deal's name across an unrelated update using the fresh version", async () => {
  const original = deal({ id: "x", version: 4, owner_id: null });
  let current = original;
  const patches: { body: unknown; version: string | null }[] = [];
  const fallback = dealBackstop;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "GET" && /\/deals\/x$/.test(request.url))
        return jsonResponse(current);
      if (request.method === "PATCH" && /\/deals\/x$/.test(request.url)) {
        const body = await request.json();
        const version = request.headers.get("If-Match");
        patches.push({ body, version });
        if (version !== String(current.version))
          return jsonResponse({ code: "version_skew" }, 409);
        current = { ...current, ...body, version: 6 };
        return jsonResponse(current);
      }
      return fallback(request);
    }),
  );
  render(<DealScreen id="x" />);
  fireEvent.click(
    await screen.findByRole("button", { name: "Change Deal name" }),
  );
  fireEvent.change(screen.getByLabelText("Deal name"), {
    target: { value: "Renamed deal" },
  });
  current = { ...original, version: 5, description: "Updated description" };
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
  await waitFor(() => expect(patches).toHaveLength(2));
  expect(patches).toEqual([
    { body: { name: "Renamed deal" }, version: "4" },
    { body: { name: "Renamed deal" }, version: "5" },
  ]);
  expect(current).toMatchObject({
    name: "Renamed deal",
    description: "Updated description",
  });
});
