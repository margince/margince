/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { QualifyDialog } from "./leads.qualify";
import { jsonResponse, lead, render, stubFetch } from "./leads.testkit";

// The deal a German reader opens while qualifying reads "12,5" as twelve and a
// half, and refuses "1.500", which reads as fifteen hundred to them.

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

const pipeline = {
  id: "pl",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s1",
      pipeline_id: "pl",
      name: "Qualify",
      position: 1,
      semantic: "open",
      win_probability: 20,
    },
  ],
};

async function qualifyWithAmount(amount: string) {
  let body: unknown;
  stubFetch(async (url, method, request) => {
    if (url.includes("/promote-preview")) {
      return jsonResponse({
        outcome: "create",
        company_id: "company-one",
        existing_deals: [],
      });
    }
    if (method === "POST" && url.includes("/promote")) {
      body = JSON.parse(await request.text());
      return jsonResponse({ contact: { id: "contact-one" }, merged: false });
    }
    if (url.includes("/installation/settings")) {
      return jsonResponse({ base_currency: "EUR", company_name: "Acme" });
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: [pipeline], page: { next_cursor: null } });
    }
    return jsonResponse({ data: [] });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QualifyDialog lead={lead} open onClose={vi.fn()} onQualified={vi.fn()} />,
    client,
    "de",
  );
  const user = userEvent.setup();
  const newDeal = await screen.findByRole("radio", {
    name: "Neuen Deal eröffnen",
  });
  await user.click(newDeal);
  await user.type(
    await screen.findByRole("textbox", { name: /^Betrag \(EUR\)/ }),
    amount,
  );
  return { user, body: () => body };
}

it("sends 12,5 as 1250 minor units", async () => {
  const f = await qualifyWithAmount("12,5");
  await f.user.click(
    screen.getByRole("button", { name: "Qualifizieren und Deal erstellen" }),
  );
  await waitFor(() =>
    expect(f.body()).toMatchObject({
      deal: { amount_minor: 1250, currency: "EUR" },
    }),
  );
});

it("refuses 1.500 by naming the wanted form, and sends nothing", async () => {
  const f = await qualifyWithAmount("1.500");
  expect(
    await screen.findByText("Gib den Betrag als 1500 oder 1500,00 ein."),
  ).toBeTruthy();
  const confirm = screen.getByRole("button", {
    name: "Qualifizieren und Deal erstellen",
  });
  await f.user.click(confirm);
  expect(f.body()).toBeUndefined();
});
