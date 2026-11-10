import { useMemo } from "react";
import { SCHEDULED_SCREEN } from "../features/mail/scheduledqueue";
import {
  useOpensSettingsTab,
  useVisibleSettingsPages,
} from "../features/settings/settingsnav";
import { settingsPageKeywords } from "../features/settings/settingssearch";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCompanyContextShown } from "../lib/companycontextreach";
import { type EntityKind, SCREEN_ENTITY } from "../lib/entity";
import { settingsHref } from "../lib/settingsrouting";
import {
  CUSTOM_SCREEN,
  customPaletteScreens,
  resolveCustomLabel,
} from "./custom";
import { CREATE_ID, CREATE_SCREENS, NAV } from "./nav";
import type { Command } from "./palette";
import { settingsCommandId } from "./paletteconfiguration";

// Each kind's "New …" label; the commands follow `CREATE_SCREENS`' order.
const CREATE_LABEL: Readonly<Record<EntityKind, MessageKey>> = {
  contact: "create.contact",
  company: "create.company",
  lead: "create.lead",
  deal: "action.newDeal",
  project: "project.new",
};

export function useBuiltinCommands(): Command[] {
  const t = useT();
  const { locale } = useLocale();
  const visible = useVisibleSettingsPages();
  const seesCompanyContext = useCompanyContextShown();
  const opensTab = useOpensSettingsTab();
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
      ...[...CREATE_SCREENS].flatMap((screen) => {
        const entity = SCREEN_ENTITY[screen];
        return entity
          ? [
              {
                id: `action:new-${entity}`,
                label: t(CREATE_LABEL[entity]),
                type: "action" as const,
                route: { screen, id: CREATE_ID },
              },
            ]
          : [];
      }),
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
        ...settingsPageKeywords(page.id, t, { seesCompanyContext, opensTab }),
        ...(page.id === "company" && seesCompanyContext
          ? [t("settings.companyRefresh")]
          : []),
      ],
      type: "setting",
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
  }, [t, visible, locale, seesCompanyContext, opensTab]);
}
