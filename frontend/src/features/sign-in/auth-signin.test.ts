/** @vitest-environment happy-dom */
import { expect, it, vi } from "vitest";
import { landSignedIn } from "./auth-signin";

it("lands a bare entry on Home in its place, so Back does not show it again", () => {
  globalThis.history.replaceState(null, "", "#/");
  const entries = globalThis.history.length;
  const onPhase = vi.fn();
  landSignedIn(onPhase);
  expect(onPhase).toHaveBeenCalledWith("success");
  expect(globalThis.location.hash).toBe("#/home");
  expect(globalThis.history.length).toBe(entries);
});

it("keeps the deep link the reader followed", () => {
  globalThis.history.replaceState(null, "", "#/deals/d1");
  landSignedIn(vi.fn());
  expect(globalThis.location.hash).toBe("#/deals/d1");
});
