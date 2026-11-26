// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { useHoverIntent } from "../design-system/hoverintent";
import { useT } from "../i18n";
import { ariaKeyShortcuts, binding, shortcutTip } from "../lib/keymap";
import type { TipState } from "./navrowtip";
import { NavTip } from "./navtip";

const TOGGLE_TIP_KEY = "rail-toggle";

/** Folds the sidebar; folded, its name and tip say what a press does. */
export function NavRailToggle({
  state,
  onToggle,
}: Readonly<{ state: TipState; onToggle: () => void }>) {
  const t = useT();
  const label = state.collapsed ? t("shell.expand") : t("shell.collapse");
  const keys = binding("sidebar").keys;
  const hover = useHoverIntent(
    () => state.onTip(TOGGLE_TIP_KEY, "pointer"),
    () => state.onTip(null),
  );
  return (
    <div className="navwrap railfoot">
      <button
        type="button"
        className="navitem navtoggle t-body"
        aria-label={label}
        aria-expanded={!state.collapsed}
        aria-keyshortcuts={ariaKeyShortcuts(keys)}
        onClick={onToggle}
        onPointerEnter={hover.onPointerEnter}
        onPointerLeave={hover.onPointerLeave}
        onFocus={() => state.onTip(TOGGLE_TIP_KEY, "focus")}
        onBlur={() => state.onTip(null)}
      >
        {state.collapsed ? (
          <PanelLeftOpen aria-hidden />
        ) : (
          <PanelLeftClose aria-hidden />
        )}
        <span className="navlabel">{t("shell.collapse")}</span>
        {state.collapsed && state.tip === TOGGLE_TIP_KEY && (
          <NavTip>{shortcutTip(label, keys, t)}</NavTip>
        )}
      </button>
    </div>
  );
}
