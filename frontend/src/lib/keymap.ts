// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { FIELD_COMMIT_KEY } from "./fieldcommit";
import type { Route, Screen } from "./router";
import { settingsHref } from "./settingsrouting";

/** `chord` takes ⌘ or Ctrl; `named` is a key the platform answers, listed and never dispatched. */
export type BindingKeys =
  | Readonly<{ kind: "chord"; key: string }>
  | Readonly<{ kind: "single"; key: string }>
  | Readonly<{ kind: "sequence"; keys: readonly [string, string] }>
  | Readonly<{ kind: "named"; key: string }>;

export type BindingGroup = "anywhere" | "goto" | "record" | "editing";

type BindingRow = Readonly<{
  id: string;
  keys: BindingKeys;
  group: BindingGroup;
  labelKey: MessageKey;
  /** `control`: the focused control answers it, and the dispatcher never does. */
  answeredBy: "dispatcher" | "control";
  /** A chord a text field keeps for itself: ⌘B is bold in every editor. */
  yieldsToFields?: boolean;
  goesTo?: Route;
}>;

export const SEQUENCE_WINDOW_MS = 1000;

const GO_TO_LEAD = "g";

const chord = (key: string) => ({ kind: "chord", key }) as const;
const single = (key: string) => ({ kind: "single", key }) as const;
const goTo = (key: string) =>
  ({ kind: "sequence", keys: [GO_TO_LEAD, key] }) as const;

/** The rail's destinations as G, then a letter; each label is the rail's own. */
const GO_TO = [
  ["home", "h", "nav.brief"],
  ["contacts", "c", "nav.contacts"],
  ["companies", "o", "nav.companies"],
  ["leads", "l", "nav.leads"],
  ["deals", "d", "nav.deals"],
  ["projects", "p", "nav.projects"],
  ["filters", "f", "filters.title"],
  ["analytics", "a", "nav.analytics"],
] as const satisfies readonly (readonly [Screen, string, MessageKey])[];

function goToRow<S extends Screen>(
  screen: S,
  letter: string,
  labelKey: MessageKey,
  goesTo: Route = { screen },
) {
  return {
    id: `goto.${screen}`,
    keys: goTo(letter),
    group: "goto",
    labelKey,
    answeredBy: "dispatcher",
    goesTo,
  } as const;
}

/** Every keyboard shortcut the product binds or lists, in the order the dialog lists them. */
const BINDING_ROWS = [
  {
    id: "search",
    keys: chord("k"),
    group: "anywhere",
    labelKey: "shell.shortcuts.search",
    answeredBy: "dispatcher",
  },
  {
    id: "search.slash",
    keys: single("/"),
    group: "anywhere",
    labelKey: "shell.shortcuts.search",
    answeredBy: "dispatcher",
  },
  {
    id: "create",
    keys: single("c"),
    group: "anywhere",
    labelKey: "shell.shortcuts.create",
    answeredBy: "dispatcher",
  },
  {
    id: "sidebar",
    keys: chord("b"),
    group: "anywhere",
    labelKey: "shell.shortcuts.sidebar",
    answeredBy: "dispatcher",
    yieldsToFields: true,
  },
  {
    id: "shortcuts",
    keys: single("?"),
    group: "anywhere",
    labelKey: "shell.shortcuts.list",
    answeredBy: "dispatcher",
  },
  {
    id: "dismiss",
    keys: { kind: "named", key: "Escape" },
    group: "anywhere",
    labelKey: "shell.shortcuts.dismiss",
    answeredBy: "control",
  },
  ...GO_TO.map(([screen, letter, labelKey]) =>
    goToRow(screen, letter, labelKey),
  ),
  goToRow("settings", "s", "nav.settings", settingsHref()),
  {
    id: "record.logActivity",
    keys: single("n"),
    group: "record",
    labelKey: "shell.shortcuts.logActivity",
    answeredBy: "dispatcher",
  },
  {
    id: "record.addTask",
    keys: single("t"),
    group: "record",
    labelKey: "shell.shortcuts.addTask",
    answeredBy: "dispatcher",
  },
  {
    id: "record.writeEmail",
    keys: single("m"),
    group: "record",
    labelKey: "shell.shortcuts.writeEmail",
    answeredBy: "dispatcher",
  },
  {
    id: "editing.paragraphSave",
    keys: chord(FIELD_COMMIT_KEY),
    group: "editing",
    labelKey: "shell.shortcuts.paragraphSave",
    answeredBy: "control",
  },
  {
    id: "editing.sendEmail",
    keys: chord(FIELD_COMMIT_KEY),
    group: "editing",
    labelKey: "shell.shortcuts.sendEmail",
    answeredBy: "control",
  },
  {
    id: "dialog.submit",
    keys: chord(FIELD_COMMIT_KEY),
    group: "editing",
    labelKey: "shell.shortcuts.submitDialog",
    answeredBy: "dispatcher",
  },
] as const satisfies readonly BindingRow[];

