/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { InstallOutcome, InstallState } from "./pwa";

const DESKTOP = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0";
const IPHONE = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Safari";
const IPAD = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15";

const shadowed: string[] = [];

function device(properties: Readonly<Record<string, unknown>>): void {
  for (const [name, value] of Object.entries(properties)) {
    Object.defineProperty(navigator, name, { configurable: true, value });
    shadowed.push(name);
  }
}

function displayMode(standalone: boolean): void {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: standalone && query === "(display-mode: standalone)",
    media: query,
  }));
}

afterEach(() => {
  for (const name of shadowed.splice(0)) {
    Reflect.deleteProperty(navigator, name);
  }
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

/** A fresh module, because the install store lives as long as the page does. */
async function page(userAgent = DESKTOP, touchPoints = 0) {
  device({ userAgent, maxTouchPoints: touchPoints });
  vi.resetModules();
  const pwa = await import("./pwa");
  pwa.listenForInstall();
  const hook = renderHook(() => pwa.useInstallState());
  return { pwa, state: () => hook.result.current };
}

function offer(outcome: InstallOutcome) {
  const prompt = vi.fn(async () => undefined);
  const event = Object.assign(
    new Event("beforeinstallprompt", { cancelable: true }),
    {
      platforms: ["web"],
      userChoice: Promise.resolve({ outcome, platform: "web" }),
      prompt,
    },
  );
  act(() => {
    window.dispatchEvent(event);
  });
  return { event, prompt };
}

async function promptFrom(
  state: InstallState,
): Promise<InstallOutcome | undefined> {
  if (state.kind !== "available") {
    throw new Error(`nothing to prompt from a state that is ${state.kind}`);
  }
  let outcome: InstallOutcome | undefined;
  await act(async () => {
    outcome = await state.prompt();
  });
  return outcome;
}

describe("the install state", () => {
  it("has nothing to offer until the browser offers", async () => {
    displayMode(false);
    const { state } = await page();
    expect(state().kind).toBe("unavailable");
  });

  it("keeps the browser's offer instead of letting it show its own", async () => {
    displayMode(false);
    const { state } = await page();
    const { event } = offer("accepted");
    expect(event.defaultPrevented).toBe(true);
    expect(state().kind).toBe("available");
  });

  it("asks once and is installed the moment the reader accepts, before the browser confirms", async () => {
    displayMode(false);
    const { state } = await page();
    const { prompt } = offer("accepted");
    expect(await promptFrom(state())).toBe("accepted");
    expect(prompt).toHaveBeenCalledTimes(1);
    expect(state().kind).toBe("installed");
  });

  it("is installed when the browser installs it without being asked", async () => {
    displayMode(false);
    const { state } = await page();
    act(() => {
      window.dispatchEvent(new Event("appinstalled"));
    });
    expect(state().kind).toBe("installed");
  });

  it("is dismissed after a dismissal, until the browser offers again", async () => {
    displayMode(false);
    const { state } = await page();
    offer("dismissed");
    expect(await promptFrom(state())).toBe("dismissed");
    expect(state().kind).toBe("dismissed");
    offer("accepted");
    expect(state().kind).toBe("available");
  });

  it("is installed when the browser installs it after a dismissal", async () => {
    displayMode(false);
    const { state } = await page();
    offer("dismissed");
    await promptFrom(state());
    act(() => {
      window.dispatchEvent(new Event("appinstalled"));
    });
    expect(state().kind).toBe("installed");
  });

  it("is dismissed again when the second offer is turned down too", async () => {
    displayMode(false);
    const { state } = await page();
    offer("dismissed");
    await promptFrom(state());
    offer("dismissed");
    expect(await promptFrom(state())).toBe("dismissed");
    expect(state().kind).toBe("dismissed");
  });

  it("is installed when the app runs standalone", async () => {
    displayMode(true);
    const { state } = await page();
    expect(state().kind).toBe("installed");
  });

  it("is installed when iOS runs it from the home screen", async () => {
    displayMode(false);
    device({ standalone: true });
    const { state } = await page(IPHONE, 5);
    expect(state().kind).toBe("installed");
  });

  it("is added by hand on an iPhone, and on an iPad that reports a Mac", async () => {
    displayMode(false);
    expect((await page(IPHONE, 5)).state().kind).toBe("manual-ios");
    expect((await page(IPAD, 5)).state().kind).toBe("manual-ios");
    expect((await page(IPAD, 0)).state().kind).toBe("unavailable");
  });
});

describe("registering the service worker", () => {
  function workerSupport(register: () => Promise<unknown>) {
    const spy = vi.fn(register);
    device({ serviceWorker: { register: spy } });
    return spy;
  }

  async function registerThenLoad() {
    vi.resetModules();
    const { registerServiceWorker } = await import("./pwa");
    registerServiceWorker();
    window.dispatchEvent(new Event("load"));
  }

  it("registers the root worker once the page has loaded, bypassing the HTTP cache", async () => {
    vi.stubEnv("PROD", true);
    const register = workerSupport(async () => ({}));
    vi.resetModules();
    const { registerServiceWorker } = await import("./pwa");
    registerServiceWorker();
    expect(register).not.toHaveBeenCalled();
    window.dispatchEvent(new Event("load"));
    expect(register).toHaveBeenCalledExactlyOnceWith("/sw.js", {
      scope: "/",
      updateViaCache: "none",
    });
  });

  it("registers nothing in a development build", async () => {
    vi.stubEnv("PROD", false);
    const register = workerSupport(async () => ({}));
    await registerThenLoad();
    expect(register).not.toHaveBeenCalled();
  });

  it("does nothing where the browser has no service workers", async () => {
    vi.stubEnv("PROD", true);
    const listen = vi.spyOn(window, "addEventListener");
    await registerThenLoad();
    expect(listen).not.toHaveBeenCalled();
  });

  it("says once that registration failed, and throws nothing", async () => {
    vi.stubEnv("PROD", true);
    const failure = new DOMException("refused", "SecurityError");
    workerSupport(async () => {
      throw failure;
    });
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    await registerThenLoad();
    await vi.waitFor(() => {
      expect(warn).toHaveBeenCalledTimes(1);
    });
    expect(warn).toHaveBeenCalledWith(
      "service worker registration failed",
      failure,
    );
  });
});
