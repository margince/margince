/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { CoreFeature } from "../lib/corefeatures";
import { routeHash } from "../lib/router";
import { useFeatureLanding } from "./featurelanding";
import { firstAvailableRoute } from "./nav";

const HOME_OFF: readonly CoreFeature[] = ["home"];

afterEach(() => {
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
});

describe("the landing on an installation without Home", () => {
  it("redirects the bare address that Back lands on", async () => {
    globalThis.history.replaceState(null, "", "#/");
    globalThis.history.pushState(null, "", "#/deals");
    renderHook(() => useFeatureLanding(true, false, HOME_OFF));
    expect(globalThis.location.hash).toBe("#/deals");
    const landed = new Promise<void>((done) =>
      globalThis.addEventListener("popstate", () => done(), { once: true }),
    );
    await act(async () => {
      globalThis.history.back();
      await landed;
    });
    expect(globalThis.location.hash).toBe(
      routeHash(firstAvailableRoute(HOME_OFF)),
    );
  });
});