export type ShortcutId = (typeof BINDING_ROWS)[number]["id"];

export type Binding = BindingRow & Readonly<{ id: ShortcutId }>;

export const KEYMAP: readonly Binding[] = BINDING_ROWS;

const BY_ID: ReadonlyMap<ShortcutId, Binding> = new Map(
  KEYMAP.map((binding) => [binding.id, binding]),
);

export function binding(id: ShortcutId): Binding {
  const found = BY_ID.get(id);
  if (found === undefined) {
    throw new Error(`no keyboard shortcut is registered as ${id}`);
  }
  return found;
}

/** A shortcut pressed with no modifier, which WCAG 2.1.4 lets a reader turn off. */
export function isCharacterKey(keys: BindingKeys): boolean {
  return keys.kind === "single" || keys.kind === "sequence";
}

export function goToBindingFor(screen: string): Binding | undefined {
  return KEYMAP.find(
    (candidate) =>
      candidate.goesTo?.screen === screen && candidate.goesTo.id === undefined,
  );
}

const APPLE_PLATFORM = /mac|iphone|ipad|ipod/i;

/** The one read of the reader's platform, which picks ⌘ or Ctrl. */
function readerPlatform(): string {
  return globalThis.navigator?.platform ?? "";
}

/** The platform modifier as the cap its keyboard prints. */
function modifierCap(platform: string): string {
  return APPLE_PLATFORM.test(platform) ? "⌘" : "Ctrl";
}

const KEY_CAPS: Readonly<Record<string, string>> = {
  ArrowRight: "→",
  ArrowLeft: "←",
  ArrowUp: "↑",
  ArrowDown: "↓",
  Escape: "Esc",
};

/** A `KeyboardEvent.key` value as the cap a keyboard prints for it. */
export function capOf(key: string): string {
  return KEY_CAPS[key] ?? (key.length === 1 ? key.toUpperCase() : key);
}

export function bindingCaps(
  keys: BindingKeys,
  platform = readerPlatform(),
): readonly string[] {
  switch (keys.kind) {
    case "chord":
      return [modifierCap(platform), capOf(keys.key)];
    case "sequence":
      return keys.keys.map(capOf);
    default:
      return [capOf(keys.key)];
  }
}

export function shortcutCaps(
  id: ShortcutId,
  platform = readerPlatform(),
): readonly string[] {
  return bindingCaps(binding(id).keys, platform);
}

/** A label and its keys as one tip, "Contacts · G then C"; `then` is the
 *  reader's word between a sequence's keys. */
export function shortcutTip(
  label: string | undefined,
  keys: BindingKeys,
  then: string,
): string {
  const caps = bindingCaps(keys).join(
    keys.kind === "sequence" ? ` ${then} ` : " ",
  );
  return label === undefined ? caps : `${label} · ${caps}`;
}

/** None for a sequence: ARIA separates alternatives with a space, so "g c"
 *  would claim that C alone goes there. */
export function ariaKeyShortcuts(keys: BindingKeys): string | undefined {
  switch (keys.kind) {
    case "chord":
      return `Meta+${capOf(keys.key)} Control+${capOf(keys.key)}`;
    case "sequence":
      return undefined;
    default:
      return keys.key;
  }
}
