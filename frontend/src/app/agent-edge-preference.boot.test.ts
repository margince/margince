/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { STORAGE_KEYS } from "../lib/storage";

const EDGE_LIGHT_KEY = STORAGE_KEYS.edgeLight.name;
afterEach(() => window.localStorage.removeItem(EDGE_LIGHT_KEY));

// Resetting the module cache also replaces React contexts, so boot cases
// stay apart from mounted panels whose providers must share those contexts.
describe("the edge light preference on a fresh page", () => {
  // Module reloads must read storage rather than another test's remembered choice.
  it("meets a reader who turned it off last week with the frame still", async () => {
    window.localStorage.setItem(EDGE_LIGHT_KEY, "off");

    vi.resetModules();
    const booted = await import("./agent-edge-preference");

    expect(booted.edgeLightShown()).toBe(false);
  });

  it("meets an install that has never chosen with the light on", async () => {
    window.localStorage.removeItem(EDGE_LIGHT_KEY);

    vi.resetModules();
    const booted = await import("./agent-edge-preference");

    expect(booted.edgeLightShown()).toBe(true);
  });
});
