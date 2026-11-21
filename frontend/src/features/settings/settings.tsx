// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { ReviewActions } from "../../lib/reviewactions";
import type { Route } from "../../lib/router";
import type { SettingsPageId } from "../../lib/settingscatalog";
import "./settings.css";
import { SettingsPage } from "./settingspage";
import {
  FOLDER_PAGE_VIEWS,
  PageLoading,
  page,
  pick,
  renewable,
} from "./settingspageviews";

// Re-exported so this module's own consumers — the tests, the stories, the
// testkit — keep asking one module for both halves. Splitting their imports
// would be churn that proves nothing about the split that matters, which is
// `src/app/**` no longer reaching the cards.
export {
  ADMIN_SEGMENT,
  SETTINGS_SCREEN,
  SETTINGS_TABS,
  settingsAddress,
  settingsRouteTab,
  useSettingsSection,
} from "./settingsnav";

// Settings governance surface (B-EP09.13b): renders FROM the live seams —
// /me (identity + effective roles), passports (mint + the metadata list,
// token shown once and never re-disclosed), consent purposes (DOI flags),
// the privacy inbox (DSRs + statutory deadlines), the attributable
// audit-log view with live filters — plus the locked autonomy-tier table
// and the automations the installation runs unattended. EP09 renders
// governance; it never authors policy.

