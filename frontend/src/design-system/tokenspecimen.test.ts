/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { describeLength, useThemeValues, useTokenValue } from "./tokenspecimen";

let sheet: HTMLStyleElement;

beforeEach(() => {
  sheet = document.createElement("style");
  sheet.textContent = `
    .avatar-mesh, .specimen-host { --specimen-ink: #111111; --specimen-gap: 8px; }
    [data-theme="dark"] .avatar-mesh,
    [data-theme="dark"] .specimen-host { --specimen-ink: #eeeeee; }
  `;
  document.head.append(sheet);
});

afterEach(() => {
  cleanup();
  sheet.remove();
  document.documentElement.removeAttribute("data-theme");
});

describe("useThemeValues", () => {
  it("reads each token in the light and the dark theme", () => {
    const { result } = renderHook(() =>
      useThemeValues(["--specimen-ink", "--specimen-gap"]),
    );
    expect(result.current.get("--specimen-ink")).toEqual({
      light: "#111111",
      dark: "#eeeeee",
    });
    expect(result.current.get("--specimen-gap")).toEqual({
      light: "8px",
      dark: "8px",
    });
  });

  it("returns an empty string for a token nothing declares", () => {
    const { result } = renderHook(() => useThemeValues(["--specimen-absent"]));
    expect(result.current.get("--specimen-absent")).toEqual({
      light: "",
      dark: "",
    });
  });

  it("removes data-theme again when the root had none", () => {
    renderHook(() => useThemeValues(["--specimen-ink"]));
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("puts back the theme the root carried before the read", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    renderHook(() => useThemeValues(["--specimen-ink"]));
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("reads again when the token list changes", () => {
    const { result, rerender } = renderHook(
      ({ tokens }) => useThemeValues(tokens),
      { initialProps: { tokens: ["--specimen-ink"] } },
    );
    rerender({ tokens: ["--specimen-gap"] });
    expect([...result.current.keys()]).toEqual(["--specimen-gap"]);
  });
});

describe("useTokenValue", () => {
  it("stays empty while no element is attached to the ref", () => {
    const { result } = renderHook(() =>
      useTokenValue<HTMLDivElement>("--specimen-ink"),
    );
    expect(result.current.value).toBe("");
    expect(result.current.width).toBe(0);
  });

  it("follows the toolbar flipping the theme on the root", async () => {
    const host = document.createElement("div");
    host.className = "specimen-host";
    document.body.append(host);
    const { result, rerender } = renderHook(
      ({ token }) => {
        const out = useTokenValue<HTMLDivElement>(token);
        out.ref.current = host;
        return out;
      },
      { initialProps: { token: "--specimen-ink" } },
    );
    // The ref is attached during render, so a token change re-runs the read.
    rerender({ token: "--specimen-gap" });
    await waitFor(() => expect(result.current.value).toBe("8px"));
    rerender({ token: "--specimen-ink" });
    await waitFor(() => expect(result.current.value).toBe("#111111"));
    await act(async () => {
      // happy-dom caches an element's computed style until the element itself
      // changes, so touch the host as a browser's style recalc would.
      host.className = "specimen-host themed";
      document.documentElement.setAttribute("data-theme", "dark");
    });
    await waitFor(() => expect(result.current.value).toBe("#eeeeee"));
    host.remove();
  });
});

describe("describeLength", () => {
  it("resolves rem against the root font size, not a fixed 16px", () => {
    const root = document.documentElement;
    root.style.fontSize = "20px";
    try {
      expect(describeLength("1rem", 0)).toBe("1rem · 20px");
      expect(describeLength("40px", 0)).toBe("2rem · 40px");
    } finally {
      root.style.removeProperty("font-size");
    }
  });

  it("falls back to 16px when the root font size is not positive", () => {
    const root = document.documentElement;
    root.style.fontSize = "0px";
    try {
      expect(describeLength("32px", 0)).toBe("2rem · 32px");
    } finally {
      root.style.removeProperty("font-size");
    }
  });

  it("prints a px value in both units", () => {
    expect(describeLength("24px", 0)).toBe("1.5rem · 24px");
  });

  it("converts a rem value at 16px to the rem", () => {
    expect(describeLength("0.5rem", 0)).toBe("0.5rem · 8px");
  });

  it("accepts a negative length", () => {
    expect(describeLength("-4px", 0)).toBe("-0.25rem · -4px");
  });

  it("falls back to the measured width when the value is not a length", () => {
    expect(describeLength("calc(100% - 4px)", 12.345)).toBe(
      "0.772rem · 12.35px",
    );
  });
});
