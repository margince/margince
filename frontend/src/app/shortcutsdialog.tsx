// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, useId } from "react";
import { Kbd } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
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
import type { MessageKey } from "../i18n/en";
import { type BindingGroup, bindingCaps, capOf, KEYMAP } from "../lib/keymap";
import { useSingleKeyShortcuts } from "../lib/shortcuts";

/** One way to press a shortcut: a chord's keys together, or a sequence's in turn. */
type ShortcutWay = Readonly<{
  caps: readonly string[];
  sequence: boolean;
}>;

type Shortcut = Readonly<{
  id: string;
  label: string;
  ways: readonly ShortcutWay[];
}>;

type ShortcutGroup = Readonly<{
  id: string;
  title: string;
  hint?: string;
  shortcuts: readonly Shortcut[];
}>;

const GROUP_WORDS: Readonly<
  Record<BindingGroup, Readonly<{ title: MessageKey; hint?: MessageKey }>>
> = {
  anywhere: { title: "shell.shortcuts.anywhere" },
  goto: { title: "shell.shortcuts.goto", hint: "shell.shortcuts.gotoHint" },
  record: { title: "shell.shortcuts.record" },
  editing: { title: "shell.shortcuts.editing" },
};

const GROUP_ORDER: readonly BindingGroup[] = [
  "anywhere",
  "goto",
  "record",
  "editing",
];

const pressedAlone = (key: string): readonly ShortcutWay[] => [
  { caps: [capOf(key)], sequence: false },
];

/** A keymap group as rows; bindings that share a label are one row with each way. */
function keymapGroup(group: BindingGroup, t: Translator): ShortcutGroup {
  const rows = new Map<MessageKey, Shortcut>();
  for (const entry of KEYMAP) {
    if (entry.group !== group) {
      continue;
    }
    const way: ShortcutWay = {
      caps: bindingCaps(entry.keys),
      sequence: entry.keys.kind === "sequence",
    };
    const row = rows.get(entry.labelKey);
    rows.set(
      entry.labelKey,
      row
        ? { ...row, ways: [...row.ways, way] }
        : { id: entry.id, label: t(entry.labelKey), ways: [way] },
    );
  }
  const words = GROUP_WORDS[group];
  return {
    id: group,
    title: t(words.title),
    hint: words.hint && t(words.hint),
    shortcuts: [...rows.values()],
  };
}

/**
 * The shortcuts the dialog lists: the keymap the dispatcher reads, then the
 * deck's keys from the definitions its own handler reads.
 */
function shortcutGroups(t: Translator): readonly ShortcutGroup[] {
  return [
    ...GROUP_ORDER.map((group) => keymapGroup(group, t)),
    {
      id: "deck",
      title: t("brief.panel.decisions"),
      hint: t("shell.shortcuts.deckHint"),
      shortcuts: [
        ...DECK_VERDICT_KEYS.map((bound) => ({
          id: bound.verdict,
          label: t(DECK_KEY_WORDS[bound.verdict]),
          ways: pressedAlone(bound.key),
        })),
        {
          id: "unstage",
          label: t(DECK_KEY_WORDS.unstage),
          ways: pressedAlone(DECK_UNSTAGE_KEY),
        },
        {
          id: "commit",
          label: t(DECK_KEY_WORDS.commit),
          ways: pressedAlone(DECK_COMMIT_KEY),
        },
      ],
    },
  ];
}

function ShortcutKeys({ ways }: Readonly<{ ways: readonly ShortcutWay[] }>) {
  const t = useT();
  return (
    <span className="acctkeys">
      {ways.map((way, at) => (
        <Fragment key={way.caps.join(" ")}>
          {at > 0 && (
            <span className="t-caption">{t("shell.shortcuts.or")}</span>
          )}
          {way.caps.map((cap, step) => (
            <Fragment key={cap}>
              {way.sequence && step > 0 && (
                <span className="t-caption">{t("shell.shortcuts.then")}</span>
              )}
              <Kbd>{cap}</Kbd>
            </Fragment>
          ))}
        </Fragment>
      ))}
    </span>
  );
}

/**
 * The keyboard shortcuts, opened from Help and feedback or with `?`.
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
  const singleKeys = useSingleKeyShortcuts();
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
        {!singleKeys && (
          <Callout
            tone="info"
            kind="standing"
            title={t("shell.shortcuts.singleKeyOff")}
          >
            {t("shell.shortcuts.singleKeyOffHint")}
          </Callout>
        )}
        {shortcutGroups(t).map((group) => (
          <section key={group.id} className="acctshortcutsgroup">
            <Eyebrow as="h3" size="xxsmall">
              {group.title}
            </Eyebrow>
            {group.hint && <p className="t-caption">{group.hint}</p>}
            <FactList
              facts={group.shortcuts.map((shortcut) => ({
                key: shortcut.id,
                term: shortcut.label,
                value: <ShortcutKeys ways={shortcut.ways} />,
              }))}
            />
          </section>
        ))}
      </div>
    </Modal>
  );
}