/** What each catalog page draws, by page id. */
export const SETTINGS_PAGE_VIEWS = {
  ...FOLDER_PAGE_VIEWS,
  // ---- me ----
  account: page(
    () =>
      Promise.all([
        import("./account-settings"),
        import("../automations/overnight-grant"),
      ]),
    ([{ AccountSettings }, { OvernightGrantCard }]) => (
      <>
        <AccountSettings />
        {/* Personal AI work that runs as the reader: their own switch, so it
            sits with their account rather than the company's automations. */}
        <OvernightGrantCard />
      </>
    ),
  ),
  notifications: page(
    () =>
      Promise.all([
        import("./notification-settings"),
        import("../brief/briefdelivery"),
      ]),
    ([{ NotificationSettingsCard }, { BriefDeliveryPanel }]) => (
      <>
        <NotificationSettingsCard />
        <BriefDeliveryPanel />
      </>
    ),
  ),
  meetings: page(
    () => import("../booking/meeting-settings"),
    ({ MeetingSettings }, { tab }) => (
      <MeetingSettings
        part={tab === "availability" || tab === "guests" ? tab : "links"}
      />
    ),
  ),
  voice: page(
    () => import("../voice-knowledge/voice-dna"),
    ({ VoiceDnaCard }, { tab }) => <VoiceDnaCard tab={tab} />,
  ),
  // What you are connected to, and what those connections did with your mail.
  // The connector list is scoped to the calling human server-side and LinkedIn
  // reads `/me`, so every tab is personal and needs no grant.
  connections: page(
    () =>
      Promise.all([
        import("../capture/capture-owner-identities"),
        import("../import/linkedin-import"),
        import("../automations/extension-units"),
        import("../capture/capture-activity"),
        import("../capture/connectors"),
      ]),
    (
      [
        { OwnerIdentitiesCard },
        { LinkedInNetworkPanel },
        { ExtensionUnitsCard },
        { CaptureActivityTab },
        { ConnectorsCard },
      ],
      { tab },
    ) =>
      pick(tab, <ConnectorsCard />, {
        // Per seat, not per connection: an address declared here is the same
        // contact, so mail among them never becomes a conversation.
        addresses: <OwnerIdentitiesCard />,
        linkedin: <LinkedInNetworkPanel />,
        // Units whose credential is the member's own, offered here rather than
        // from the rail because enabling one adds something to configure.
        accounts: <ExtensionUnitsCard scope="user" />,
        activity: <CaptureActivityTab />,
      }),
  ),

  // ---- company ----
  // The company itself, then the installation's own facts. The base currency is
  // declared on Regional and every rate on Money converts to it.
  company: page(
    () =>
      Promise.all([
        import("./legalentities"),
        import("../sign-in/installation-settings"),
        import("./rates"),
        import("./taxrates"),
        import("./followupsettings"),
        import("../mail/signaturetemplatecard"),
        import("../companies/company-context"),
      ]),
    (
      [
        { LegalEntitiesCard },
        { InstallationSettingsCard },
        { FxRatesCard },
        { TaxRatesCard },
        { FollowUpSettingsCard },
        { SignatureTemplateCard },
        { CompanyContextCard },
      ],
      { tab },
    ) =>
      pick(tab, <CompanyContextCard />, {
        entities: <LegalEntitiesCard />,
        regional: <InstallationSettingsCard part="regional" />,
        money: (
          <>
            <FxRatesCard />
            <TaxRatesCard />
          </>
        ),
        email: (
          <>
            <FollowUpSettingsCard />
            <SignatureTemplateCard />
          </>
        ),
      }),
  ),
  // Who may sign in, and how: one vendor OAuth app serves sign-in and mail.
  authentication: page(
    () =>
      Promise.all([
        import("./oauth-apps-card"),
        import("../automations/oauth-app"),
        import("../sign-in/sign-in-methods"),
      ]),
    (
      [{ OAuthAppsCard }, { UnitOAuthAppCards }, { SignInMethodsCard }],
      { tab },
    ) =>
      pick(tab, <SignInMethodsCard part="methods" />, {
        grants: <SignInMethodsCard part="grants" />,
        apps: (
          <>
            <OAuthAppsCard />
            <UnitOAuthAppCards />
          </>
        ),
      }),
  ),

  // ---- sales ----
  // How deals move, where they come from, and why they are won or lost.
  deals: page(
    () =>
      Promise.all([
        import("./acquisitionsources"),
        import("./lossreasons"),
        import("../deals/reviewtemplates"),
        import("./legalreviewsettings"),
        import("../sign-in/installation-settings"),
        import("./settings.pipelines"),
      ]),
    (
      [
        { AcquisitionSourcesCard },
        { LossReasonsCard, LegacyLossReasonsCard },
        { ReviewTemplatesCard },
        { LegalReviewSettingsCard },
        { InstallationSettingsCard },
        { PipelinesCard },
      ],
      { tab },
    ) =>
      pick(tab, <PipelinesCard />, {
        sources: <AcquisitionSourcesCard />,
        loss: (
          <>
            <LossReasonsCard />
            <LegacyLossReasonsCard />
          </>
        ),
        reviews: <ReviewTemplatesCard />,
        // Whether a stage may wait for legal is a pipeline question.
        legal: <LegalReviewSettingsCard />,
        forecast: <InstallationSettingsCard part="forecast" />,
      }),
  ),
  leads: page(
    () => import("../leads/leadvocab"),
    ({ LeadOriginsCard, LeadDisqualifyReasonsCard, LeadHandlingCard }) => (
      <>
        <LeadOriginsCard />
        <LeadDisqualifyReasonsCard />
        <LeadHandlingCard />
      </>
    ),
  ),
  records: page(
    () =>
      Promise.all([
        import("../lists/tagadmin"),
        import("../records/recordroles"),
        import("../records/customfields"),
      ]),
    (
      [{ TagVocabularyCard }, { RecordRolesCard }, { CustomFieldsAdmin }],
      { tab },
    ) =>
      pick(tab, <CustomFieldsAdmin />, {
        tags: <TagVocabularyCard />,
        roles: <RecordRolesCard />,
      }),
  ),
  products: page(
    () =>
      Promise.all([
        import("../deals/products"),
        import("../deals/offertemplates"),
      ]),
    ([{ ProductsAdmin }, { OfferTemplatesAdmin }]) => (
      <>
        <ProductsAdmin />
        <OfferTemplatesAdmin />
      </>
    ),
  ),

  // ---- data ----
  // The rules in the order mail meets them, then their outcome: which domains
  // ended up refused a company.
  capture: page(
    () =>
      Promise.all([
        import("../capture/own-domains"),
        import("../capture/consumer-mail-domains"),
        import("../capture/blocked-domains"),
        import("../../components/capture-settings"),
        import("./internalmailsettings"),
        import("../../components/mail-sharing"),
      ]),
    (
      [
        { OwnDomainsCard },
        { ConsumerMailDomainsCard },
        { BlockedDomainsCard },
        { CaptureSettingsCard },
        { InternalMailCaptureCard },
        { MailSharingCard },
      ],
      { tab },
    ) =>
      pick(tab, <MailSharingCard />, {
        domains: (
          <>
            <OwnDomainsCard />
            <ConsumerMailDomainsCard />
            <BlockedDomainsCard />
          </>
        ),
        enrichment: <CaptureSettingsCard />,
        internal: <InternalMailCaptureCard />,
      }),
  ),
  // What the INSTALLATION is wired to. Each is shared by everybody, which is why
  // they sit under the company heading and the personal connections do not.
  integrations: page(
    () =>
      Promise.all([
        import("./financesource"),
        import("./webhooks"),
        import("../capture/connectors"),
        import("./sharedmailboxes"),
        import("../automations/extension-units"),
        import("../capture/integrations-provider"),
      ]),
    (
      [
        { FinanceSourceCard },
        { WebhooksCard },
        { WorkspaceBotPanel },
        { SharedMailboxesCard },
        { ExtensionUnitsCard },
        { ProviderCard },
      ],
      { tab },
    ) =>
      pick(tab, <ProviderCard />, {
        accounting: <FinanceSourceCard />,
        webhooks: <WebhooksCard />,
        telegram: (
          <>
            <WorkspaceBotPanel provider="telegram" />
            <WorkspaceBotPanel provider="dispact" />
          </>
        ),
        "shared-mailboxes": <SharedMailboxesCard />,
        // A unit whose secret is `workspace`-scoped holds the installation's
        // credential. Which page a unit lands on is its manifest's decision.
        units: <ExtensionUnitsCard scope="workspace" />,
      }),
  ),
  knowledge: page(
    () => import("../voice-knowledge/knowledge"),
    ({ KnowledgeCard }) => <KnowledgeCard />,
  ),
  import: page(
    () => import("../import/import"),
    ({ ImportCard }, { route }) => <ImportCard subpage={route?.id2} />,
  ),

  // ---- ai ----
  // Each AI tab is its own address. The unsaved guard watches addresses, so it
  // sees a move between them, and each card claims it itself.
  models: page(
    () =>
      Promise.all([
        import("../ai/ai-routing"),
        import("../ai/ai-tasks"),
        import("./followupsettings"),
        import("../ai/ai-price-sync"),
        import("../ai/ai-provider-keys"),
      ]),
    (
      [
        { AiRoutingCard },
        { AiTasksCard },
        { PrivateMailRequestsCard },
        { ModelPricesCard },
        { AiProviderKeysCard },
      ],
      { tab },
    ) =>
      pick(tab, <AiProviderKeysCard />, {
        tiers: <AiRoutingCard />,
        tasks: (
          <>
            <AiTasksCard />
            <PrivateMailRequestsCard />
          </>
        ),
        prices: <ModelPricesCard />,
      }),
  ),
  mcp: page(
    () => import("../automations/agent-clients"),
    ({ McpCards }) => <McpCards />,
  ),
  usage: page(
    () =>
      Promise.all([
        import("../automations/agentallowances"),
        import("../automations/agentusage"),
        import("../ai/aiusage"),
        import("../ai/ai-admin"),
      ]),
    (
      [
        { AgentAllowancesCard },
        { AgentUsageByUserCard },
        { AiUsageCard },
        { AiBudgetCard },
      ],
      { tab },
    ) =>
      pick(tab, <AiBudgetCard />, {
        agents: (
          <>
            <AgentAllowancesCard />
            <AgentUsageByUserCard />
          </>
        ),
        spend: <AiUsageCard />,
      }),
  ),
  "model-calls": page(
    () => import("../ai/aicalls"),
    ({ AiCallsCard }) => <AiCallsCard />,
  ),
  automations: page(
    () => import("../automations/automations"),
    ({ AutomationsAdmin }, { tab }) => (
      <AutomationsAdmin view={tab === "library" ? "library" : "mine"} />
    ),
  ),
  extensions: page(
    () => import("../automations/extension-access"),
    ({ ExtensionAccessCard }) => <ExtensionAccessCard />,
  ),
} satisfies Record<SettingsPageId, ReturnType<typeof page>>;

