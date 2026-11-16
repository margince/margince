// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { addressedAPathWeDoNotServe } from "./servedpath";

const at = (path: string) => globalThis.history.replaceState(null, "", path);

afterEach(() => {
  vi.unstubAllEnvs();
  at("/");
});

describe("addressedAPathWeDoNotServe", () => {
  it("serves the build's own documents", () => {
    for (const path of ["/", "/index.html"]) {
      at(path);
      expect(addressedAPathWeDoNotServe()).toBe(false);
    }
  });

  it("names a path a reader typed as one it does not serve", () => {
    at("/settings#/home");
    expect(addressedAPathWeDoNotServe()).toBe(true);
  });

  it("serves Storybook's story frame under Storybook alone", () => {
    at("/iframe.html?id=story#/onboarding");
    expect(addressedAPathWeDoNotServe()).toBe(true);

    vi.stubEnv("STORYBOOK", "true");
    expect(addressedAPathWeDoNotServe()).toBe(false);
    at("/settings");
    expect(addressedAPathWeDoNotServe()).toBe(true);
  });
});
