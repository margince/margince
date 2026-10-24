// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { CompanyHoldSection } from "./companyhold";

type Company = components["schemas"]["Company"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderSection(company: Company): unknown[] {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /capture/counterparty-holds": () => jsonResponse({ data: [] }),
    "POST /capture/counterparty-holds": (body) => {
      posted.push(body);
      return jsonResponse({}, 201);
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyHoldSection company={company} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return posted;
}

it("holds the company's domain and never an invented address", async () => {
  const posted = renderSection({
    website_url: "https://www.example.com",
  } as Company);
  fireEvent.click(await screen.findByText("Private correspondence"));
  expect(screen.queryByText(/with this address/)).toBeNull();
  fireEvent.click(
    await screen.findByRole("button", { name: /everyone at example\.com/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "Keep private" }));
  await vi.waitFor(() =>
    expect(posted).toEqual([{ kind: "domain", value: "example.com" }]),
  );
  expect(JSON.stringify(posted)).not.toContain("x@");
});

it("renders nothing for a company with no website", () => {
  renderSection({ website_url: null } as Company);
  expect(screen.queryByText("Private correspondence")).toBeNull();
});

it("offers the hold from the primary domain row when website_url is absent", async () => {
  renderSection({
    website_url: null,
    domains: [{ domain: "example.com", is_primary: true }],
  } as Company);
  expect(await screen.findByText("Private correspondence")).toBeTruthy();
});
