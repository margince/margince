// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { companyContextCapabilitiesQueryKey } from "../../lib/companycontextreach";
import { SETTINGS_PAGES } from "../../lib/settingscatalog";
import { meFixture } from "../../testing/mefixture";
import { usePageReadOnly, useVisibleSettingsTabs } from "./settingsnav";

// A settings tab is visible and read-only on one context. The company
// profile's tab opens on whether the company-context card draws, a query
// rather than a fact of the build. A read-only check without it called the tab
// read-only for the Admin it is written for.

const COMPANY_PAGE = SETTINGS_PAGES.find((page) => page.id === "company");

function wrapper(roles: string[], readEnabled: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    ["me"],
    meFixture({
      roles,
      rowScope: "all",
      allow: { installation_settings: ["read", "update"] },
    }),
  );
  client.setQueryData(companyContextCapabilitiesQueryKey, {
    read_enabled: readEnabled,
  });
  return ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the company profile tab", () => {
  it("is not read-only for the Admin who sees it", () => {
    const { result } = renderHook(
      () => ({
        tabs: useVisibleSettingsTabs(COMPANY_PAGE),
        readOnly: usePageReadOnly("company", "profile"),
      }),
      { wrapper: wrapper(["admin"], true) },
    );

    expect(result.current.tabs.map((tab) => tab.id)).toContain("profile");
    expect(result.current.readOnly).toBe(false);
  });

  it("is read-only where the company-context card does not draw", () => {
    const { result } = renderHook(() => usePageReadOnly("company", "profile"), {
      wrapper: wrapper(["admin"], false),
    });

    expect(result.current).toBe(true);
  });
});
