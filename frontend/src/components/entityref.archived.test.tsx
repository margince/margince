// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { EntityRef } from "./entityref";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function answering(archived: boolean) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(new Request(input).url);
    return Response.json({
      data: url.searchParams
        .getAll("id")
        .map((id) => ({ id, name: "Brandt Automotive GmbH", archived })),
    });
  });
}

function renderCompanyRef() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <EntityRef kind="company" id="ref-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("marks an archived company as archived beside its name", async () => {
  answering(true);
  renderCompanyRef();
  expect(
    await screen.findByRole("link", { name: "Brandt Automotive GmbH" }),
  ).toBeVisible();
  expect(screen.getByText("Archived")).toBeVisible();
});

it("draws a live company with no mark", async () => {
  answering(false);
  renderCompanyRef();
  await screen.findByRole("link", { name: "Brandt Automotive GmbH" });
  expect(screen.queryByText("Archived")).toBeNull();
});
