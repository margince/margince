/** @vitest-environment happy-dom */
import { QueryClient } from "@tanstack/react-query";
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetToSignedOut } from "./session";
import { STORAGE_KEYS } from "./storage";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Unconditionally, not at the end of the case that installed a spy: a case
  // that fails its assertion never reaches its own teardown, and a leaked
  // console spy silences the rest of the file — which looks exactly like a
  // suite that passes.
  vi.restoreAllMocks();
});

describe("signing out", () => {
  afterEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it("forgets what the member left in storage and keeps what no one else inherits", async () => {
    const declined = `${STORAGE_KEYS.platformDeclined.prefix}account-1`;
    localStorage.setItem(STORAGE_KEYS.faultsSeen.name, "run-1");
    sessionStorage.setItem(STORAGE_KEYS.oauthAttempt.name, "gmail");
    localStorage.setItem(STORAGE_KEYS.theme.name, "dark");
    localStorage.setItem(declined, "1");

    await resetToSignedOut(new QueryClient());

    expect(localStorage.getItem(STORAGE_KEYS.faultsSeen.name)).toBeNull();
    expect(sessionStorage.getItem(STORAGE_KEYS.oauthAttempt.name)).toBeNull();
    expect(localStorage.getItem(STORAGE_KEYS.theme.name)).toBe("dark");
    expect(localStorage.getItem(declined)).toBe("1");
  });
});
