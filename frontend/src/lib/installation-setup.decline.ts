// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import type { components } from "../api/schema";
import { useMe } from "./session";
import { readStored, removeStored, STORAGE_KEYS, writeStored } from "./storage";

/**
 * Where "Not now" on a first-run question is remembered.
 *
 * Each question is asked of the contact running the cold start, once. The
 * server has no word for "asked and declined" — a step is simply unconfigured
 * until it is answered, from here or from Settings — so the decline lives in
 * this browser.
 *
 * KEYED BY THE ACCOUNT THAT GAVE IT. A mark keyed on the browser alone outlives
 * the installation it was about: a machine that had run one cold start carried
 * that answer into the next, and the second installation's setup skipped the
 * question with nothing on screen to say why. A re-claimed installation mints
 * its own administrator, so its cold start asks again; the same contact on the
 * same installation is still asked once.
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

// The account:step pairs declined in THIS tab, whether or not storage kept it.
const declinedThisSession = new Set<string>();

/**
 * Forgets the in-tab declines — the counterpart to `localStorage.clear()`,
 * and needed for the same reason: this set is the half of the answer that
 * storage did not keep, so clearing one without the other leaves a decline
 * standing that the caller believes they erased.
 */
export function forgetSetupDeclines(): void {
  declinedThisSession.clear();
}

/** Whether `account` declined `step`. An unknown account — the session probe
 *  has not answered yet — has declined nothing, which is the reading that asks
 *  rather than the one that hides. */
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
  // Recorded here FIRST, and read back first: a browser that refuses storage
  // still has to honour the answer for as long as the tab is open.
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

/**
 * The steps this account declined in this browser, live: the answer every
 * caller of `outstandingStep` passes it, so the gate and the act in front of it
 * re-read the same fact the moment it changes.
 */
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
