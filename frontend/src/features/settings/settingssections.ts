// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { SettingsPageId } from "../../lib/settingscatalog";

// The panel titles and setting names each page draws, as catalog keys. Search
// finds a page by one of its settings in the reader's language. A tabbed page
// lists them per tab, led by the tab's own name, so a hit opens the right tab.
export const SETTINGS_SECTIONS = {
  account: [
    "settings.profileTitle",
    "account.signInCard",
    "account.browserCard",
    "account.pictureLabel",
    "settings.displayName",
    "settings.greetingName",
    "settings.signature",
    "password.title",
    "auth.signOut",
    "mfa.title",
    "locale.switchLabel",
    "account.languageCard",
    "account.timezoneLabel",
    "settings.appearance",
    "account.singleKeyShortcuts",
    "overnightGrant.title",
    "overnightGrant.label",
  ],
  meetings: {
    links: [
      "settings.part.meetings.links",
      "scheduling.linksTitle",
      "scheduling.setup.title",
      "scheduling.acceptBookings",
    ],
    availability: [
      "settings.part.meetings.availability",
      "scheduling.whenTitle",
      "workingHours.from",
      "workingHours.until",
      "scheduling.days",
      "scheduling.noticeRow",
      "scheduling.bufferRow",
      "scheduling.horizonRow",
    ],
    guests: [
      "settings.part.meetings.guests",
      "scheduling.guestView",
      "scheduling.companyRow",
      "scheduling.yourName",
      "scheduling.reminderRow",
    ],
  },
  voice: [
    "settings.voice.start.addTitle",
    "settings.voice.addFirstLabel",
    "settings.voice.pasteLabel",
    "settings.voice.sentMailLabel",
  ],
  agents: [
    "aiAccess.connect.title",
    "agents.connected",
    "aiAccess.dev.title",
    "settings.passports",
    "tools.title",
    "settings.autonomy",
    "autonomy.title",
    "approvalPreferences.title",
    "settings.passportUseAi",
    "settings.tierRead",
    "settings.tierSend",
    "settings.tierWait",
    "settings.tierAdvance",
  ],
  notifications: [
    "notifications.title",
    "notifications.browser.label",
    "delivery.panelTitle",
    "notifications.class.approval_pending.label",
    "notifications.class.project_health.label",
    "notifications.class.coach.label",
    "notifications.class.lead_sla.label",
    "notifications.class.automation.label",
    "notifications.class.capture.label",
    "notifications.class.system.label",
    "delivery.morningLabel",
    "delivery.weeklyLabel",
    "delivery.hourLabel",
    "delivery.weekdayLabel",
    "delivery.quietLabel",
    "delivery.combineLabel",
  ],
  connections: {
    mail: ["settings.part.connections.mail", "connectors.title"],
    addresses: [
      "settings.part.connections.addresses",
      "ownerIdentities.title",
      "ownerIdentities.addLabel",
    ],
    linkedin: [
      "settings.part.connections.linkedin",
      "linkedinReach.title",
      "linkedinImport.editProfileTitle",
      "linkedinImport.reachVisibleLabel",
      "linkedinImport.importLabel",
      "linkedinImport.choose",
    ],
    accounts: ["settings.part.connections.accounts"],
    activity: [
      "settings.part.connections.activity",
      "captureExclusions.title",
      "captureActivity.windowTitle",
      "captureActivity.outcomes",
      "connectionOutcomes.title",
      "senders.title",
      "heldThreads.title",
      "linkedinReach.title",
    ],
  },
  company: {
    profile: ["settings.part.company.profile"],
    entities: ["settings.part.company.entities", "legalEntities.title"],
    regional: [
      "settings.part.company.regional",
      "installationSettings.companyTitle",
      "installationSettings.timezone",
      "installationSettings.baseCurrency",
      "installationSettings.dateFormat",
      "installationSettings.timeFormat",
      "installationSettings.baseLanguage",
      "installationSettings.fiscalYearStart",
      "installationSettings.callWhatsApp",
      "installationSettings.callTelegram",
      "installationSettings.callZalo",
    ],
    money: [
      "settings.part.company.money",
      "settings.rates.fxTitle",
      "taxRates.title",
    ],
    email: [
      "settings.part.company.email",
      "followUpSettings.title",
      "signatureTemplate.title",
      "followUpSettings.days",
      "signatureTemplate.label",
    ],
  },
  authentication: {
    methods: [
      "settings.part.authentication.methods",
      "signInMethods.title",
      "signInMethods.password",
      "signInMethods.requireMfa",
    ],
    apps: ["settings.part.authentication.apps", "oauthApps.title"],
    grants: ["settings.part.authentication.grants", "groupRoles.title"],
  },
  members: [],
  teams: [],
  roles: [
    "roles.title",
    "sharedMergeReviews.title",
    "sharedMergeReviews.switch",
  ],
  seats: ["license.seats.title", "license.seats.inUse"],
  deals: {
    pipelines: ["settings.part.deals.pipelines", "pipeline.catalog"],
    sources: ["settings.part.deals.sources", "acqSources.title"],
    loss: [
      "settings.part.deals.loss",
      "lossReasons.title",
      "lossReasons.legacyTitle",
    ],
    reviews: ["settings.part.deals.reviews"],
    legal: [
      "settings.part.deals.legal",
      "legalReview.settingsTitle",
      "legalReview.settingsSwitch",
    ],
    forecast: [
      "settings.part.deals.forecast",
      "installationSettings.forecastTitle",
      "installationSettings.forwardMeasure",
      "installationSettings.likelyThreshold",
      "installationSettings.individualPerformance",
      "installationSettings.selfCorrecting",
    ],
  },
  leads: [
    "leadOrigins.title",
    "leadReasons.title",
    "leadHandling.title",
    "leadHandling.firstResponse",
    "leadHandling.targetMinutes",
  ],
  partners: ["partnerStages.title"],
  records: {
    fields: [
      "settings.part.records.fields",
      "cf.title",
      "cf.object",
      "cf.audit.title",
    ],
    tags: ["settings.part.records.tags", "tagAdmin.listLabel"],
    roles: ["settings.part.records.roles", "recordRoles.title"],
  },
  products: ["product.title", "template.title"],
  capture: {
    sharing: [
      "settings.part.capture.sharing",
      "mailSharing.cardTitle",
      "mailSharing.label",
      "mailSharing.sharedPosture.label",
      "captureMailSync.interval.label",
      "captureReview.passCap.label",
    ],
    domains: [
      "settings.part.capture.domains",
      "ownDomains.title",
      "consumerMail.title",
      "blockedDomains.title",
      "consumerMail.baselineTitle",
      "consumerMail.baselineSearchLabel",
    ],
    enrichment: [
      "settings.part.capture.enrichment",
      "captureSettings.title",
      "captureSettings.autoEnrich.label",
      "captureSettings.signatureEnrich.label",
      "captureReading.dailyCap.label",
      "captureReading.maxPages.label",
      "captureReading.maxMiB.label",
      "captureReading.wall.label",
    ],
    internal: ["settings.part.capture.internal", "internalMail.title"],
  },
  integrations: {
    contact: ["settings.part.integrations.contact", "provider.title"],
    accounting: [
      "settings.part.integrations.accounting",
      "financeSource.title",
    ],
    webhooks: ["settings.part.integrations.webhooks", "webhooks.title"],
    telegram: [
      "settings.part.integrations.telegram",
      "connectors.telegramTitle",
      "connectors.bot.dispactTitle",
    ],
    "shared-mailboxes": [
      "settings.part.integrations.sharedMailboxes",
      "sharedMailboxes.title",
    ],
    units: ["settings.part.integrations.units"],
  },
  "web-forms": ["webForms.title"],
  knowledge: ["knowledge.title"],
  import: [
    "import.hub.title",
    "import.hub.uploadLabel",
    "import.hub.templateTitle",
  ],
  usage: {
    allowance: [
      "settings.part.usage.allowance",
      "aiAdmin.allowance",
      "aiAdmin.used",
      "aiAdmin.resets",
      "aiAdmin.basis",
    ],
    agents: [
      "settings.part.usage.agents",
      "agentAllowances.title",
      "agentUsage.title",
    ],
    spend: ["settings.part.usage.spend", "aiusage.title"],
  },
  models: {
    providers: ["settings.part.models.providers", "aiProviderKeys.title"],
    tiers: ["settings.part.models.tiers", "aiRouting.title"],
    tasks: ["settings.part.models.tasks", "aiTasks.title"],
    prices: [
      "settings.part.models.prices",
      "aiPriceSync.title",
      "aiPriceSync.autoSync.label",
      "aiPriceSync.lastSyncedLabel",
    ],
  },
  mcp: [
    "agentClients.methods.title",
    "agentClients.title",
    "agentConnections.title",
    "agentClients.methods.published",
    "agentClients.methods.dynamic",
    "agentClients.methods.admin",
    "agentClients.experiments.title",
    "agentClients.experiments.clientCheck",
    "agentConnections.ttl.label",
  ],
  "model-calls": [],
  automations: {
    mine: ["settings.part.automations.mine", "auto.instances"],
    library: ["settings.part.automations.library", "auto.catalog"],
  },
  privacy: {
    consent: ["settings.part.privacy.consent", "settings.purposes"],
    customers: ["settings.part.privacy.customers", "privacy.customerNotices"],
    retention: [
      "settings.part.privacy.retention",
      "retention.title",
      "retention.retainOnly",
    ],
    requests: ["settings.part.privacy.requests", "privacy.requestsPanel"],
    duties: ["settings.part.privacy.duties", "notice.title"],
  },
  jurisdiction: [
    "jurisdiction.effects.title",
    "jurisdiction.selected.label",
    "jurisdiction.documents.title",
  ],
  audit: [],
  "system-health": {
    overview: ["health.overview"],
    diagnostics: [
      "health.diagnostics",
      "health.history",
      "embedreindex.statusLabel",
      "embedreindex.rebuildLabel",
      "recoveryHealth.lastDrill",
      "recoveryHealth.lastBackup",
      "embedreindex.title",
      "dataTasks.title",
      "settings.jobs",
      "settings.extIngest",
      "settings.providerHealth",
      "settings.captureHealth",
      "settings.automaticWork",
      "settings.recoveryHealth",
    ],
    configuration: [
      "health.configuration",
      "operations.schedules.title",
      "operations.pacing.title",
      "operations.schedules.fold",
      "operations.agentRunner.label",
      "operations.webhookRetry.label",
      "operations.timeScan.label",
      "operations.closeDate.label",
      "operations.followUp.label",
      "operations.retention.label",
      "operations.geocode.label",
      "operations.technical.label",
      "operations.gmailWatchScan.label",
      "operations.gmailWatchRenew.label",
      "operations.graphWatchScan.label",
      "operations.graphWatchRenew.label",
      "operations.sendRateLimit.label",
      "operations.sendRateWindow.label",
      "operations.sendMaxAge.label",
    ],
  },
  configuration: ["configuration.title"],
  extensions: [],
  reset: ["settings.dangerZone", "settings.resetDataLabel"],
} as const satisfies Record<
  SettingsPageId,
  readonly MessageKey[] | Readonly<Record<string, readonly MessageKey[]>>
