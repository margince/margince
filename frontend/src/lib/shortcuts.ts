// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import { isChordOf, isLetterKey, pressedLetter } from "./chord";
import { screenEnabled, useDisabledCoreFeatures } from "./corefeatures";
import { deviceSwitch, useDeviceSwitch } from "./devicepreference";
import { isImeCommit } from "./imecommit";
import { characterKeyRefused, isTextEntry } from "./keyguard";
import {
  ariaKeyShortcuts,
  type Binding,
  binding,
  isCharacterKey,
  KEYMAP,
  SEQUENCE_WINDOW_MS,
  type ShortcutId,
} from "./keymap";
import { navigate } from "./router";
import { STORAGE_KEYS } from "./storage";

// WCAG 2.1.4: a reader can turn off every shortcut that is one character key.
const singleKeys = deviceSwitch(STORAGE_KEYS.singleKeyShortcuts);

export const singleKeyShortcutsOn = singleKeys.read;
export const setSingleKeyShortcutsOn = singleKeys.write;

export function useSingleKeyShortcuts(): boolean {
  return useDeviceSwitch(singleKeys);
}

/** A binding's `aria-keyshortcuts`, absent while the reader has it turned off. */
export function useShortcutAria(id: ShortcutId): string | undefined {
  const on = useSingleKeyShortcuts();
  const keys = binding(id).keys;
  return on || !isCharacterKey(keys) ? ariaKeyShortcuts(keys) : undefined;
}

type Slot = Readonly<{ run: () => void }>;

const slots = new Map<ShortcutId, Slot[]>();

function mountShortcutHandler(id: ShortcutId, run: () => void): () => void {
  const slot: Slot = { run };
  slots.set(id, [...(slots.get(id) ?? []), slot]);
  return () => {
    slots.set(
      id,
      (slots.get(id) ?? []).filter((mounted) => mounted !== slot),
    );
  };
}

/** What a press of `id` runs: the handler mounted last, or nothing. */
function shortcutHandler(id: ShortcutId): (() => void) | undefined {
  return slots.get(id)?.at(-1)?.run;
}

/** Answer a binding while this component is mounted and `enabled`. */
export function useShortcutHandler(
  id: ShortcutId,
  handler: () => void,
  enabled = true,
): void {
  const latest = useRef(handler);
  useEffect(() => {
    latest.current = handler;
  });
  useEffect(
    () =>
      enabled ? mountShortcutHandler(id, () => latest.current()) : undefined,
    [id, enabled],
  );
}

/** Answer `id` while `enabled`, and give its `aria-keyshortcuts` then only, so
 *  a control never names a key that does nothing. */
export function useShortcut(
  id: ShortcutId,
  handler: () => void,
  enabled = true,
): string | undefined {
  useShortcutHandler(id, handler, enabled);
  const keys = useShortcutAria(id);
  return enabled ? keys : undefined;
}

/** Answer each go-to binding whose screen this installation has turned on. */
export function useGoToShortcuts(): void {
  const disabled = useDisabledCoreFeatures();
  useEffect(() => {
    const unmount = KEYMAP.flatMap((entry) => {
      const target = entry.goesTo;
      return target && screenEnabled(target.screen, disabled)
        ? [mountShortcutHandler(entry.id, () => navigate(target))]
        : [];
    });
    return () => {
      for (const release of unmount) {
        release();
      }
    };
  }, [disabled]);
}

const DISPATCHED = KEYMAP.filter((entry) => entry.answeredBy === "dispatcher");

function chordPressed(event: KeyboardEvent): Binding | undefined {
  return DISPATCHED.find(
    (entry) => entry.keys.kind === "chord" && isChordOf(event, entry.keys.key),
  );
}

/** A letter is matched by place and refused with Shift; `?` and `/` as typed. */
function characterPressed(event: KeyboardEvent): string | null {
  if (!isLetterKey(event.key) && event.key.length === 1) {
    const typed = DISPATCHED.some(
      (entry) => entry.keys.kind === "single" && entry.keys.key === event.key,
    );
    if (typed) {
      return event.key;
    }
  }
  return event.shiftKey ? null : pressedLetter(event);
}

type ShortcutDispatcher = Readonly<{
  onKeyDown: (event: KeyboardEvent) => void;
  /** Forget a sequence's first key, as when nothing listens any more. */
  reset: () => void;
}>;

/** The one place a key press becomes a binding. A sequence's window runs on
 *  `setTimeout`, so fake timers drive it in a test. */
export function createShortcutDispatcher(
  options: Readonly<{
    singleKeysOn: () => boolean;
    handlerFor?: (id: ShortcutId) => (() => void) | undefined;
  }>,
): ShortcutDispatcher {
  const handlerFor = options.handlerFor ?? shortcutHandler;
  let lead: { key: string; expiry: ReturnType<typeof setTimeout> } | null =
    null;

  const dropLead = () => {
    if (lead) {
      clearTimeout(lead.expiry);
      lead = null;
    }
  };
  const run = (entry: Binding | undefined, event: KeyboardEvent): boolean => {
    const handler = entry ? handlerFor(entry.id) : undefined;
    if (!handler) {
      return false;
    }
    event.preventDefault();
    handler();
    return true;
  };
  const sequenceAfter = (first: string, second: string) =>
    DISPATCHED.find(
      (entry) =>
        entry.keys.kind === "sequence" &&
        entry.keys.keys[0] === first &&
        entry.keys.keys[1] === second,
    );
  const leadsAnswered = (key: string) =>
    DISPATCHED.some(
      (entry) =>
        entry.keys.kind === "sequence" &&
        entry.keys.keys[0] === key &&
        handlerFor(entry.id) !== undefined,
    );

  const onCharacter = (event: KeyboardEvent) => {
    const pending = lead?.key;
    dropLead();
    const key = characterPressed(event);
    if (key === null) {
      return;
    }
    if (pending !== undefined && run(sequenceAfter(pending, key), event)) {
      return;
    }
    if (leadsAnswered(key)) {
      event.preventDefault();
      lead = { key, expiry: setTimeout(dropLead, SEQUENCE_WINDOW_MS) };
      return;
    }
    run(
      DISPATCHED.find(
        (entry) => entry.keys.kind === "single" && entry.keys.key === key,
      ),
      event,
    );
  };

  const onKeyDown = (event: KeyboardEvent) => {
    if (
      event.defaultPrevented ||
      event.repeat ||
      isImeCommit({ nativeEvent: event })
    ) {
      return;
    }
    const chord = chordPressed(event);
    if (chord) {
      if (!(chord.yieldsToFields && isTextEntry(event.target))) {
        run(chord, event);
      }
      return;
    }
    if (!options.singleKeysOn() || characterKeyRefused(event)) {
      dropLead();
      return;
    }
    onCharacter(event);
  };

  return { onKeyDown, reset: dropLead };
}

const appDispatcher = createShortcutDispatcher({
  singleKeysOn: singleKeyShortcutsOn,
});
let listening = 0;

/**
 * Listen for every binding while this component is mounted. However many
 * mount it, the window hears one listener, so no press runs twice.
 */
export function useShortcutDispatcher(): void {
  useEffect(() => {
    listening += 1;
    if (listening === 1) {
      globalThis.addEventListener("keydown", appDispatcher.onKeyDown);
    }
    return () => {
      listening -= 1;
      if (listening === 0) {
        globalThis.removeEventListener("keydown", appDispatcher.onKeyDown);
        appDispatcher.reset();
      }
    };
  }, []);
}
