// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import type { MessageKey } from "../i18n/en";

/** `offline`: the browser reports no network. `unreachable`: it has one, and
 *  Margince is not answering on it. */
export type Connectivity = "online" | "offline" | "unreachable";

export type Outage = Exclude<Connectivity, "online">;

/** A request that never reached Margince, and which outage stopped it. */
export class ConnectivityError extends Error {
  readonly outage: Outage;
  readonly method: string;
  constructor(outage: Outage, request: Request, cause: unknown) {
    super(`${request.method} ${request.url} did not reach Margince`, {
      cause,
    });
    this.name = "ConnectivityError";
    this.outage = outage;
    this.method = request.method;
  }
}

const READ_METHODS = new Set(["GET", "HEAD"]);

/** A write the network refused was not saved, and says which outage stopped
 *  it; a read saved nothing, and reruns by itself once the connection is back. */
export function unsavedWriteKey(error: unknown): MessageKey | null {
  if (!(error instanceof ConnectivityError) || READ_METHODS.has(error.method)) {
    return null;
  }
  return error.outage === "offline"
    ? "connectivity.unsaved.offline"
    : "connectivity.unsaved.unreachable";
}

/** Proxied to the api on every origin the app is served from. */
const HEALTH_URL = "/healthz";
const FIRST_PROBE_MS = 2_000;
const PROBE_CEILING_MS = 30_000;
// A probe into a dead socket would otherwise hold back every probe after it.
const PROBE_DEADLINE_MS = 10_000;

let reached = true;
let failedProbes = 0;
let probeTimer: ReturnType<typeof setTimeout> | undefined;
let probing = false;
let published: Connectivity = connectivityNow();
const subscribers = new Set<() => void>();

function deviceOnline(): boolean {
  return typeof navigator === "undefined" || navigator.onLine !== false;
}

function documentHidden(): boolean {
  return (
    typeof document !== "undefined" && document.visibilityState === "hidden"
  );
}

export function connectivityNow(): Connectivity {
  if (!deviceOnline()) {
    return "offline";
  }
  return reached ? "online" : "unreachable";
}

function publish(): void {
  const now = connectivityNow();
  if (now === published) {
    return;
  }
  published = now;
  for (const notify of subscribers) {
    notify();
  }
}

function cancelProbe(): void {
  globalThis.clearTimeout(probeTimer);
  probeTimer = undefined;
}

function backoff(): number {
  return Math.min(FIRST_PROBE_MS * 2 ** failedProbes, PROBE_CEILING_MS);
}

// Nobody listening means nobody to tell, and a hidden tab has nobody to show.
function scheduleProbe(delayMs: number): void {
  if (
    connectivityNow() !== "unreachable" ||
    subscribers.size === 0 ||
    probeTimer !== undefined ||
    probing ||
    documentHidden()
  ) {
    return;
  }
  probeTimer = globalThis.setTimeout(() => {
    probeTimer = undefined;
    void probe();
  }, delayMs);
}

async function probe(): Promise<void> {
  probing = true;
  const healthy = await answersHealthy();
  probing = false;
  if (healthy) {
    reportReached();
  } else if (connectivityNow() === "unreachable") {
    failedProbes += 1;
    scheduleProbe(backoff());
  }
}

async function answersHealthy(): Promise<boolean> {
  const deadline = new AbortController();
  const expiry = globalThis.setTimeout(
    () => deadline.abort(),
    PROBE_DEADLINE_MS,
  );
  try {
    const response = await globalThis.fetch(HEALTH_URL, {
      cache: "no-store",
      signal: deadline.signal,
    });
    return response.ok;
  } catch {
    // Unanswered is the verdict this probe was sent to hear.
    return false;
  } finally {
    globalThis.clearTimeout(expiry);
  }
}

/** Any HTTP answer from the api, a 5xx included, proves Margince reachable. */
export function reportReached(): void {
  reached = true;
  failedProbes = 0;
  cancelProbe();
  publish();
}

/** A request that never reached the api. Only a device that is online can say
 *  anything about the server, so an offline one keeps the server's standing. */
export function reportUnreached(): Outage {
  if (!deviceOnline()) {
    publish();
    return "offline";
  }
  reached = false;
  publish();
  scheduleProbe(backoff());
  return "unreachable";
}

// A reader coming back to the tab or the network wants an answer now, not at
// the end of a backoff that grew while nobody was looking.
function resumeProbing(): void {
  failedProbes = 0;
  cancelProbe();
  scheduleProbe(0);
}

function onDeviceOnline(): void {
  publish();
  resumeProbing();
}

function onDeviceOffline(): void {
  publish();
  cancelProbe();
}

function onVisibilityChange(): void {
  if (documentHidden()) {
    cancelProbe();
  } else {
    resumeProbing();
  }
}

function listen(): void {
  window.addEventListener("online", onDeviceOnline);
  window.addEventListener("offline", onDeviceOffline);
  document.addEventListener("visibilitychange", onVisibilityChange);
}

function stopListening(): void {
  window.removeEventListener("online", onDeviceOnline);
  window.removeEventListener("offline", onDeviceOffline);
  document.removeEventListener("visibilitychange", onVisibilityChange);
}

// With nobody listening the probe stops, so the server's standing is dropped
// rather than handed stale to the next listener.
export function subscribeConnectivity(notify: () => void): () => void {
  if (subscribers.size === 0 && typeof window !== "undefined") {
    listen();
  }
  subscribers.add(notify);
  published = connectivityNow();
  scheduleProbe(backoff());
  return () => {
    subscribers.delete(notify);
    if (subscribers.size > 0) {
      return;
    }
    if (typeof window !== "undefined") {
      stopListening();
    }
    cancelProbe();
    reached = true;
    failedProbes = 0;
  };
}

export function useConnectivity(): Connectivity {
  return useSyncExternalStore(subscribeConnectivity, connectivityNow);
}
