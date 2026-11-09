// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The tab strip a long settings page draws above its parts.
//
// Which tabs exist and who may open each is the catalog's (`tabs` on a page
// row). This file only names them and draws the strip, so the rail, search and
// the page resolve one table.

import { RecordTabs } from "../../design-system/recordtabs";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { navigate } from "../../lib/router";
import type { SettingsPage, SettingsTabRow } from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";

type TabbedPage = Extract<SettingsPage, { tabs: unknown }>;
type TabbedPageId = TabbedPage["id"];
type TabId<P extends TabbedPageId> = Extract<
  TabbedPage,
  { id: P }
>["tabs"][number]["id"];

/**
 * The words on each tab. A tab missing here is a TypeScript error, so the
 * strip cannot fall behind the catalog.
 */
const TAB_LABELS: {
  readonly [P in TabbedPageId]: Record<TabId<P>, MessageKey>;
} = {
  meetings: {
    links: "settings.part.meetings.links",
    availability: "settings.part.meetings.availability",
    guests: "settings.part.meetings.guests",
  },
  connections: {
    mail: "settings.part.connections.mail",
    addresses: "settings.part.connections.addresses",
    linkedin: "settings.part.connections.linkedin",
    accounts: "settings.part.connections.accounts",
    activity: "settings.part.connections.activity",
  },
  company: {
    profile: "settings.part.company.profile",
    entities: "settings.part.company.entities",
    regional: "settings.part.company.regional",
    money: "settings.part.company.money",
    email: "settings.part.company.email",
  },
  authentication: {
    methods: "settings.part.authentication.methods",
    apps: "settings.part.authentication.apps",
    grants: "settings.part.authentication.grants",
  },
  deals: {
    pipelines: "settings.part.deals.pipelines",
    sources: "settings.part.deals.sources",
    loss: "settings.part.deals.loss",
    reviews: "settings.part.deals.reviews",
    legal: "settings.part.deals.legal",
    forecast: "settings.part.deals.forecast",
  },
  records: {
    fields: "settings.part.records.fields",
    tags: "settings.part.records.tags",
    roles: "settings.part.records.roles",
  },
  capture: {
    sharing: "settings.part.capture.sharing",
    domains: "settings.part.capture.domains",
    enrichment: "settings.part.capture.enrichment",
    internal: "settings.part.capture.internal",
  },
  integrations: {
    contact: "settings.part.integrations.contact",
    accounting: "settings.part.integrations.accounting",
    webhooks: "settings.part.integrations.webhooks",
    telegram: "settings.part.integrations.telegram",
    units: "settings.part.integrations.units",
  },
  models: {
    providers: "settings.part.models.providers",
    tiers: "settings.part.models.tiers",
    tasks: "settings.part.models.tasks",
    prices: "settings.part.models.prices",
  },
  usage: {
    allowance: "settings.part.usage.allowance",
    agents: "settings.part.usage.agents",
    spend: "settings.part.usage.spend",
  },
  automations: {
    mine: "settings.part.automations.mine",
    library: "settings.part.automations.library",
  },
  privacy: {
    consent: "settings.part.privacy.consent",
    customers: "settings.part.privacy.customers",
    retention: "settings.part.privacy.retention",
    requests: "settings.part.privacy.requests",
    duties: "settings.part.privacy.duties",
  },
  "system-health": {
    overview: "health.overview",
    diagnostics: "health.diagnostics",
    configuration: "health.configuration",
  },
};

// The same table read by a page id off the address bar.
const LABELS_BY_PAGE: Readonly<
  Record<string, Readonly<Record<string, MessageKey>> | undefined>
> = TAB_LABELS;

/** The label key of one tab, for search and the strip alike. */
export function settingsTabLabel(
  page: string,
  tab: string,
): MessageKey | undefined {
  return Object.hasOwn(LABELS_BY_PAGE, page)
    ? LABELS_BY_PAGE[page]?.[tab]
    : undefined;
}

/**
 * The strip. It routes rather than switching state, so each tab is an address
 * a reader can share and the unsaved guard sees every move between them.
 */
export function SettingsTabStrip({
  page,
  tabs,
  current,
}: Readonly<{
  page: SettingsPage;
  tabs: readonly SettingsTabRow[];
  current: string;
}>) {
  const t = useT();
  const ids = tabs.map((tab) => tab.id);
  const first = ids[0];
  if (first === undefined || ids.length < 2) return null;
  const labels = Object.fromEntries(
    ids.map((id) => {
      const key = settingsTabLabel(page.id, id);
      return [id, key === undefined ? id : t(key)];
    }),
  );
  return (
    <RecordTabs
      options={ids}
      value={current}
      labels={labels}
      label={t("settings.partsLabel")}
      onChange={(next) =>
        navigate(settingsHref(page.id, next === first ? undefined : next))
      }
    />
  );
}
