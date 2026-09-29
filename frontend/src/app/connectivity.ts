// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useSyncExternalStore } from "react";
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
// Only a surface that states the outage may hold one: a pause nothing explains
// is, to the reader, a page that never loads.
let watchers = 0;

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

// A hidden tab has nobody to show the answer to.
function scheduleProbe(delayMs: number): void {
  if (
    connectivityNow() !== "unreachable" ||
    watchers === 0 ||
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

/** A request that never reached the api: the outage a banner now states, or
 *  null where none watches and the failure stays that surface's own to show. */
export function reportUnreached(): Outage | null {
  if (watchers === 0) {
    return null;
  }
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

/** Follows the state without holding an outage open: the device's own report
 *  still reaches it, and a server outage only while something watches. */
export function subscribeConnectivity(notify: () => void): () => void {
  if (subscribers.size === 0 && typeof window !== "undefined") {
    listen();
  }
  subscribers.add(notify);
  published = connectivityNow();
  return () => {
    subscribers.delete(notify);
    if (subscribers.size === 0 && typeof window !== "undefined") {
      stopListening();
    }
  };
}

// The last watcher gone, the server's standing goes with the probe that kept
// it, so every follower is released rather than left paused behind nothing.
export function watchConnectivity(notify: () => void): () => void {
  const unsubscribe = subscribeConnectivity(notify);
  watchers += 1;
  return () => {
    unsubscribe();
    watchers -= 1;
    if (watchers === 0) {
      cancelProbe();
      reached = true;
      failedProbes = 0;
      publish();
    }
  };
}

/** For the surface that states the outage, and only that one: while it
 *  watches, a server outage is held, probed and paused behind. */
export function useConnectivity(): Connectivity {
  return useSyncExternalStore(watchConnectivity, connectivityNow);
}

// One session check in flight at a time, whichever screen sent it.
let checking = false;

/** A screen that states the outage itself holds it open while `active`, and
 *  checks once so a failure lands while it holds and the probe can end it. */
export function useOutageRecovery(
  active: boolean,
  check: () => Promise<{ response: Response }>,
  recheck: () => void,
): void {
  const sent = useRef(false);
  useEffect(
    () => (active ? watchConnectivity(() => undefined) : undefined),
    [active],
  );
  useEffect(() => {
    if (!active || sent.current || checking) {
      return;
    }
    sent.current = true;
    checking = true;
    // Not `recheck`: refetching the session drops the screen for a splash,
    // and the remount would check again, and again.
    check()
      .then(
        ({ response }) => {
          if (response.ok) {
            recheck();
          }
        },
        // Refused: the client has recorded the outage this check was sent for.
        () => undefined,
      )
      .finally(() => {
        checking = false;
      });
  }, [active, check, recheck]);
}