>;

/**
 * The section keys one page draws, flat. On a tabbed page, only the tabs
 * `opensTab` lets through, so search never offers a part the reader cannot
 * open.
 */
export function settingsSectionKeys(
  page: SettingsPageId,
  opensTab: (tab: string) => boolean = () => true,
): readonly MessageKey[] {
  const sections:
    | readonly MessageKey[]
    | Readonly<Record<string, readonly MessageKey[]>> = SETTINGS_SECTIONS[page];
  if (Array.isArray(sections)) return sections;
  return Object.entries(sections)
    .filter(([tab]) => opensTab(tab))
    .flatMap(([, keys]) => keys);
}

/** The tab of `page` that draws the section `key`, if the page has tabs. */
export function settingsSectionTab(
  page: SettingsPageId,
  key: MessageKey,
): string | undefined {
  const sections:
    | readonly MessageKey[]
    | Readonly<Record<string, readonly MessageKey[]>> = SETTINGS_SECTIONS[page];
  if (Array.isArray(sections)) return undefined;
  return Object.entries(sections).find(([, keys]) => keys.includes(key))?.[0];
}

// The company-context card's names, drawn where `companyContextShown` says.
export const COMPANY_CONTEXT_SECTIONS: readonly MessageKey[] = [
  "settings.brandTitle",
  "settings.companyContextTitle",
  "settings.companySourceTitle",
  "ob.field.display_name",
  "ob.field.offer_summary",
  "ob.field.icp",
  "settings.companyPositioning",
  "ob.field.value_proposition",
  "ob.field.usp",
  "ob.field.customer_pains",
  "ob.field.desired_outcomes",
  "ob.field.buying_center",
  "ob.field.buying_intents",
  "ob.field.common_objections",
  "ob.field.sales_motion",
  "settings.companyIdentity",
  "ob.field.legal_name",
  "ob.field.registered_address",
  "ob.field.register_vat",
  "ob.field.industry",
  "ob.field.history",
  "settings.companyWebsite",
  "settings.companyRefreshRow",
];
