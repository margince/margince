import type { ReactNode } from "react";
import { CaptureSettingsCard } from "../../components/capture-settings";
import { MailSharingCard } from "../../components/mail-sharing";
import type { ReviewActions } from "../../lib/reviewactions";
import type { Route } from "../../lib/router";
import { AiBudgetCard } from "../ai/ai-admin";
import { ModelPricesCard } from "../ai/ai-price-sync";
import { AiProviderKeysCard } from "../ai/ai-provider-keys";
import { AiRoutingCard } from "../ai/ai-routing";
import { AiTasksCard } from "../ai/ai-tasks";
import { AiCallsCard } from "../ai/aicalls";
import { AiUsageCard } from "../ai/aiusage";
import { McpCards } from "../automations/agent-clients";
import { AgentAllowancesCard } from "../automations/agentallowances";
import { AgentUsageByUserCard } from "../automations/agentusage";
import { AutomationsAdmin } from "../automations/automations";
import { ExtensionAccessCard } from "../automations/extension-access";
import { ExtensionUnitsCard } from "../automations/extension-units";
import { UnitOAuthAppCards } from "../automations/oauth-app";
import { OvernightGrantCard } from "../automations/overnight-grant";
import { MeetingSettings } from "../booking/meeting-settings";
import { BriefDeliveryPanel } from "../brief/briefdelivery";
import { BlockedDomainsCard } from "../capture/blocked-domains";
import { CaptureActivityTab } from "../capture/capture-activity";
import { OwnerIdentitiesCard } from "../capture/capture-owner-identities";
import { ConnectorsCard, TelegramConnectorsPanel } from "../capture/connectors";
import { ConsumerMailDomainsCard } from "../capture/consumer-mail-domains";
import { ProviderCard } from "../capture/integrations-provider";
import { OwnDomainsCard } from "../capture/own-domains";
import { CompanyContextCard } from "../companies/company-context";
import { OfferTemplatesAdmin } from "../deals/offertemplates";
import { ProductsAdmin } from "../deals/products";
import { ReviewTemplatesCard } from "../deals/reviewtemplates";
import { ImportCard } from "../import/import";
import { LinkedInNetworkPanel } from "../import/linkedin-import";
import {
  LeadDisqualifyReasonsCard,
  LeadHandlingCard,
  LeadOriginsCard,
} from "../leads/leadvocab";
import { TagVocabularyCard } from "../lists/tagadmin";
import { SignatureTemplateCard } from "../mail/signaturetemplatecard";
import { CustomFieldsAdmin } from "../records/customfields";
import { RecordRolesCard } from "../records/recordroles";
import { InstallationSettingsCard } from "../sign-in/installation-settings";
import { SignInMethodsCard } from "../sign-in/sign-in-methods";
import { KnowledgeCard } from "../voice-knowledge/knowledge";
import { VoiceDnaCard, VoiceIntakeHost } from "../voice-knowledge/voice-dna";
import { AccountSettings } from "./account-settings";
import { AcquisitionSourcesCard } from "./acquisitionsources";
import { FinanceSourceCard } from "./financesource";
import { FollowUpSettingsCard } from "./followupsettings";
import { InstallationConfigurationPage } from "./installation-configuration";
import { InternalMailCaptureCard } from "./internalmailsettings";
import { LegalEntitiesCard } from "./legalentities";
import { LegalReviewSettingsCard } from "./legalreviewsettings";
import { LicenseCard } from "./license";
import { LegacyLossReasonsCard, LossReasonsCard } from "./lossreasons";
import { NotificationSettingsCard } from "./notification-settings";
import { OAuthAppsCard } from "./oauth-apps-card";
import { PartnerStagesCard } from "./partnerstages";
import { FxRatesCard } from "./rates";
import { RolesSettings } from "./roles-settings";
import { AiAccessTab } from "./settings.aiaccess";
import { JurisdictionCard } from "./settings.jurisdiction";
import { PipelinesCard } from "./settings.pipelines";
import { PrivacyLanes } from "./settings.privacy";
import { SystemHealthPage } from "./settings.systemhealth";
import { AuditLogCard } from "./settings-audit";
import { SharedMergeReviewsCard } from "./sharedmergereviews";
import { TaxRatesCard } from "./taxrates";
import { TeamsCard } from "./users-access";
import { UsersAdminCard } from "./users-admin";
import { WebFormsCard } from "./webforms";
import { WebhooksCard } from "./webhooks";
import "./settings.css";

