// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { type BindingKeys, goToBindingFor, shortcutTip } from "../lib/keymap";
import { useSingleKeyShortcuts } from "../lib/shortcuts";
import type { NavTrailLevel } from "./nav";

export type TipState = Readonly<{
  collapsed: boolean;
  tip: string | null;
  onTip: (key: string | null, by?: "pointer" | "focus") => void;
}>;

/** A destination's go-to keys while the reader keeps single-key shortcuts on.
 *  Only the destinations level has them. */
function useGoToKeys(
  level: NavTrailLevel,
  id: string,
): BindingKeys | undefined {
  const on = useSingleKeyShortcuts();
  const bound = level.path.length === 0 ? goToBindingFor(id) : undefined;
  return on ? bound?.keys : undefined;
}

/**
 * A row's tip: the label while the rail is collapsed, and the go-to keys
 * whenever the row has them. A row with neither raises no tip.
 */
export function useRowTip(
  state: TipState,
  row: Readonly<{
    key: string;
    level: NavTrailLevel;
    id: string;
    label: string;
  }>,
) {
  const t = useT();
  const keys = useGoToKeys(row.level, row.id);
  const label = state.collapsed ? row.label : undefined;
  const text = keys
    ? shortcutTip(label, keys, t("shell.shortcuts.then"))
    : (label ?? null);
  return {
    text,
    raise: (by: "pointer" | "focus") => {
      if (text !== null) {
        state.onTip(row.key, by);
      }
    },
  };
}
