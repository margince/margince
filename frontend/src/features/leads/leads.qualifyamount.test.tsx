/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { steppedClock } from "../../testing/steppedclock";
import { QualifyDialog } from "./leads.qualify";
import { jsonResponse, lead, render, stubFetch } from "./leads.testkit";

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  Reflect.deleteProperty(globalThis, "jest");
  vi.unstubAllGlobals();
  localStorage.clear();
});

// The dialog opened on "Open a new deal" in an installation keeping currency.
// refusal, when given, is the server's answer to the promotion.
async function newDealDialog(
  refusal?: unknown,
  currency = "EUR",
  locale: "en" | "de" = "en",
) {
  const user = steppedClock();
  vi.setSystemTime(new Date("2026-10-05T12:00:00Z"));
  const posted: unknown[] = [];
  stubFetch(async (url, method, request) => {
    if (url.includes("/promote-preview"))
      return jsonResponse({ outcome: "create", existing_deals: [] });
    if (method === "POST" && url.includes("/promote")) {
      posted.push(JSON.parse(await request.text()));
      return refusal
        ? jsonResponse(refusal, 422)
        : jsonResponse({ contact: { id: "contact-one" }, merged: false });
    }
    if (url.includes("/installation/settings"))
      return jsonResponse({ base_currency: currency });
    return jsonResponse({ data: [] });
  });
  render(
    <QualifyDialog lead={lead} open onClose={vi.fn()} onQualified={vi.fn()} />,
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    locale,
  );
  const de = locale === "de";
  await user.click(
    await screen.findByRole("radio", {
      name: de ? "Neuen Deal eröffnen" : "Open a new deal",
    }),
  );
  const amount = await screen.findByLabelText(
    `${de ? "Betrag" : "Amount"} (${currency})`,
  );
  return { user, amount, posted };
}

it("sends a decimal amount in minor units with its currency", async () => {
  const f = await newDealDialog();
  await f.user.type(f.amount, "1.5");
  await f.user.click(
    screen.getByRole("button", { name: "Qualify and open deal" }),
  );
  await waitFor(() => expect(f.posted).toHaveLength(1));
  expect(f.posted[0]).toMatchObject({
    deal: { amount_minor: 150, currency: "EUR" },
  });
});

it("refuses a negative amount beside the field and sends nothing", async () => {
  const f = await newDealDialog();
  await f.user.type(f.amount, "-5");
  expect(
    await screen.findByText("Enter an amount of zero or more."),
  ).toBeVisible();
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  expect(confirm).toBeDisabled();
  await f.user.click(confirm);
  expect(f.posted).toHaveLength(0);
});

it("refuses a figure finer than the currency beside the field and sends nothing", async () => {
  const f = await newDealDialog(undefined, "VND");
  await f.user.type(f.amount, "1.5");
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  await f.user.click(confirm);
  expect(
    screen.getByText("This amount has more decimals than VND allows."),
  ).toBeVisible();
  expect(f.amount).toHaveValue("1.5");
  expect(confirm).toBeDisabled();
  expect(f.posted).toHaveLength(0);

  await f.user.clear(f.amount);
  await f.user.type(f.amount, "2");
  await f.user.click(confirm);
  await waitFor(() => expect(f.posted).toHaveLength(1));
  expect(f.posted[0]).toMatchObject({
    deal: { amount_minor: 2, currency: "VND" },
  });
});

// Leaving "Open a new deal" drops the refused text with its editor. The
// refusal goes with it, and the amount the editor comes back on can be sent.
it("drops a precision refusal when the deal choice takes its editor away", async () => {
  const f = await newDealDialog(undefined, "VND");
  await f.user.type(f.amount, "1");
  await f.user.tab();
  await f.user.type(await screen.findByLabelText("Amount (VND)"), ".5");
  await f.user.click(
    screen.getByRole("button", { name: "Qualify and open deal" }),
  );
  expect(
    screen.getByText("This amount has more decimals than VND allows."),
  ).toBeVisible();

  await f.user.click(
    screen.getByRole("radio", { name: "Qualify without a deal" }),
  );
  await f.user.click(screen.getByRole("radio", { name: "Open a new deal" }));
  expect(
    screen.queryByText("This amount has more decimals than VND allows."),
  ).toBeNull();
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  expect(confirm).toBeEnabled();
  await f.user.click(confirm);
  await waitFor(() => expect(f.posted).toHaveLength(1));
  expect(f.posted[0]).toMatchObject({
    deal: { amount_minor: 1, currency: "VND" },
  });
});

// A prefix committed key by key is never sent in place of the figure typed.
it("refuses a figure too large to store and sends nothing", async () => {
  const f = await newDealDialog();
  await f.user.type(f.amount, "99999999999999999999");
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  expect(confirm).toBeDisabled();
  await f.user.click(confirm);
  expect(
    screen.getByText("Enter an amount of at most 90071992547409.91."),
  ).toBeVisible();
  expect(f.posted).toHaveLength(0);
});

