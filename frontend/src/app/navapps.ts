import { Plug } from "lucide-react";
import { ENTRY_ICONS } from "../components/extensionentries";
import { translate } from "../i18n";
import {
  EXTENSION_APP_SCREEN,
  type ExtensionAppGroup,
  type UnitApp,
} from "../lib/extensionapps";
import type { NavLevelEntry } from "./subnav";

// A composed unit's apps as drawer rows, in the group each declared
// (lib/extensionapps.ts).
//
// Appended after the product's rows, as a fork's are. The caller has already
// dropped the apps this reader may not open, so the entry and the route share one
// decision. The label is built per locale because the level is built once.
export function appItems(
  group: ExtensionAppGroup | undefined,
  apps: readonly UnitApp[],
): readonly NavLevelEntry[] {
  return apps
    .filter((app) => app.group === group)
    .map((app) => ({
      id: app.unit,
      prefix: [EXTENSION_APP_SCREEN],
      label: {
        en: translate("en", app.label),
        de: translate("de", app.label),
        vi: translate("vi", app.label),
      },
      icon: Object.hasOwn(ENTRY_ICONS, app.icon)
        ? (ENTRY_ICONS[app.icon] ?? Plug)
        : Plug,
    }));
}
