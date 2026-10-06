// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { RetireFieldConfirm } from "./customfields.retire";
import { LISTS_KEY } from "./lists.queries";
import { jsonResponse } from "./story-utils";

type CustomField = components["schemas"]["CustomField"];

const field: CustomField = {
  id: "01a0f000-0000-7000-8000-0000000000f1",
  object: "contact",
  label: "Loyalty band",
  slug: "loyalty_band",
  type: "text",
  status: "active",
  column_name: "cf_loyalty_band",
  created_by: "00000000-0000-4000-8000-000000000001",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the retire confirmation", () => {
  it("reads the lists on the field again after any list changes", async () => {
    let name = "Gold band";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          lists: [{ id: "l1", name, sharing: "team" }],
          unseen_count: 0,
        }),
      ),
    );
    // Nothing goes stale on its own here, so only an invalidation refetches.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <RetireFieldConfirm
            field={field}
            onClose={() => {}}
            onRetired={() => {}}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("Gold band")).toBeInTheDocument();
    name = "Gold band (renamed)";
    await act(() => client.invalidateQueries({ queryKey: [LISTS_KEY] }));
    expect(await screen.findByText("Gold band (renamed)")).toBeInTheDocument();
  });
});