it("puts the server's amount refusal under the field in the reader's words", async () => {
  const f = await newDealDialog({
    type: "about:blank",
    title: "Validation failed",
    status: 422,
    code: "validation_error",
    detail: "amount_minor cannot be negative",
    details: {
      errors: [
        {
          field: "amount_minor",
          code: "amount_negative",
          message: "amount_minor cannot be negative",
        },
      ],
    },
  });
  await f.user.type(f.amount, "5");
  await f.user.click(
    screen.getByRole("button", { name: "Qualify and open deal" }),
  );
  expect(
    await screen.findByText("Enter an amount of zero or more."),
  ).toBeVisible();
  expect(screen.queryByText(/amount_minor/)).toBeNull();
});

it("does not promise whole units in the hint", async () => {
  await newDealDialog();
  expect(screen.queryByText(/Whole units/)).toBeNull();
});

it("takes no amount before the base currency is known, then keeps the typed amount at its scale", async () => {
  const user = steppedClock();
  vi.setSystemTime(new Date("2026-10-05T12:00:00Z"));
  const posted: unknown[] = [];
  let answerCurrency: (response: Response) => void = () => {};
  const currencyRead = new Promise<Response>((resolve) => {
    answerCurrency = resolve;
  });
  stubFetch(async (url, method, request) => {
    if (url.includes("/promote-preview"))
      return jsonResponse({ outcome: "create", existing_deals: [] });
    if (method === "POST" && url.includes("/promote")) {
      posted.push(JSON.parse(await request.text()));
      return jsonResponse({ contact: { id: "contact-one" }, merged: false });
    }
    if (url.includes("/installation/settings")) return currencyRead;
    return jsonResponse({ data: [] });
  });
  render(
    <QualifyDialog lead={lead} open onClose={vi.fn()} onQualified={vi.fn()} />,
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  await user.click(
    await screen.findByRole("radio", { name: "Open a new deal" }),
  );
  const early = await screen.findByLabelText("Amount ()");
  expect(early).toBeDisabled();
  await user.type(early, "500");

  answerCurrency(jsonResponse({ base_currency: "VND" }));
  const amount = await screen.findByLabelText("Amount (VND)");
  await waitFor(() => expect(amount).toBeEnabled());
  expect(amount).toHaveValue("");
  await user.type(amount, "500");
  await user.click(
    screen.getByRole("button", { name: "Qualify and open deal" }),
  );
  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    deal: { amount_minor: 500, currency: "VND" },
  });
});

it("says the base currency could not be read and reads it again on Retry", async () => {
  const user = steppedClock();
  vi.setSystemTime(new Date("2026-10-05T12:00:00Z"));
  let settingsUp = false;
  stubFetch(async (url) => {
    if (url.includes("/promote-preview"))
      return jsonResponse({ outcome: "create", existing_deals: [] });
    if (url.includes("/installation/settings"))
      return settingsUp
        ? jsonResponse({ base_currency: "VND" })
        : jsonResponse({ detail: "unavailable" }, 503);
    return jsonResponse({ data: [] });
  });
  render(
    <QualifyDialog lead={lead} open onClose={vi.fn()} onQualified={vi.fn()} />,
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  await user.click(
    await screen.findByRole("radio", { name: "Open a new deal" }),
  );
  expect(
    await screen.findByText(
      "The currency could not be read, so no amount can be entered yet.",
    ),
  ).toBeTruthy();

  settingsUp = true;
  await user.click(screen.getByRole("button", { name: "Retry" }));
  const amount = await screen.findByLabelText("Amount (VND)");
  await waitFor(() => expect(amount).toBeEnabled());
});

// A German reader writes "12,5" for twelve and a half, and "1.500" reads as
// fifteen hundred to them, so the dialog refuses it.
it("sends 12,5 typed in German as 1250 minor units", async () => {
  const f = await newDealDialog(undefined, "EUR", "de");
  await f.user.type(f.amount, "12,5");
  await f.user.click(
    screen.getByRole("button", { name: "Qualifizieren und Deal erstellen" }),
  );
  await waitFor(() => expect(f.posted).toHaveLength(1));
  expect(f.posted[0]).toMatchObject({
    deal: { amount_minor: 1250, currency: "EUR" },
  });
});

it("refuses 1.500 typed in German by naming the wanted form, and sends nothing", async () => {
  const f = await newDealDialog(undefined, "EUR", "de");
  await f.user.type(f.amount, "1.500");
  const confirm = screen.getByRole("button", {
    name: "Qualifizieren und Deal erstellen",
  });
  expect(confirm).toBeDisabled();
  await f.user.click(confirm);
  expect(
    screen.getByText("Gib den Betrag als 1500 oder 1500,00 ein."),
  ).toBeVisible();
  expect(f.posted).toHaveLength(0);
});
