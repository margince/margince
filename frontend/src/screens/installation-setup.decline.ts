// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import { readStored, STORAGE_KEYS, writeStored } from "../app/storage";
import { useMe } from "./common";

/**
 * Where "Not now" on the platform question is remembered.
 *
 * The app step is asked of the contact running the cold start, once. The server
 * has no word for "asked and declined" — the step is simply unconfigured until
 * an app is stored, from here or from Settings — so the decline lives in this
 * browser.
 *
 * KEYED BY THE ACCOUNT THAT GAVE IT. A mark keyed on the browser alone outlives
 * the installation it was about: a machine that had run one cold start carried
 * that answer into the next, and the second installation's setup skipped the
 * platform question with nothing on screen to say why — the one step that asks
 * for the company's OAuth app, silently gone, on the run that most needed
 * it. A re-claimed installation mints its own administrator, so its cold start
 * asks again; the same contact on the same installation is still asked once.
 */
function declinedKey(account: string) {
  return { family: STORAGE_KEYS.platformDeclined, member: account };
}

// The accounts that declined in THIS tab, whether or not storage kept it.
const declinedThisSession = new Set<string>();

/**
 * Forgets the in-tab declines — the counterpart to `localStorage.clear()`,
 * and needed for the same reason: this set is the half of the answer that
 * storage did not keep, so clearing one without the other leaves a decline
 * standing that the caller believes they erased. A test suite whose cases
 * each start from an unanswered question is the only caller today.
 */
export function forgetPlatformDeclines(): void {
  declinedThisSession.clear();
}

/** Whether `account` declined the question. An unknown account — the session
 *  probe has not answered yet — has declined nothing, which is the reading
 *  that asks rather than the one that hides. */
function platformDeclined(account: string | null): boolean {
  if (account === null) {
    return false;
  }
  // Storage blocked and nothing declined in this tab either: the question is
  // asked again, which is the safe reading of not knowing.
  return (
    declinedThisSession.has(account) || readStored(declinedKey(account)) === "1"
  );
}

// Who is watching the decline: the gate on this screen and the act that
// stands in front of it. Storage has no change event in the tab that wrote
// it, so the write tells them itself.
const declinedListeners = new Set<() => void>();

function subscribeDeclined(listener: () => void): () => void {
  declinedListeners.add(listener);
  return () => declinedListeners.delete(listener);
}

function rememberPlatformDeclined(account: string | null): void {
  if (account === null) {
    return;
  }
  // Recorded here FIRST, and read back first: a browser that refuses storage
  // still has to honour the answer for as long as the tab is open. Writing
  // only to storage meant a private window asked the question again on the
  // very next render, which is the step reappearing under the reader.
  declinedThisSession.add(account);
  writeStored(declinedKey(account), "1");
  for (const listener of declinedListeners) {
    listener();
  }
}

/** The signed-in account the decline belongs to, or null while the session
 *  probe is still answering. */
function useAccount(): string | null {
  const me = useMe();
  return me.data?.user.id ?? null;
}

/**
 * Whether this account declined the platform question in this browser, live:
 * the answer every caller of `outstandingStep` passes it, so the gate and the
 * act in front of it re-read the same fact the moment it changes.
 */
export function usePlatformDeclined(): boolean {
  const account = useAccount();
  return useSyncExternalStore(
    subscribeDeclined,
    () => platformDeclined(account),
    () => false,
  );
}

/** Records the decline against the account that gave it. Its own hook so the
 *  gate does not have to hold the account itself to hand it back. */
export function useRememberPlatformDeclined(): () => void {
  const account = useAccount();
  return () => rememberPlatformDeclined(account);
}
