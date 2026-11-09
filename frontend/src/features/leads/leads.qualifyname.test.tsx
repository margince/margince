/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
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

// The dialog under a held company-name read. `body` is what the promotion sent.
function heldNameDialog() {
  const user = steppedClock();
  vi.setSystemTime(new Date("2026-10-05T12:00:00Z"));
  let releaseName: () => void = () => {};
  const nameRead = new Promise<void>((resolve) => {
    releaseName = resolve;
  });
  let body: unknown;
  stubFetch(async (url, method, request) => {
    const named = companyNamesAnswer(url, { "c-7": "Nordwind Holding SE" });
    if (named) {
      await nameRead;
      return named;
    }
    if (url.includes("/promote-preview")) {
      return jsonResponse({
        outcome: "create",
        company_id: "c-7",
        existing_deals: [],
      });
    }
    if (method === "POST" && url.includes("/promote")) {
      body = JSON.parse(await request.text());
      return jsonResponse({ contact: { id: "contact-one" }, merged: false });
    }
    return jsonResponse({ data: [] });
  });
  render(
    <QualifyDialog
      lead={{ ...lead, company_id: "c-7" }}
      open
      onClose={vi.fn()}
      onQualified={vi.fn()}
    />,
    new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  );
  return { user, releaseName: () => releaseName(), body: () => body };
}

// A new deal is named after the lead's company. While the linked company's own
// name is still being read, the typed name stands in, and it may be the stale
// one. Confirming waits for the read, so the stale name is never saved.
it("waits for the linked company's own name before opening a deal named after it", async () => {
  const { user, releaseName, body } = heldNameDialog();
  await user.click(
    await screen.findByRole("radio", { name: "Open a new deal" }),
  );
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  expect(confirm).toBeDisabled();

  releaseName();
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);
  await waitFor(() =>
    expect(body()).toMatchObject({
      deal: { name: expect.stringContaining("Nordwind Holding SE") },
    }),
  );
});

// A deal name the reader typed is not the company's, so nothing waits on it.
it("does not wait for the company's name when the deal is named by hand", async () => {
  const { user, body } = heldNameDialog();
  await user.click(
    await screen.findByRole("radio", { name: "Open a new deal" }),
  );
  await user.type(screen.getByLabelText("Deal name"), "Fleet renewal");
  const confirm = screen.getByRole("button", { name: "Qualify and open deal" });
  expect(confirm).toBeEnabled();
  await user.click(confirm);
  await waitFor(() =>
    expect(body()).toMatchObject({ deal: { name: "Fleet renewal" } }),
  );
});
