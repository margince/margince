/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { LegalEntitiesCard } from "./legalentities";
import { jsonResponse, render } from "../../testing/settings.testkit";

// Settings › Company › Legal entities: every seat reads the list, and only a
// seat holding legal_entity:create is offered a new one.

const ADMIN: GrantSpec = { legal_entity: ["read", "create", "update"] };
const READER: GrantSpec = { legal_entity: ["read"] };

function entity(id: string, extra: object = {}) {
  return {
    id,
    is_default: false,
    display_name: "",
    address: {},
    member_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const DEFAULT_ONLY = [
  entity("le-default", { is_default: true, display_name: "Acme" }),
];

type Call = { method: string; url: string; body: unknown };

function backend(allow: GrantSpec, rows: object[], calls: Call[] = []) {
  return vi.fn(async (request: Request) => {
    const raw = request.method === "GET" ? "" : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
    });
    if (request.url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method === "POST") {
      return jsonResponse(entity("le-new", JSON.parse(raw)), 201);
    }
    return jsonResponse({ data: rows });
  });
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("lists the default entity alone, named after the company and marked default", async () => {
  vi.stubGlobal("fetch", backend(READER, DEFAULT_ONLY));
  render(<LegalEntitiesCard />);
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(within(row).getByText("Acme")).toBeInTheDocument();
  expect(
    within(row).getByText(en["legalEntities.defaultBadge"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: en["legalEntities.addOpen"] }),
  ).toBeNull();
});

it("names the default entity of an undescribed company in words", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [entity("le-default", { is_default: true })]),
  );
  render(<LegalEntitiesCard />);
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(
    within(row).getByText(en["legalEntities.defaultUnnamed"]),
  ).toBeInTheDocument();
});

it("describes a further entity with its name, legal name and country", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY, calls));
  render(<LegalEntitiesCard />);
  await user.click(
    await screen.findByRole("button", { name: en["legalEntities.addOpen"] }),
  );
  const dialog = await screen.findByRole("dialog");
  const [name, legal, country] = within(dialog).getAllByRole("textbox");
  await user.type(name as HTMLElement, "Wien");
  await user.type(legal as HTMLElement, "Wien GmbH");
  await user.type(country as HTMLElement, "at");
  await user.click(
    within(dialog).getByRole("button", {
      name: en["legalEntities.addConfirm"],
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    calls.some(
      (call) =>
        call.method === "POST" &&
        call.url.endsWith("/legal-entities") &&
        JSON.stringify(call.body) ===
          JSON.stringify({
            display_name: "Wien",
            legal_name: "Wien GmbH",
            country: "at",
          }),
    ),
  ).toBe(true);
});
