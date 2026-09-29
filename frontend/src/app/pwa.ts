// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";

/** The script the build emits at the site root (scripts/vite-pwa.ts). */
export const SERVICE_WORKER_URL = "/sw.js";

export type InstallOutcome = "accepted" | "dismissed";

/** `manual-ios` is an iPhone or iPad browser, where Add to Home Screen is by
 *  hand; `available.prompt` asks the browser once; `installed` includes this visit. */
export type InstallState =
  | Readonly<{ kind: "installed" }>
  | Readonly<{ kind: "available"; prompt: () => Promise<InstallOutcome> }>
  | Readonly<{ kind: "manual-ios" }>
  | Readonly<{ kind: "unavailable" }>;

/** Chromium's install offer; no standard names it yet. */
interface BeforeInstallPromptEvent extends Event {
  readonly platforms: readonly string[];
  readonly userChoice: Promise<
    Readonly<{ outcome: InstallOutcome; platform: string }>
  >;
  prompt(): Promise<void>;
}

declare global {
  interface WindowEventMap {
    beforeinstallprompt: BeforeInstallPromptEvent;
    appinstalled: Event;
  }
}

let offered: BeforeInstallPromptEvent | null = null;
let installed = false;
let snapshot: InstallState | null = null;
const subscribers = new Set<() => void>();

function runsInstalled(): boolean {
  return (
    window.matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator && navigator.standalone === true)
  );
}

// iPadOS Safari reports a Mac, and only its touch points give it away.
function appleTouchDevice(): boolean {
  const agent = navigator.userAgent;
  return (
    /iPhone|iPad|iPod/.test(agent) ||
    (/Macintosh/.test(agent) && navigator.maxTouchPoints > 1)
  );
}

function offer(event: BeforeInstallPromptEvent): InstallState {
  return {
    kind: "available",
    prompt: async () => {
      if (offered === event) {
        offered = null;
      }
      await event.prompt();
      const { outcome } = await event.userChoice;
      // Installed from here: `appinstalled` can arrive later, and nothing is offered meanwhile.
      if (outcome === "accepted") {
        installed = true;
      }
      refresh();
      return outcome;
    },
  };
}

function currentState(): InstallState {
  if (installed || runsInstalled()) {
    return { kind: "installed" };
  }
  if (offered !== null) {
    return offer(offered);
  }
  return { kind: appleTouchDevice() ? "manual-ios" : "unavailable" };
}

function refresh(): void {
  snapshot = currentState();
  for (const notify of subscribers) {
    notify();
  }
}

function subscribe(notify: () => void): () => void {
  subscribers.add(notify);
  return () => {
    subscribers.delete(notify);
  };
}

function getSnapshot(): InstallState {
  snapshot ??= currentState();
  return snapshot;
}

/**
 * Keeps the browser's install offer for the app to present later. Called before
 * the first render, because the browser may make the offer before React mounts.
 */
export function listenForInstall(): void {
  window.addEventListener("beforeinstallprompt", (event) => {
    event.preventDefault();
    offered = event;
    refresh();
  });
  window.addEventListener("appinstalled", () => {
    installed = true;
    offered = null;
    refresh();
  });
}

export function useInstallState(): InstallState {
  return useSyncExternalStore(subscribe, getSnapshot);
}

/** After load, so installing the worker never competes with the app's own first load. */
export function registerServiceWorker(): void {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) {
    return;
  }
  window.addEventListener(
    "load",
    () => {
      navigator.serviceWorker
        .register(SERVICE_WORKER_URL, { scope: "/", updateViaCache: "none" })
        .catch((reason: unknown) => {
          console.warn("service worker registration failed", reason);
        });
    },
    { once: true },
  );
}
