// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { Offer } from "./offers";
import { EditOfferHeaderModal } from "./offers.header";

// The offer editor names which legal entity issues the offer, once the
// installation trades as more than one.

const OFFER: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "A-1007",
  revision: 1,
  status: "draft",
  currency: "EUR",
  buyer_company_id: null,
  valid_until: "2026-08-01",
  intro_text: null,
  terms_text: null,
  net_minor: 0,
  tax_minor: 0,
  gross_minor: 0,
  template_id: null,
  legal_entity_id: "le-default",
  language: "de",
  line_items: [],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function entity(id: string, name: string, isDefault: boolean) {
  return {
    id,
    is_default: isDefault,
    display_name: name,
    address: {},
    member_count: 0,
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi(entities: object[]) {
  const writes: { url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method !== "GET") {
        writes.push({ url: request.url, body: await request.json() });
        return Response.json(OFFER);
      }
      if (request.url.endsWith("/legal-entities")) {
        return Response.json({ data: entities });
      }
      return Response.json({ data: [], page: { next_cursor: null } });
    }),
  );
  return writes;
}

function renderModal() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <EditOfferHeaderModal open onClose={vi.fn()} offer={OFFER} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("offers no entity to pick while the installation trades as one", async () => {
  stubApi([entity("le-default", "Acme", true)]);
  renderModal();
  await screen.findByRole("textbox", { name: "Valid until" });
  expect(
    screen.queryByRole("combobox", { name: en["offer.legalEntity"] }),
  ).toBeNull();
});

it("saves the entity the reader picked", async () => {
  const writes = stubApi([
    entity("le-default", "Acme", true),
    entity("le-vienna", "Acme Wien", false),
  ]);
  renderModal();
  const user = userEvent.setup();
  const picker = await screen.findByRole("combobox", {
    name: en["offer.legalEntity"],
  });
  expect(picker).toHaveTextContent("Acme");
  await pickOption(user, picker, "Acme Wien");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]?.body).toMatchObject({ legal_entity_id: "le-vienna" });
});

it("saves a changed language in lower case, and sends none when it is unchanged", async () => {
  const writes = stubApi([entity("le-default", "Acme", true)]);
  renderModal();
  const user = userEvent.setup();
  const language = await screen.findByRole("textbox", {
    name: en["offer.language"],
  });
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]?.body).not.toHaveProperty("language");

  await user.clear(language);
  await user.type(language, "EN");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]?.body).toMatchObject({ language: "en" });
});
