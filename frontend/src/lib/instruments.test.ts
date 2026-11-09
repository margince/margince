// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  notifyNavigate,
  reportError,
  resetInstruments,
  startInstruments,
} from "./instruments";

afterEach(() => {
  resetInstruments();
  vi.restoreAllMocks();
});

describe("startInstruments", () => {
  it("hands each unit its version and its own edge path", () => {
    const seen: unknown[] = [];
    startInstruments({ alpha: (ctx) => void seen.push(ctx) }, "1.2.3");
    expect(seen).toEqual([
      { version: "1.2.3", edge: "/v1/public/instrument/alpha/" },
    ]);
  });

  it("survives an instrument that throws, and still starts the others", () => {
    const err = vi.spyOn(console, "error").mockImplementation(() => {});
    const onError = vi.fn();
    startInstruments(
      {
        bad: () => {
          throw new Error("init");
        },
        good: () => ({ onError }),
      },
      "",
    );
    reportError(new Error("x"), { caught: true });
    expect(onError).toHaveBeenCalledOnce();
    expect(err).toHaveBeenCalled();
  });
});

describe("reportError", () => {
  it("passes the component stack and whether a boundary caught it", () => {
    const onError = vi.fn();
    startInstruments({ a: () => ({ onError }) }, "");
    const e = new Error("render");
    reportError(e, { componentStack: "at X", caught: false });
    expect(onError).toHaveBeenCalledWith(e, {
      componentStack: "at X",
      caught: false,
    });
  });

  it("swallows a hook that throws", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    startInstruments(
      {
        a: () => ({
          onError: () => {
            throw new Error("hook");
          },
        }),
      },
      "",
    );
    expect(() => reportError(new Error("x"), { caught: true })).not.toThrow();
  });

  it("logs an async hook that rejects instead of leaving it unhandled", async () => {
    const err = vi.spyOn(console, "error").mockImplementation(() => {});
    const failure = new Error("async hook");
    startInstruments(
      { a: () => ({ onError: async () => Promise.reject(failure) }) },
      "",
    );
    reportError(new Error("x"), { caught: true });
    await vi.waitFor(() =>
      expect(err).toHaveBeenCalledWith(
        "extension a: instrument onError threw",
        failure,
      ),
    );
  });
});

describe("notifyNavigate", () => {
  it("tells instruments once per distinct route pattern", () => {
    const onNavigate = vi.fn();
    startInstruments({ a: () => ({ onNavigate }) }, "");
    notifyNavigate("contacts/:id");
    notifyNavigate("contacts/:id");
    notifyNavigate("deals");
    expect(onNavigate.mock.calls).toEqual([["contacts/:id"], ["deals"]]);
  });
});

describe("boot order", () => {
  // Starting after the first render would miss the errors of that render.
  it("starts the instruments before the app first renders", () => {
    const main = readFileSync(resolve(__dirname, "../main.tsx"), "utf8");
    const start = main.indexOf("startInstruments(");
    expect(start).toBeGreaterThan(-1);
    expect(start).toBeLessThan(main.indexOf("createRoot(root"));
  });
});