import type { SettingsPageId } from "../../lib/settingscatalog";
import { ResetDataCard } from "./settings.reset";
import {
  ADMIN_SEGMENT,
  SETTINGS_SCREEN,
  SETTINGS_TABS,
  settingsAddress,
  settingsRouteTab,
  useSettingsSection,
} from "./settingsnav";
import { SettingsPage } from "./settingspage";

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
};

// Settings governance surface (B-EP09.13b): renders FROM the live seams —
// /me (identity + effective roles), passports (mint + the metadata list,
// token shown once and never re-disclosed), consent purposes (DOI flags),
// the privacy inbox (DSRs + statutory deadlines), the attributable
// audit-log view with live filters — plus the locked autonomy-tier table
// and the automations the installation runs unattended. EP09 renders
// governance; it never authors policy.

// What a page keeps across its tabs. Each tab is remounted on arrival, so
// work still running when the reader switches tabs lives here instead.
function PageFrame({
  id,
  children,
}: Readonly<{ id: SettingsPageId; children: ReactNode }>) {
  return id === "voice" ? (
    <VoiceIntakeHost>{children}</VoiceIntakeHost>
  ) : (
    children
  );
}

export function tabContent(
  id: SettingsPageId,
  tab: string,
  route?: Route,
): ReactNode {
  switch (id) {
    // ---- me ----
    case "account":
      return (
        <>
          <AccountSettings />
          {/* Personal AI work that runs as the reader: their own switch, so it
              sits with their account rather than the company's automations. */}
          <OvernightGrantCard />
        </>
      );
    case "notifications":
      return (
        <>
          <NotificationSettingsCard />
          <BriefDeliveryPanel />
        </>
      );
    case "meetings":
      return <MeetingSettings part={meetingsPart(tab)} />;
    case "voice":
      return <VoiceDnaCard tab={tab} />;
    case "agents":
      return <AiAccessTab />;
    case "connections":
      return connectionsTab(tab);

    // ---- company ----
    case "company":
      return companyTab(tab);
    case "authentication":
      // Who may sign in, and how: one vendor OAuth app serves sign-in and mail.
      if (tab === "apps")
        return (
          <>
            <OAuthAppsCard />
            <UnitOAuthAppCards />
          </>
        );
      return (
        <SignInMethodsCard part={tab === "grants" ? "grants" : "methods"} />
      );

    // ---- members, teams, roles and seats ----
    case "members":
      return <UsersAdminCard />;
    case "teams":
      // Its own page rather than a second card under members. Team Lead is
      // team-scoped, so this is also where an admin decides whose records a
      // Team Lead's membership hands over.
      return <TeamsCard />;
    case "roles":
      // Who may merge a colleague's duplicate of a contact is a question of
      // who may change what.
      return (
        <>
          <RolesSettings />
          <SharedMergeReviewsCard />
        </>
      );
    case "seats":
      return <LicenseCard />;

    // ---- sales ----
    case "deals":
      return dealsTab(tab);
    case "leads":
      return (
        <>
          <LeadOriginsCard />
          <LeadDisqualifyReasonsCard />
          <LeadHandlingCard />
        </>
      );
    case "partners":
      return <PartnerStagesCard />;
    case "records":
      if (tab === "tags") return <TagVocabularyCard />;
      if (tab === "roles") return <RecordRolesCard />;
      return <CustomFieldsAdmin />;
    case "products":
      return (
        <>
          <ProductsAdmin />
          <OfferTemplatesAdmin />
        </>
      );

    // ---- data ----
    case "capture":
      return captureTab(tab);
    case "integrations":
      return integrationsTab(tab);
    case "web-forms":
      return <WebFormsCard />;
    case "knowledge":
      return <KnowledgeCard />;
    case "import":
      return <ImportCard subpage={route?.id2} />;

    // ---- ai ----
    case "models":
      return modelsTab(tab);
    case "mcp":
      return <McpCards />;
    case "usage":
      if (tab === "agents")
        return (
          <>
            <AgentAllowancesCard />
            <AgentUsageByUserCard />
          </>
        );
      return tab === "spend" ? <AiUsageCard /> : <AiBudgetCard />;
    case "model-calls":
      return <AiCallsCard />;
    case "automations":
      return <AutomationsAdmin view={tab === "library" ? "library" : "mine"} />;

    // ---- compliance ----
    case "privacy":
      return <PrivacyLanes tab={tab} />;
    case "jurisdiction":
      return <JurisdictionCard />;
    case "audit":
      // Its own page: it answers to `audit_log`, where the privacy page answers
      // to consent and retention, and one reader can hold one without the other.
      return <AuditLogCard />;

    // ---- system ----
    case "system-health":
      return <SystemHealthPage view={tab} />;
    case "configuration":
      return <InstallationConfigurationPage />;
    case "extensions":
      return <ExtensionAccessCard />;
    case "reset":
      // Its own page: a reindex, a queue reading and "empty the installation"
      // were three verbs on one screen, separated by nothing.
      return <ResetDataCard />;
  }
}

