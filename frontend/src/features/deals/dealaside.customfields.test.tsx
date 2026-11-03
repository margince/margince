/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import type {
  CustomField,
  ObjectCustomFields,
} from "../../components/customfields.form";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

// The catalog is mocked so the rail is tested against controlled fields (#9249).
const fields: CustomField[] = [
  {
    id: "cf-1",
    object: "deal",
    label: "Ticket URL",
    slug: "ticket_url",
    type: "text",
    status: "active",
    column_name: "cf_ticket_url",
    show_in_create: true,
    reportable: false,
    created_by: "u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];
vi.mock("../../components/customfields.form", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../components/customfields.form")>();
  return {
    ...actual,
    useObjectCustomFields: (): ObjectCustomFields => ({
      fields,
      loading: false,
      failed: false,
      retry: () => undefined,
      formFields: [],
      recordSlice: () => ({}),
      toBody: () => ({}),
      toPatch: () => ({}),
    }),
  };
});

const deal = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 1,
  pipeline_id: "p1",
  stage_id: "s1",
  currency: "EUR",
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  cf_ticket_url: "TCK-42",
} as components["schemas"]["Deal"];

afterEach(cleanup);

function show(current: components["schemas"]["Deal"]) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read"] }, { seat: "full" }),
  });
  render(
    <StoryProviders>
      <DealAside deal={current} stages={[]} />
    </StoryProviders>,
  );
}

it("shows a custom field with a value as a read row on the deal's rail", () => {
  show(deal);
  expect(screen.getByText("Ticket URL")).toBeTruthy();
  expect(screen.getByText("TCK-42")).toBeTruthy();
});

it("folds an unset custom field with the other empty fields", () => {
  show({ ...deal, cf_ticket_url: null } as components["schemas"]["Deal"]);
  expect(screen.queryByText("Ticket URL")).toBeNull();
});

it("draws a custom field that holds a web address as a link", () => {
  show({
    ...deal,
    cf_ticket_url: "https://example.com/t/42",
  } as components["schemas"]["Deal"]);
  expect(
    screen.getByRole("link", { name: /example\.com\/t\/42/ }),
  ).toBeTruthy();
});
