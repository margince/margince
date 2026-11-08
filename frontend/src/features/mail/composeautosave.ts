// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import { useBeforeUnload } from "../../components/unsaved";
import type { MessageKey } from "../../i18n/en";
import { connectivityNow, subscribeConnectivity } from "../../lib/connectivity";
import { ProblemError, problemCodeOf } from "../../lib/problem";
import type {
  SaveAsk,
  SavedDraftAnchor,
  SavedDraftFields,
} from "./composesaveddraftcalls";

// How long the words rest before the draft keeps them.
const AUTOSAVE_MS = 1000;

/**
 * Keeps the draft once the reader pauses: while `arm` holds, each change to
 * `key` restarts a short wait, and the save then writes what is on screen.
 * While `leave` holds, a page going away writes at once, with a request the
 * browser finishes after the page has gone.
 *
 * The ask is read when the write leaves, so it carries the newest words and
 * the newest version.
 */
function useAutosave(
  input: Readonly<{
    arm: boolean;
    leave: boolean;
    key: string;
    save: (ask: SaveAsk) => void;
    askOf: () => SaveAsk | null;
  }>,
) {
  const { arm, leave, key, save } = input;
  const latest = useRef(input.askOf);
  useEffect(() => {
    latest.current = input.askOf;
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: each change to the words restarts the pause the save waits for
  useEffect(() => {
    if (!arm) return;
    const timer = setTimeout(() => {
      const ask = latest.current();
      if (ask) save(ask);
    }, AUTOSAVE_MS);
    return () => clearTimeout(timer);
  }, [arm, key, save]);
  useEffect(() => {
    if (!leave) return;
    const flush = () => {
      const ask = latest.current();
      if (ask) save({ ...ask, keepalive: true });
    };
    globalThis.addEventListener("pagehide", flush);
    return () => globalThis.removeEventListener("pagehide", flush);
  }, [leave, save]);
}

function useOnReconnect(active: boolean, onReconnect: () => void) {
  const latest = useRef(onReconnect);
  useEffect(() => {
    latest.current = onReconnect;
  });
  useEffect(() => {
    if (!active) return;
    return subscribeConnectivity(() => {
      if (connectivityNow() === "online") latest.current();
    });
  }, [active]);
}

type DraftSave = Readonly<{
  isPending: boolean;
  isError: boolean;
  error: unknown;
  variables: SaveAsk | undefined;
  mutate: (ask: SaveAsk) => void;
}>;

/** What the composer hands the hook that keeps its draft. */
export type DraftKeeper = Readonly<{
  /** Where the draft is kept; a save attempt counts only at this anchor. */
  anchor: SavedDraftAnchor | null;
  /** The saved draft has been read, so a save cannot race the restore. */
  read: boolean;
  /** Another window moved the draft, and the reader has not chosen yet. */
  conflicted: boolean;
  /** What is on screen is not what the server holds. */
  unsaved: boolean;
  /** Words on screen that nothing has kept, with or without an anchor. */
  unkept: boolean;
  save: DraftSave;
  fields: SavedDraftFields;
  askOf: (quiet: boolean) => SaveAsk | null;
}>;

// A version conflict is no refusal to retry: it waits for the reader's choice
// of which words win.
export function saveOutcome(save: DraftSave) {
  const failed = save.isError && problemCodeOf(save.error) !== "version_skew";
  // A quiet save failed under the reader's hand, so a close still owes them
  // one try before it may discard the words.
  const quietly = failed && save.variables?.quiet === true;
  const failure: MessageKey | null = !failed
    ? null
    : !quietly
      ? "compose.savedDraftFailed"
      : unanswered(save.error)
        ? "compose.savedDraftAutosaveFailed"
        : "compose.savedDraftAutosaveRefused";
  return { failed, quietly, failure };
}

/** Writes the fields on screen now, unless a save is already out. */
export function writeKept(keeper: DraftKeeper, quiet: boolean) {
  const ask = keeper.save.isPending ? null : keeper.askOf(quiet);
  if (ask) keeper.save.mutate(ask);
}

// No answer came, so the connection's return is worth a try. An answer, even
// a 5xx, leaves the connection as it was and waits for an edit or Retry.
function unanswered(error: unknown): boolean {
  return !(error instanceof ProblemError);
}

// A send in flight holds every save, because the send discards the draft a
// save would create and nothing would discard it after.
export function useKeptDraft(keeper: DraftKeeper, sending: boolean) {
  const { save } = keeper;
  const outcome = saveOutcome(save);
  const onScreen = JSON.stringify([keeper.anchor, keeper.fields]);
  // Each version of the words is tried once: a refusal waits for an edit,
  // and a differing answer starts no second save.
  const tried = save.variables
    ? JSON.stringify([save.variables.anchor, save.variables.fields])
    : null;
  const writable =
    keeper.unsaved && !keeper.conflicted && !sending && !save.isPending;
  useAutosave({
    arm: writable && keeper.read && tried !== onScreen,
    leave: writable,
    key: onScreen,
    save: save.mutate,
    askOf: () => keeper.askOf(true),
  });
  useOnReconnect(
    outcome.failed && unanswered(save.error) && writable && keeper.read,
    () => writeKept(keeper, true),
  );
  useBeforeUnload(keeper.unkept);
}
