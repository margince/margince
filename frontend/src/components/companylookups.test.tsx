/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import {
  LIFECYCLE_LABELS as FORMAT_LABELS,
  LIFECYCLE_OPTIONS as FORMAT_OPTIONS,
} from "../format/companylifecycle";
import { type Locale, LocaleProvider, useT } from "../i18n";
import {
  formatSizeBand,
  LIFECYCLE_LABELS,
  LIFECYCLE_OPTIONS,
  useHealthReason,
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
