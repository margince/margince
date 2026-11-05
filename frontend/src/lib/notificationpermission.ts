// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whether this browser lets Margince show notifications, as one shared value.
// The reminder poll and the settings row read the same answer, so a grant on
// the settings row starts the poll with no reload.

import { useSyncExternalStore } from "react";

export type BrowserNotificationPermission =
  | NotificationPermission
  | "unsupported";

const listeners = new Set<() => void>();
let watching = false;

function readPermission(): BrowserNotificationPermission {
  return typeof Notification === "undefined"
    ? "unsupported"
    : Notification.permission;
}

// The browser holds the answer, so every read asks it; a change only tells
// the readers to ask again.
function publish(): void {
  for (const listener of listeners) listener();
}

// The browser's own settings can change the answer while the tab is open. A
// browser without the Permissions API is read again on the next render.
function watchBrowserSettings(): void {
  if (watching || typeof navigator === "undefined" || !navigator.permissions)
    return;
  watching = true;
  navigator.permissions
    .query({ name: "notifications" })
    .then((status) => {
      status.onchange = publish;
    })
    .catch(() => {
      watching = false;
    });
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  watchBrowserSettings();
  return () => {
    listeners.delete(listener);
  };
}

export function useNotificationPermission(): BrowserNotificationPermission {
  return useSyncExternalStore(subscribe, readPermission);
}

/** Asks the browser. Call it only from a click: browsers refuse an ask made on load. */
export async function requestNotificationPermission(): Promise<BrowserNotificationPermission> {
  if (typeof Notification === "undefined") return "unsupported";
  const answer = await Notification.requestPermission();
  publish();
  return answer;
}
