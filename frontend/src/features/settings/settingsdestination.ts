// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { navigateReplacing, type Route } from "../../lib/router";
import type { SettingsPage, SettingsTabRow } from "../../lib/settingscatalog";
import { settingsHref, settingsRouteTarget } from "../../lib/settingsrouting";
import {
  usePageReadOnly,
  useVisibleSettingsPages,
  useVisibleSettingsTabs,
} from "./settingsnav";

/** What one settings address shows this reader. */
export type SettingsDestination =
  | { readonly kind: "home" }
  | { readonly kind: "boundary"; readonly boundary: "denied" | "unknown" }
  | {
      readonly kind: "page";
      readonly page: SettingsPage;
      readonly tabs: readonly SettingsTabRow[];
      /** The open tab, or "" on a page without tabs. */
      readonly tab: string;
      /** The reader consults this tab and changes nothing on it. */
      readonly readOnly: boolean;
    };

/**
 * Resolve an address against what this reader may open, and rewrite a legacy
 * spelling to the current one.
 *
 * An address the reader may not open, a tab included, is a boundary with the
 * URL left exactly as typed, so they can quote it to the colleague who has it.
 */
export function useSettingsDestination(route: Route): SettingsDestination {
  const target = settingsRouteTarget(route);
  const visible = useVisibleSettingsPages();
  const named =
    target.kind === "page"
      ? visible.find((page) => page.id === target.page)
      : undefined;
  const tabs = useVisibleSettingsTabs(named);
  const asked = target.kind === "page" ? target.tab : undefined;
  // The first tab the reader may open is the page's bare address.
  const firstTab = tabs[0]?.id;
  const tab = asked ?? firstTab ?? "";
  const opens =
    named !== undefined &&
    (asked === undefined || tabs.some((each) => each.id === asked));
  const readOnly = usePageReadOnly(named?.id, tab);
  // Only a page this reader reached is rewritten to, and replaced rather than
  // pushed, or Back would land on the address that redirects.
  const rewriteTo =
    target.kind === "page" && target.legacy && opens ? named?.id : undefined;
  const rewriteTab = tab === firstTab ? undefined : tab;
  useEffect(() => {
    if (rewriteTo) navigateReplacing(settingsHref(rewriteTo, rewriteTab));
  }, [rewriteTo, rewriteTab]);

  if (target.kind === "home") return { kind: "home" };
  if (target.kind === "unknown")
    return { kind: "boundary", boundary: "unknown" };
  if (!opens || named === undefined)
    return { kind: "boundary", boundary: "denied" };
  return { kind: "page", page: named, tabs, tab, readOnly };
}
