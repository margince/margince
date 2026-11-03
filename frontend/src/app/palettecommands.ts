import { useMemo } from "react";
import { SCHEDULED_SCREEN } from "../features/mail/scheduledqueue";
import { useVisibleSettingsPages } from "../features/settings/settingsnav";
import { settingsPageKeywords } from "../features/settings/settingssearch";
import { useLocale, useT } from "../i18n";
import { useCompanyContextShown } from "../lib/companycontextreach";
import { settingsHref } from "../lib/settingsrouting";
import {
  CUSTOM_SCREEN,
  customPaletteScreens,
  resolveCustomLabel,
} from "./custom";
import { CREATE_ID, NAV } from "./nav";
import type { Command } from "./palette";
import { settingsCommandId } from "./paletteconfiguration";

export function useBuiltinCommands(): Command[] {
  const t = useT();
  const { locale } = useLocale();
  const visible = useVisibleSettingsPages();
  const seesCompanyContext = useCompanyContextShown();
  return useMemo(() => {
    const screens: Command[] = NAV.map((item) => ({
      id: `screen:${item.screen}`,
      label: t(item.labelKey),
      keywords: [item.screen, ...(item.aliases ?? [])],
      type: "screen",
      route: { screen: item.screen },
    }));
    const forkScreens: Command[] = customPaletteScreens().map((screen) => ({
      id: `screen:${CUSTOM_SCREEN}/${screen.key}`,
      label: resolveCustomLabel(screen.palette.label, locale, t),
      keywords: [screen.key],
      type: "screen",
      route: { screen: CUSTOM_SCREEN, id: screen.key },
    }));
    const actions: Command[] = [
      {
        id: "action:new-deal",
        label: t("action.newDeal"),
        type: "action",
        route: { screen: "deals", id: CREATE_ID },
      },
      {
        id: "action:booking",
        label: t("action.booking"),
        type: "action",
        route: { screen: "book" },
      },
    ];
    const settingsScreens: Command[] = visible.map((page) => ({
      id: settingsCommandId(page.id),
      label: t(`settings.tab.${page.id}`),
      keywords: [
        page.id,
        ...settingsPageKeywords(page.id, t, { seesCompanyContext }),
        ...(page.id === "company" && seesCompanyContext
          ? [t("settings.companyRefresh")]
          : []),
      ],
      type: "screen",
      route: settingsHref(page.id),
    }));
    const offRailScreens: Command[] = [
      {
        id: `screen:${SCHEDULED_SCREEN}`,
        label: t("nav.scheduled"),
        keywords: [SCHEDULED_SCREEN, t("compose.scheduleSend")],
        type: "screen",
        route: { screen: SCHEDULED_SCREEN },
      },
    ];
    return [
      ...screens,
      ...forkScreens,
      ...actions,
      ...offRailScreens,
      ...settingsScreens,
    ];
  }, [t, visible, locale, seesCompanyContext]);
}
