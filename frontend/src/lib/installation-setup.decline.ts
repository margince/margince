// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import type { components } from "../api/schema";
import { useMe } from "./session";
import { readStored, removeStored, STORAGE_KEYS, writeStored } from "./storage";

/**
 * Where "Not now" on a first-run question is remembered. The server only knows
 * a step as unconfigured, so the decline lives in this browser.
 * It is keyed by account: a re-claimed installation mints a new administrator,
 * so its cold start asks again instead of inheriting an old answer.
 */
export type DeclinableStep = Extract<
  components["schemas"]["InstallationSetupStep"]["step"],
  "ai_models" | "oauth_app"
>;

/** Every first-run question, each of which can be passed with "Not now". */
export const DECLINABLE_STEPS: readonly DeclinableStep[] = [
  "ai_models",
  "oauth_app",
];

function declinedKey(step: DeclinableStep, account: string) {
  const family =
    step === "ai_models"
      ? STORAGE_KEYS.aiDeclined
      : STORAGE_KEYS.platformDeclined;
  return { family, member: account };
}

// The account:step pairs declined in this tab, whether or not storage kept them.
const declinedThisSession = new Set<string>();

/** Forgets the in-tab declines, the counterpart to `localStorage.clear()`.
 *  Clearing only storage would leave a decline standing in this set. */
export function forgetSetupDeclines(): void {
  declinedThisSession.clear();
}

/** Whether `account` declined `step`. An unknown account has declined
 *  nothing, so the question is asked rather than hidden. */
function stepDeclined(step: DeclinableStep, account: string | null): boolean {
  if (account === null) {
    return false;
  }
  // Storage blocked and nothing declined in this tab either: the question is
  // asked again, which is the safe reading of not knowing.
  return (
    declinedThisSession.has(`${account}:${step}`) ||
    readStored(declinedKey(step, account)) === "1"
  );
}

// Who is watching the declines: the gate on the setup screen and the act that
// stands in front of it. Storage has no change event in the tab that wrote it,
// so the write tells them itself.
const declinedListeners = new Set<() => void>();

function subscribeDeclined(listener: () => void): () => void {
  declinedListeners.add(listener);
  return () => declinedListeners.delete(listener);
}

function notify(): void {
  for (const listener of declinedListeners) {
    listener();
  }
}

function rememberDeclined(step: DeclinableStep, account: string | null): void {
  if (account === null) {
    return;
  }
  // Recorded in the tab first, so a browser that refuses storage still honours
  // the answer while the tab is open.
  declinedThisSession.add(`${account}:${step}`);
  writeStored(declinedKey(step, account), "1");
  notify();
}

function withdrawDeclined(step: DeclinableStep, account: string | null): void {
  if (account === null) {
    return;
  }
  declinedThisSession.delete(`${account}:${step}`);
  removeStored(declinedKey(step, account));
  notify();
}

/** The signed-in account a decline belongs to, or null while the session
 *  probe is still answering. */
function useAccount(): string | null {
  const me = useMe();
  return me.data?.user.id ?? null;
}

// One string per answer, because useSyncExternalStore compares snapshots by
// identity and a fresh Set every render would loop.
function declinedSnapshot(account: string | null): string {
  return DECLINABLE_STEPS.filter((step) => stepDeclined(step, account)).join(
    ",",
  );
}

/** The steps this account declined in this browser, live. The gate and the
 *  act in front of it re-read the same fact the moment it changes. */
export function useSetupDeclines(): ReadonlySet<DeclinableStep> {
  const account = useAccount();
  const snapshot = useSyncExternalStore(
    subscribeDeclined,
    () => declinedSnapshot(account),
    () => "",
  );
  return new Set(
    DECLINABLE_STEPS.filter((step) => snapshot.split(",").includes(step)),
  );
}

/** Records and withdraws a decline against the account that gave it. Its own
 *  hook so the gate does not have to hold the account to hand it back. */
export function useSetupDecline(): Readonly<{
  decline: (step: DeclinableStep) => void;
  withdraw: (step: DeclinableStep) => void;
}> {
  const account = useAccount();
  return {
    decline: (step) => rememberDeclined(step, account),
    withdraw: (step) => withdrawDeclined(step, account),
  };
}
