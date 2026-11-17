// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { ScreenAddress } from "../../lib/screenaddress";
import { useShownCompanyTab } from "./companytab";

const drawnFor =
  (address: string) =>
  ({ children }: Readonly<{ children: ReactNode }>) =>
    createElement(ScreenAddress.Provider, { value: address }, children);

afterEach(() => {
  cleanup();
  globalThis.history.replaceState(null, "", "/");
});

describe("an archived account addressed on a tab it cannot answer", () => {
  it("opens Overview, and the address names it", () => {
    globalThis.history.replaceState(null, "", "/#/companies/o1/deals");
    const view = renderHook(() => useShownCompanyTab("o1", true, "deals"), {
      wrapper: drawnFor("#/companies/o1/deals"),
    });

    expect(view.result.current).toBe("overview");
    expect(globalThis.location.hash).toBe("#/companies/o1/overview");
  });

  it("leaves the entry the reader moved on to when its read lands late", () => {
    globalThis.history.replaceState(null, "", "/#/home");
    const view = renderHook(
      ({ archived }) => useShownCompanyTab("o1", archived, "deals"),
      {
        wrapper: drawnFor("#/companies/o1/deals"),
        initialProps: { archived: false },
      },
    );

    view.rerender({ archived: true });

    expect(globalThis.location.hash).toBe("#/home");
  });
});
