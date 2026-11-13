/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  LIFECYCLE_LABELS as FORMAT_LABELS,
  LIFECYCLE_OPTIONS as FORMAT_OPTIONS,
} from "../format/companylifecycle";
import { type Locale, LocaleProvider, useT } from "../i18n";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import {
  formatSizeBand,
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  useHealthReason,
  usePaymentHealth,
} from "./companylookups";

afterEach(cleanup);

function reasonIn(locale: Locale, days: number): string {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <LocaleProvider initial={locale}>{children}</LocaleProvider>
  );
  const { result } = renderHook(() => useHealthReason(), { wrapper });
  return result.current({
    reason: "fallback sentence",
    reason_code: "last_met",
    reason_params: { days },
  });
}

describe("useHealthReason on a meeting held today", () => {
  it.each([
    ["en", "Last met them today."],
    ["de", "Heute getroffen."],
    ["vi", "Đã họp hôm nay."],
  ] as const)("says today in %s, never a count of zero", (locale, today) => {
    expect(reasonIn(locale, 0)).toBe(today);
  });

  it("keeps the counted sentence for a meeting on an earlier day", () => {
    expect(reasonIn("en", 1)).toBe("Last met them 1 day ago.");
    expect(reasonIn("en", 5)).toBe("Last met them 5 days ago.");
  });
});

describe("formatSizeBand", () => {
  it.each([
    ["en", "51–200 employees"],
    ["de", "51–200 Mitarbeitende"],
    ["vi", "51–200 nhân viên"],
  ] as const)("says what a band counts in %s", (locale, expected) => {
    const wrapper = ({ children }: { children: ReactNode }) => (
      <LocaleProvider initial={locale}>{children}</LocaleProvider>
    );
    const { result } = renderHook(() => formatSizeBand(useT(), "51-200"), {
      wrapper,
    });
    expect(result.current).toBe(expected);
  });
});

describe("the lifecycle lookups", () => {
  it("are the one list and label map the format layer holds", () => {
    expect(LIFECYCLE_OPTIONS).toBe(FORMAT_OPTIONS);
    expect(LIFECYCLE_LABELS).toBe(FORMAT_LABELS);
  });
});

// The health line reads the same overdue rule as the company banner.
describe("usePaymentHealth on overdue money", () => {
  type Summary = components["schemas"]["CompanyFinanceSummary"];
  const overdue: Summary = {
    company_id: "o-1",
    state: "connected",
    overdue: { amount_minor: 1240000, currency: "EUR" },
    last_synced_at: "2026-11-02T09:00:00Z",
  };

  async function healthOf(summary: Summary) {
    installFetchStub({
      "GET /companies/o-1/finance-summary": () => jsonResponse(summary),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    );
    const { result } = renderHook(() => usePaymentHealth("o-1"), { wrapper });
    await waitFor(() => expect(client.isFetching()).toBe(0));
    await waitFor(() => expect(result.current).toBeDefined());
    return result.current;
  }

  it("is at risk while the figure is current", async () => {
    expect(await healthOf(overdue)).toEqual({
      rating: "at_risk",
      reason: "Payment is overdue.",
    });
  });

  it.each(["stale", "error"] as const)(
    "dates the overdue reason from a %s sync",
    async (state) => {
      const health = await healthOf({ ...overdue, state });
      expect(health?.reason).toMatch(
        /^Payment was overdue at the last accounting sync on .*2026.*\.$/,
      );
    },
  );
});
