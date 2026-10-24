/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { useHealthReason } from "./companylookups";

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
