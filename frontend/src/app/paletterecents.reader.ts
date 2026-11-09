// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { fetchEntityReference } from "../components/entityref.queries";
import { problemCodeOf, problemStatus } from "../lib/problem";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  watchStored,
} from "../lib/storage";
import {
  type RecentEntry,
  readRecents,
  recentIdentity,
  writeRecents,
} from "./paletterecents.storage";

export type RecentRecord = Extract<RecentEntry, { kind: "record" }>;
export type ShownRecent = RecentEntry & { logo?: string | null };
type Reading = { stored: RecentEntry | null; shown: ShownRecent | null };
export type RecentState = {
  entries: ShownRecent[];
  hasStored: boolean;
  pending: boolean;
  activating?: string;
};
const key = STORAGE_KEYS.paletteRecents;
const rememberQueues = new Map<string, Promise<void>>();
// Pending session checks must not reorder the searches the reader executed.
async function serializeRemember(seat: string, write: () => Promise<void>) {
  const previous = rememberQueues.get(seat) ?? Promise.resolve();
  let release = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const tail = previous.then(() => gate);
  rememberQueues.set(seat, tail);
  try {
    await previous;
    await write();
  } finally {
    release();
    if (rememberQueues.get(seat) === tail) rememberQueues.delete(seat);
  }
}

const EMPTY: RecentState = { entries: [], hasStored: false, pending: false };
function definitive(error: unknown) {
  return (
    problemCodeOf(error) === "permission_denied" ||
    problemStatus(error) === 403 ||
    problemStatus(error) === 404
  );
}
async function validate(
  entry: RecentEntry,
  signal: AbortSignal,
): Promise<Reading> {
  if (entry.kind === "query") return { stored: entry, shown: entry };
  try {
    const reference = await fetchEntityReference(
      entry.entity_type,
      entry.id,
      signal,
    );
    if (!reference || reference.archived) return { stored: null, shown: null };
    const name = reference.name?.trim();
    if (!name) return { stored: entry, shown: null };
    const fresh = { ...entry, label: name };
    return { stored: fresh, shown: { ...fresh, logo: reference.logo } };
  } catch (error) {
    return definitive(error)
      ? { stored: null, shown: null }
      : { stored: entry, shown: null };
  }
}

// Each opening checks records before exposing their names. Its own storage
// writes retain that reading, including when a browser refuses the write.
export function createRecentReader(
  seat: string | undefined,
  enabled: boolean,
  current: () => boolean,
  verifySeat: (signal: AbortSignal) => Promise<boolean>,
) {
  const initial = enabled && seat ? readRecents(seat).entries : [];
  const pending = initial.some((entry) => entry.kind === "record");
  let state: RecentState = {
    entries: pending
      ? initial.filter((entry) => entry.kind === "query")
      : initial,
    hasStored: initial.length > 0,
    pending,
  };
  let active = false;
  let writing = false;
  let notify = () => {};
  let checking: AbortController | undefined;
  let activation: AbortController | undefined;
  let navigated = false;
  const entries = () => (seat ? readRecents(seat).entries : []);
  const update = (next: RecentState) => {
    state = next;
    notify();
  };
  const write = (kept: RecentEntry[]) => {
    if (!seat) return;
    writing = true;
    try {
      writeRecents(seat, kept);
    } finally {
      writing = false;
    }
  };
  const cancel = () => {
    activation?.abort();
    activation = undefined;
    if (state.activating) update({ ...state, activating: undefined });
  };
  const refresh = () => {
    checking?.abort();
    if (!active || !current()) return;
    const stored = entries();
    const raw = readStored(key);
    const controller = new AbortController();
    checking = controller;
    const pending = stored.some((entry) => entry.kind === "record");
    update({
      entries: pending
        ? stored.filter((entry) => entry.kind === "query")
        : stored,
      hasStored: stored.length > 0,
      pending,
      activating: state.activating,
    });
    void Promise.all(
      stored.map((entry) => validate(entry, controller.signal)),
    ).then((results) => {
      if (
        controller.signal.aborted ||
        !active ||
        !current() ||
        readStored(key) !== raw
      )
        return;
      const kept = results.flatMap((result) =>
        result.stored ? [result.stored] : [],
      );
      if (
        state.activating &&
        !kept.some((entry) => recentIdentity(entry) === state.activating)
      )
        cancel();
      update({
        entries: results.flatMap((result) =>
          result.shown ? [result.shown] : [],
        ),
        hasStored: kept.length > 0,
        pending: false,
        activating: state.activating,
      });
      if (JSON.stringify(stored) !== JSON.stringify(kept)) write(kept);
      const hasStored = entries().length > 0;
      if (state.hasStored !== hasStored) update({ ...state, hasStored });
    });
  };
  return {
    getSnapshot: () => state,
    subscribe(changed: () => void) {
      if (!enabled) return () => {};
      active = true;
      notify = changed;
      const stop = watchStored(key, (change) => {
        if (writing) return;
        if (
          change !== "write" ||
          !entries().some((entry) => recentIdentity(entry) === state.activating)
        )
          cancel();
        refresh();
      });
      refresh();
      return () => {
        active = false;
        notify = () => {};
        checking?.abort();
        activation?.abort();
        stop();
      };
    },
    cancel,
    clear() {
      cancel();
      checking?.abort();
      update(EMPTY);
      writing = true;
      try {
        removeStored(key);
      } finally {
        writing = false;
      }
    },
    async remember(query: string, record?: RecentRecord) {
      if (!seat || !current()) return;
      let cancelled = false;
      const proof = new AbortController();
      const stop = watchStored(key, (change) => {
        if (change !== "write") {
          cancelled = true;
          proof.abort();
        }
      });
      const added: RecentEntry[] = record ? [record] : [];
      if (query.trim()) added.push({ kind: "query", text: query.trim() });
      const hadHistory = readStored(key) !== null;
      const writable = () =>
        !cancelled && current() && (!hadHistory || readStored(key) !== null);
      try {
        await serializeRemember(seat, async () => {
          if (!writable()) return;
          if (!(await verifySeat(proof.signal)) || !writable()) return;
          write([...added, ...entries()]);
          if (active) refresh();
        });
      } finally {
        stop();
      }
    },
    async openRecord(entry: RecentRecord, go: () => void, refused: () => void) {
      const identity = recentIdentity(entry);
      if (navigated || state.activating === identity) return;
      cancel();
      const controller = new AbortController();
      activation = controller;
      update({ ...state, activating: identity });
      const result = await validate(entry, controller.signal);
      const stillActive = () =>
        !controller.signal.aborted &&
        activation === controller &&
        active &&
        current();
      if (!stillActive()) return;
      activation = undefined;
      const latest = entries();
      if (!latest.some((item) => recentIdentity(item) === identity)) {
        update({ ...state, activating: undefined });
        return;
      }
      const rest = latest.filter((item) => recentIdentity(item) !== identity);
      if (result.shown && result.stored) {
        write([result.stored, ...rest]);
        update({ ...state, activating: undefined });
        navigated = true;
        go();
        return;
      }
      if (!result.stored) write(rest);
      update({
        ...state,
        entries: state.entries.filter(
          (item) => recentIdentity(item) !== identity,
        ),
        hasStored: entries().length > 0,
        activating: undefined,
      });
      refused();
    },
  };
}