const VoiceIntakeHost = renewable(async () => ({
  default: (await import("../voice-knowledge/voice-dna")).VoiceIntakeHost,
}));

export function tabContent(
  id: SettingsPageId,
  tab: string,
  route?: Route,
  lead?: ReactNode,
): ReactNode {
  const Page = SETTINGS_PAGE_VIEWS[id];
  return (
    <PageLoading id={id}>
      {lead}
      <Page tab={tab} route={route} />
    </PageLoading>
  );
}

// What a page keeps across its tabs. Each tab is remounted on arrival, so
// work still running when the reader switches tabs lives here instead.
function PageFrame({
  id,
  children,
}: Readonly<{ id: SettingsPageId; children: ReactNode }>) {
  if (id !== "voice") return children;
  return (
    <PageLoading id={id}>
      <VoiceIntakeHost>{children}</VoiceIntakeHost>
    </PageLoading>
  );
}

/**
 * The settings screen: one page of the catalog, chosen by the address. Its
 * frame is SettingsPage; this module supplies the cards each page draws.
 */
export function SettingsScreen({
  route,
  reviewActions,
  aiSetup,
}: Readonly<{
  route: Route;
  reviewActions?: ReviewActions;
  /** The first-run AI form, from the sign-in feature through the shell. It
   *  opens the AI models page, so a skipped key is set where keys live. */
  aiSetup?: ReactNode;
}>) {
  return (
    <SettingsPage
      route={route}
      reviewActions={reviewActions}
      content={(id, tab, at) =>
        tabContent(
          id,
          tab,
          at,
          id === "models" && tab === "providers" ? aiSetup : null,
        )
      }
      Frame={PageFrame}
    />
  );
}
