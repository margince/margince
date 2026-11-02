// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { Kbd } from "../design-system/atoms";
import {
  DECK_COMMIT_KEY,
  DECK_UNSTAGE_KEY,
  DECK_VERDICT_KEYS,
} from "../design-system/decisiondeck.verdicts";
import { Eyebrow } from "../design-system/eyebrow";
import { FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { DECK_KEY_WORDS } from "../features/brief/brief.deckwords";
import { type Translator, useT } from "../i18n";
import { DISMISS_KEY, sidebarHotkeyCaps } from "./hotkeys";
import { paletteHotkeyCaps } from "./palette";

export type Shortcut = Readonly<{
  id: string;
  label: string;
  caps: readonly string[];
}>;

export type ShortcutGroup = Readonly<{
  id: string;
  title: string;
  hint?: string;
  shortcuts: readonly Shortcut[];
}>;

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

/**
 * Every shortcut the product binds, read from the definitions its handlers
 * read. Nothing here names a key of its own.
 */
export function shortcutGroups(
  t: Translator,
  platform: string,
): readonly ShortcutGroup[] {
  return [
    {
      id: "anywhere",
      title: t("shell.shortcuts.anywhere"),
      shortcuts: [
        {
          id: "search",
          label: t("shell.shortcuts.search"),
          caps: paletteHotkeyCaps(platform),
        },
        {
          id: "sidebar",
          label: t("shell.shortcuts.sidebar"),
          caps: sidebarHotkeyCaps(platform),
        },
        {
          id: "dismiss",
          label: t("shell.shortcuts.dismiss"),
          caps: [capOf(DISMISS_KEY)],
        },
      ],
    },
    {
      id: "deck",
      title: t("brief.panel.decisions"),
      hint: t("shell.shortcuts.deckHint"),
      shortcuts: [
        ...DECK_VERDICT_KEYS.map((bound) => ({
          id: bound.verdict,
          label: t(DECK_KEY_WORDS[bound.verdict]),
          caps: [capOf(bound.key)],
        })),
        {
          id: "unstage",
          label: t(DECK_KEY_WORDS.unstage),
          caps: [capOf(DECK_UNSTAGE_KEY)],
        },
        {
          id: "commit",
          label: t(DECK_KEY_WORDS.commit),
          caps: [capOf(DECK_COMMIT_KEY)],
        },
      ],
    },
  ];
}

/**
 * The keyboard shortcuts, opened from Help and feedback.
 *
 * `form`, not `confirm`: `confirm` asks before an act and carries a verb pair,
 * and this asks nothing. It is a short surface a reader opens, reads and leaves.
 */
export function ShortcutsDialog({
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
      returnFocusTo={returnFocusTo}
    >
      <Heading size="large" id={titleId} className="modal-title">
        {t("shell.help.shortcuts")}
      </Heading>
      <div className="acctshortcuts">
        {shortcutGroups(t, navigator.platform).map((group) => (
          <section key={group.id} className="acctshortcutsgroup">
            <Eyebrow as="h3" size="xxsmall">
              {group.title}
            </Eyebrow>
            {group.hint && <p className="t-caption">{group.hint}</p>}
            <FactList
              facts={group.shortcuts.map((shortcut) => ({
                key: shortcut.id,
                term: shortcut.label,
                value: (
                  <span className="acctkeys">
                    {shortcut.caps.map((cap) => (
                      <Kbd key={cap}>{cap}</Kbd>
                    ))}
                  </span>
                ),
              }))}
            />
          </section>
        ))}
      </div>
    </Modal>
  );
}
