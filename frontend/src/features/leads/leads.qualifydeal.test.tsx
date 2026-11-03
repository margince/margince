/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { leadPromotePreviewKey } from "../../lib/leadkeys";
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

function fixture(deals: { id: string; name: string }[], cursor?: string) {
  const user = steppedClock();
  vi.setSystemTime(new Date("2026-10-05T12:00:00Z"));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  let body: unknown;
  let current = {
    outcome: "create",
    company_id: "company-one",
    existing_deals: deals,
    existing_deals_cursor: cursor,
  };
  stubFetch(async (url, method, request) => {
    if (url.includes("/promote-preview")) return jsonResponse(current);
    if (method === "POST" && url.includes("/promote")) {
      body = JSON.parse(await request.text());
      return jsonResponse({ contact: { id: "contact-one" }, merged: false });
    }
    return jsonResponse({ data: [] });
  });
  render(
    <QualifyDialog lead={lead} open onClose={vi.fn()} onQualified={vi.fn()} />,
    client,
  );
  return {
    user,
    body: () => body,
    refresh: async (next = current) => {
      current = next;
      await act(async () => {
        await client.invalidateQueries({
          queryKey: leadPromotePreviewKey(lead.id),
        });
      });
    },
  };
}

it("preselects the only eligible deal and submits only its id", async () => {
  const f = fixture([{ id: "deal-one", name: "Annual renewal" }]);
  expect(
    await screen.findByRole("radio", { name: "Add to an existing deal" }),
  ).toBeChecked();
  expect(
    screen.getByRole("radio", { name: "Open a new deal" }),
  ).not.toBeChecked();
  await f.user.click(
    screen.getByRole("button", { name: "Qualify and add to deal" }),
  );
  await waitFor(() =>
    expect(f.body()).toMatchObject({ existing_deal_id: "deal-one" }),
  );
  expect(f.body()).not.toHaveProperty("deal");
});

it("keeps an explicit opt-out when the preview refreshes", async () => {
  const f = fixture([{ id: "deal-one", name: "Annual renewal" }]);
  await f.user.click(
    await screen.findByRole("radio", { name: "Qualify without a deal" }),
  );
  await f.refresh();
  expect(
    screen.getByRole("radio", { name: "Add to an existing deal" }),
  ).not.toBeChecked();
  await f.user.click(screen.getByRole("button", { name: "Qualify" }));
  await waitFor(() => expect(f.body()).toBeTruthy());
  expect(f.body()).not.toHaveProperty("existing_deal_id");
  expect(f.body()).not.toHaveProperty("deal");
});

it("offers a new deal without an existing-deal control when there are none", async () => {
  fixture([]);
  await screen.findByText("Qualifying creates a new contact.");
  expect(
    screen.queryByRole("radio", { name: "Add to an existing deal" }),
  ).toBeNull();
  expect(screen.getByRole("radio", { name: "Open a new deal" })).toBeTruthy();
});

it("does not treat the first page as a unique deal when another page exists", async () => {
  fixture([{ id: "deal-one", name: "Annual renewal" }], "next-page");
  expect(
    await screen.findByRole("radio", { name: "Add to an existing deal" }),
  ).not.toBeChecked();
  expect(screen.getByRole("button", { name: "Show more deals" })).toBeTruthy();
});

it("refuses a selected deal after its employer changes", async () => {
  const f = fixture([{ id: "deal-one", name: "Annual renewal" }]);
  const existing = await screen.findByRole("radio", {
    name: "Add to an existing deal",
  });
  expect(existing).toBeChecked();
  await f.refresh({
    outcome: "create",
    company_id: "company-two",
    existing_deals: [{ id: "deal-two", name: "New rollout" }],
    existing_deals_cursor: undefined,
  });
  await screen.findByText(
    "That deal is no longer available. Choose another deal or qualify without one.",
  );
  expect(
    screen.getByRole("radio", { name: "Add to an existing deal" }),
  ).not.toBeChecked();
  expect(screen.getByRole("button", { name: "Qualify" })).toBeDisabled();
  await f.user.click(screen.getByRole("button", { name: "Qualify" }));
  expect(screen.getByRole("status")).toHaveTextContent(
    "That deal is no longer available",
  );
  expect(f.body()).toBeUndefined();
});

it("pins the automatically selected deal when a refresh returns a different single candidate", async () => {
  const f = fixture([{ id: "deal-one", name: "Annual renewal" }]);
  expect(
    await screen.findByRole("radio", { name: "Add to an existing deal" }),
  ).toBeChecked();
  await f.refresh({
    outcome: "create",
    company_id: "company-one",
    existing_deals: [{ id: "deal-two", name: "New rollout" }],
    existing_deals_cursor: undefined,
  });
  await screen.findByText(
    "That deal is no longer available. Choose another deal or qualify without one.",
  );
  expect(screen.getByRole("button", { name: "Qualify" })).toBeDisabled();
  expect(
    screen.getByRole("radio", { name: "Add to an existing deal" }),
  ).not.toBeChecked();
  expect(f.body()).toBeUndefined();
});