function meetingsPart(tab: string) {
  if (tab === "availability" || tab === "guests") return tab;
  return "links";
}

// What you are connected to, and what those connections did with your mail.
// The connector list is scoped to the calling human server-side and LinkedIn
// reads `/me`, so every tab is personal and needs no grant.
function connectionsTab(tab: string): ReactNode {
  switch (tab) {
    case "addresses":
      // Per seat, not per connection: an address declared here is the same
      // contact, so mail among them never becomes a conversation.
      return <OwnerIdentitiesCard />;
    case "linkedin":
      return <LinkedInNetworkPanel />;
    case "accounts":
      // Units whose credential is the member's own, offered here rather than
      // from the rail because enabling one adds something to configure.
      return <ExtensionUnitsCard scope="user" />;
    case "activity":
      return <CaptureActivityTab />;
    default:
      return <ConnectorsCard />;
  }
}

// The company itself, then the installation's own facts. The base currency is
// declared on Regional and every rate on Money converts to it.
function companyTab(tab: string): ReactNode {
  switch (tab) {
    case "entities":
      return <LegalEntitiesCard />;
    case "regional":
      return <InstallationSettingsCard part="regional" />;
    case "money":
      return (
        <>
          <FxRatesCard />
          <TaxRatesCard />
        </>
      );
    case "email":
      return (
        <>
          <FollowUpSettingsCard />
          <SignatureTemplateCard />
        </>
      );
    default:
      return <CompanyContextCard />;
  }
}

// How deals move, where they come from, and why they are won or lost.
function dealsTab(tab: string): ReactNode {
  switch (tab) {
    case "sources":
      return <AcquisitionSourcesCard />;
    case "loss":
      return (
        <>
          <LossReasonsCard />
          <LegacyLossReasonsCard />
        </>
      );
    case "reviews":
      return <ReviewTemplatesCard />;
    case "legal":
      // Whether a stage may wait for legal is a pipeline question.
      return <LegalReviewSettingsCard />;
    case "forecast":
      return <InstallationSettingsCard part="forecast" />;
    default:
      return <PipelinesCard />;
  }
}

// The rules in the order mail meets them, then their outcome: which domains
// ended up refused a company.
function captureTab(tab: string): ReactNode {
  switch (tab) {
    case "domains":
      return (
        <>
          <OwnDomainsCard />
          <ConsumerMailDomainsCard />
          <BlockedDomainsCard />
        </>
      );
    case "enrichment":
      return <CaptureSettingsCard />;
    case "internal":
      return <InternalMailCaptureCard />;
    default:
      return <MailSharingCard />;
  }
}

// What the INSTALLATION is wired to. Each is shared by everybody, which is why
// they sit under the company heading and the personal connections do not.
function integrationsTab(tab: string): ReactNode {
  switch (tab) {
    case "accounting":
      return <FinanceSourceCard />;
    case "webhooks":
      return <WebhooksCard />;
    case "telegram":
      return <TelegramConnectorsPanel />;
    case "units":
      // A unit whose secret is `workspace`-scoped holds the installation's
      // credential. Which page a unit lands on is its manifest's decision.
      return <ExtensionUnitsCard scope="workspace" />;
    default:
      return <ProviderCard />;
  }
}

// Each AI tab is its own address. The unsaved guard watches addresses, so it
// sees a move between them, and each card claims it itself.
function modelsTab(tab: string): ReactNode {
  switch (tab) {
    case "tiers":
      return <AiRoutingCard />;
    case "tasks":
      return <AiTasksCard />;
    case "prices":
      return <ModelPricesCard />;
    default:
      return <AiProviderKeysCard />;
  }
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
        id === "models" && tab === "providers" ? (
          <>
            {aiSetup}
            {tabContent(id, tab, at)}
          </>
        ) : (
          tabContent(id, tab, at)
        )
      }
      Frame={PageFrame}
    />
  );
}
