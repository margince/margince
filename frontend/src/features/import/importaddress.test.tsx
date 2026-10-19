/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { runHref, useAddressedRun } from "./importaddress";

const RUN_PAGE = "#/settings/import/run";

afterEach(() => {
  cleanup();
  globalThis.location.hash = "";
});

describe("the run in the address", () => {
  it("reads the run the address names", () => {
    globalThis.location.hash = `${RUN_PAGE}?run=r1`;
    const { result } = renderHook(() => useAddressedRun());
    expect(result.current[0]).toBe("r1");
  });

  it("names no run when the address carries none", () => {
    globalThis.location.hash = RUN_PAGE;
    const { result } = renderHook(() => useAddressedRun());
    expect(result.current[0]).toBeNull();
  });

  it("writes a run into the address and clears it, keeping the other dials", async () => {
    globalThis.location.hash = `${RUN_PAGE}?ask=1`;
    const { result } = renderHook(() => useAddressedRun());

    act(() => result.current[1]("r2"));
    await waitFor(() => expect(result.current[0]).toBe("r2"));
    expect(globalThis.location.hash).toBe(`${RUN_PAGE}?ask=1&run=r2`);

    act(() => result.current[1](null));
    await waitFor(() => expect(result.current[0]).toBeNull());
    expect(globalThis.location.hash).toBe(`${RUN_PAGE}?ask=1`);
  });

  it("links the run page opened on one run", () => {
    expect(runHref("r 1")).toBe(`${RUN_PAGE}?run=r+1`);
  });
});
