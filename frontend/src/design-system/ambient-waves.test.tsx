// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { AmbientWaves } from "./ambient-waves";

// jsdom's `getContext("webgl2")` always returns null, so this is the fallback
// path every real browser without WebGL2 also takes: no renderer, no loop, and
// the caller's own CSS ground stays what a reader sees. That fallback is the
// one thing here that must never regress: it is the whole reason the
// component is allowed to fail closed rather than throw.

// vite.config.ts does not enable globals, so @testing-library's auto-cleanup
// never runs here without this.
afterEach(cleanup);

describe("AmbientWaves, on a host without WebGL2", () => {
  it("renders an aria-hidden canvas carrying its own class and the caller's", () => {
    const { container } = render(<AmbientWaves className="welcome-ground" />);
    const canvas = container.querySelector("canvas");

    expect(canvas).toBeInTheDocument();
    expect(canvas).toHaveAttribute("aria-hidden", "true");
    expect(canvas).toHaveClass("ambient-waves");
    expect(canvas).toHaveClass("welcome-ground");
  });

  it("renders the same canvas class with no className passed", () => {
    // The default is an empty string, not undefined: a caller-less mount
    // must not print a trailing space or an "undefined" token into the class
    // list.
    const { container } = render(<AmbientWaves />);
    const canvas = container.querySelector("canvas");

    expect(canvas).toHaveAttribute("class", "ambient-waves");
  });

  it("unmounts without throwing", () => {
    // There is no renderer to dispose of on this host, but the effect's
    // cleanup still runs, and a fallback path that assumed a live renderer
    // would throw here first.
    const { unmount } = render(<AmbientWaves />);
    expect(() => unmount()).not.toThrow();
  });
});
