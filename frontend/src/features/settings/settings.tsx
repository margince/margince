import { type ReactNode, useEffect } from "react";
import { CaptureSettingsCard } from "../../components/capture-settings";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import {
  type ReviewActions,
  ReviewActionsContext,
  useReviewActions,
} from "../../lib/reviewactions";
import { navigateReplacing, type Route } from "../../lib/router";
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
import { OAuthAppCards } from "../automations/oauth-app";
import { OvernightGrantCard } from "../automations/overnight-grant";
import { MeetingSettings } from "../booking/meeting-settings";
import { BriefDeliveryPanel } from "../brief/briefdelivery";
import { BlockedDomainsCard } from "../capture/blocked-domains";
import { CaptureActivityTab } from "../capture/capture-activity";
import { OwnerIdentitiesCard } from "../capture/capture-owner-identities";
import { ConnectionOutcomesCard } from "../capture/connection-outcomes";
import { ConnectorsCard } from "../capture/connectors";
import { ConsumerMailDomainsCard } from "../capture/consumer-mail-domains";
import { ProviderCard } from "../capture/integrations-provider";
import { OwnDomainsCard } from "../capture/own-domains";
import { CompanyContextCard } from "../companies/company-context";
import { OfferTemplatesAdmin } from "../deals/offertemplates";
import { ProductsAdmin } from "../deals/products";
import { ReviewTemplatesCard } from "../deals/reviewtemplates";
import { ImportCard } from "../import/import";
import { LinkedInImportCard } from "../import/linkedin-import";
import {
  LeadDisqualifyReasonsCard,
  LeadHandlingCard,
  LeadSourcesCard,
} from "../leads/leadvocab";
import { TagVocabularyCard } from "../lists/tagadmin";
import { SignatureTemplateCard } from "../mail/signaturetemplatecard";
import { CustomFieldsAdmin } from "../records/customfields";
import { RecordRolesCard } from "../records/recordroles";
import { InstallationSettingsCard } from "../sign-in/installation-settings";
import { SignInMethodsCard } from "../sign-in/sign-in-methods";
import { KnowledgeCard } from "../voice-knowledge/knowledge";
import { VoiceDnaCard } from "../voice-knowledge/voice-dna";
import { AccountSettings } from "./account-settings";
import { AcquisitionSourcesCard } from "./acquisitionsources";
import { FinanceSourceCard } from "./financesource";
import { FollowUpSettingsCard } from "./followupsettings";
import { InstallationConfigurationPage } from "./installation-configuration";
import { LicenseCard } from "./license";
import { MailSharingCard, MailSharingPostureRow } from "./mail-sharing";
import { NotificationSettingsCard } from "./notification-settings";
import { FxRatesCard } from "./rates";
import { RolesSettings } from "./roles-settings";
import { AiAccessTab } from "./settings.aiaccess";
import { JurisdictionCard } from "./settings.jurisdiction";
import { PipelinesCard } from "./settings.pipelines";
import { PrivacyLanes } from "./settings.privacy";
import { StageAutomationCard } from "./settings.stageautomation";
import { SystemHealthPage } from "./settings.systemhealth";
import { AuditLogCard } from "./settings-audit";
import { TeamsCard } from "./users-access";
import { UsersAdminCard } from "./users-admin";
import { WebhooksCard } from "./webhooks";
import "./settings.css";

import { ResetDataCard } from "./settings.reset";
import { readsOnly, type SettingsPageId } from "./settingscatalog";
import { SettingsBoundary, SettingsHome } from "./settingshome";
import {
  ADMIN_SEGMENT,
  SETTINGS_SCREEN,
  SETTINGS_TABS,
  settingsAddress,
  settingsRouteTab,
  useSettingsReach,
  useSettingsSection,
  useVisibleSettingsPages,
} from "./settingsnav";
import { settingsHref, settingsRouteTarget } from "./settingsrouting";

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

export function tabContent(id: SettingsPageId, route?: Route): ReactNode {
  switch (id) {
    // ---- me ----
    case "account":
      return <AccountSettings />;
    case "meetings":
      return <MeetingSettings />;
    case "voice":
      return <VoiceDnaCard />;
    case "agents":
      return <AiAccessTab />;
    case "notifications":
      return (
        <>
          <NotificationSettingsCard />
          <BriefDeliveryPanel />
        </>
      );
    case "connections":
      return <ConnectionsTab />;
    // Beside `connections` and after it on purpose: that page says what you are
    // connected to, this one says what those connections did with your mail.
    case "capture-activity":
      return <CaptureActivityTab />;

    // ---- company ----
    case "company":
      // The company first (brand, business context and the website that context
      // is read from), then the installation's own facts. The currency pair stays
      // ADJACENT: the base currency is declared in InstallationSettingsCard and
      // every rate below converts to it.
      return (
        <>
          <CompanyContextCard />
          <InstallationSettingsCard />
          <FxRatesCard />
          <FollowUpSettingsCard />
          <SignatureTemplateCard />
        </>
      );
    case "authentication":
      // Who may sign in, and how: one vendor OAuth app serves sign-in and mail.
      return (
        <>
          <SignInMethodsCard />
          <OAuthAppCards />
        </>
      );

    // ---- contacts ----
    case "members":
      return <UsersAdminCard />;
    case "teams":
      // Its own page rather than a second card under members. Teams are share
      // targets and a way to address a group, but Team Lead is team-scoped
      // (RowScopeTeam) — so this is also where an admin decides whose records a
      // Team Lead's membership hands over, which is not the roster's question.
      return <TeamsCard />;
    case "roles":
      return <RolesSettings />;
    case "seats":
      return <LicenseCard />;

    // ---- sales ----
    case "pipelines":
      return <PipelinesCard />;
    case "stageautomation":
      return <StageAutomationCard />;
    case "leads":
      return (
        <>
          <LeadSourcesCard />
          <LeadDisqualifyReasonsCard />
          <LeadHandlingCard />
        </>
      );
    case "acquisition":
      return <AcquisitionSourcesCard />;
    case "reviewtemplates":
      return <ReviewTemplatesCard />;
    case "recordroles":
      return <RecordRolesCard />;
    case "fields":
      return <CustomFieldsAdmin />;
    case "tags":
      return <TagVocabularyCard />;
    case "products":
      return (
        <>
          <ProductsAdmin />
          <OfferTemplatesAdmin />
        </>
      );

    // ---- data ----
    case "capture":
      return (
        <>
          {/* The rules in the order mail meets them, then their outcome:
              which domains ended up refused a company. */}
          <MailSharingCard />
          <OwnDomainsCard />
          <CaptureSettingsCard />
          <ConsumerMailDomainsCard />
          <BlockedDomainsCard />
        </>
      );
    case "integrations":
      return <IntegrationsTab />;
    case "knowledge":
      return <KnowledgeCard />;
    case "import":
      return <ImportCard subpage={route?.id2} />;

    // ---- ai ----
    // Each AI page is its own address. The unsaved guard watches addresses, so
    // it sees a move between them, and each card claims it itself.
    case "models":
      return (
        <>
          <AiProviderKeysCard />
          {/* The prices those vendors charge, below the keys that decide which vendors are priced. */}
          <ModelPricesCard />
          <AiRoutingCard />
          <AiTasksCard />
        </>
      );
    case "automations":
      return <AutomationsAdmin />;
    case "usage":
      return (
        <>
          {/* The allowance, each agent's limits and use, then the spend. */}
          <AiBudgetCard />
          <AgentAllowancesCard />
          <AgentUsageByUserCard />
          <AiUsageCard />
        </>
      );
    case "mcp":
      return <McpCards />;
    case "model-calls":
      return <AiCallsCard />;

    // ---- governance ----
    case "privacy":
      return <PrivacyLanes />;
    case "jurisdiction":
      return <JurisdictionCard />;
    case "audit":
      // Its own page: it answers to `audit_log`, where the privacy page answers
      // to consent and retention, and one reader can hold one without the other.
      return <AuditLogCard />;
    case "system-health":
      return <SystemHealthPage />;
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

// What YOU are connected to. Most surfaces here read a per-user seam: the
// server scopes mail connectors to the caller, and LinkedIn reads `/me`. So it
// sits in the personal group and needs no grant. The catalog marks it `mixed`:
// ConnectorsCard's second panel is the workspace's Telegram bot. The
// mail-sharing row states a rule that is changed on Capture rules.
function ConnectionsTab() {
  return (
    <>
      {/* The rule first, then the mailboxes that live under it: sharing is a
          posture every user works under, not a property of any one connection
          below. It is STATED here and CHANGED on Capture rules — this page is
          the reader's own mailboxes, and the switch that decides whether
          everybody's captured mail is shared does not belong among them. */}
      <MailSharingPostureRow />
      <ConnectorsCard />
      {/* Per seat, not per connection: an address declared here is the same
          contact, so mail among them never becomes a conversation. */}
      <OwnerIdentitiesCard />
      <OvernightGrantCard />
      <LinkedInImportCard />
      <ConnectionOutcomesCard />
      {/* Last, and only when the installation composed a unit whose credential
          is the member's OWN — that is what a `user`-scoped secret is, and it
          is the same thing every card above it holds. A unit is offered here
          rather than from the rail because enabling one adds something to
          configure, not a destination. */}
      <ExtensionUnitsCard scope="user" />
    </>
  );
}

// What the INSTALLATION is wired to: one shared contact-data credential and the
// outbound subscriptions. Both are workspace-wide — a key everybody spends from
// and a webhook everybody's writes fire — which is why they sit under the
// company heading and the personal connections do not.
function IntegrationsTab() {
  return (
    <>
      <ProviderCard />
      <FinanceSourceCard />
      <WebhooksCard />
      {/* A unit whose secret is `workspace`-scoped holds the installation's
          credential, like the cards above, so it is offered here. Which page a
          unit lands on is its manifest's decision, never this file's. */}
      <ExtensionUnitsCard scope="workspace" />
    </>
  );
}

// The shape a record takes: which fields it carries, which stages it moves
// through, and the priced things that go on an offer. Four surfaces that were
// three separate screens behind door-cards and one editor inline — a door is
// not a section, and the doors are gone.
/**
 * The settings screen: one page of the catalog, chosen by the address.
 *
 * Flat addresses now — `#/settings/audit`, not `#/settings/admin/audit`. The
 * group segment said which HALF of settings a reader was in, which was a fact
 * about who the page was for rather than about the page, so moving one between
 * groups moved its bookmark too. `settingsRouteTarget` still answers every
 * address the product minted, rewriting it once on arrival.
 */
export function SettingsScreen({
  route,
  reviewActions,
}: Readonly<{ route: Route; reviewActions?: ReviewActions }>) {
  const inheritedReviewActions = useReviewActions();
  const t = useT();
  const target = settingsRouteTarget(route);
  const visible = useVisibleSettingsPages();
  // The same pages, partitioned. `visible` still decides the BOUNDARY — may
  // this reader open the address at all — and the partition decides only how
  // the page presents itself once opened.
  const reach = useSettingsReach();
  // The page the address names, if this reader may open it.
  const named =
    target.kind === "page"
      ? visible.find((page) => page.id === target.page)
      : undefined;
  // An address this reader may not open, or one nothing answers, is a BOUNDARY
  // — not a redirect to Account.
  //
  // The fallback it replaces was silent in the worst way: it rewrote the URL to
  // the page it had chosen, so a reader who followed a colleague's link saw
  // Account, saw an address saying Account, and had no way to tell that the
  // link had gone somewhere else. They would report the link as broken, and the
  // sender would open it and find it worked.
  //
  // The URL is left EXACTLY as typed. That is the whole affordance: the reader
  // can read what they asked for, copy it, and ask the colleague who has it.
  const boundary =
    target.kind === "unknown"
      ? "unknown"
      : target.kind === "page" && named === undefined
        ? "denied"
        : undefined;
  const active = named ?? visible[0];
  // Only a page this reader actually reached can be rewritten to. A legacy
  // address they may not open is a boundary, and rewriting it would replace the
  // address they need to quote with one that is not theirs either.
  const legacy = target.kind === "page" && target.legacy && named !== undefined;
  // Whether this page is one the reader consults rather than one they work in.
  // Read off the same partition the rail is built from, so a page that left the
  // rail for being read-only is the page that explains itself here.
  const readOnlyPage = active !== undefined && readsOnly(reach, active.id);
  // A legacy admin address is answered AND rewritten: the reader gets the page
  // they asked for, and the URL bar then says where that page lives, so the
  // link they copy from it is the current one. Replaced rather than pushed, or
  // Back would land on the address that redirects and trap them there.
  //
  // Keyed on the entry the route RESOLVED to rather than on the segment it
  // carried, so a rewrite never invents an address: a legacy link to a page the
  // reader may not open is left as typed, and the boundary answers it.
  useEffect(() => {
    if (legacy && active) {
      navigateReplacing(settingsHref(active.id));
    }
  }, [legacy, active]);
  // No nav column and no heading of its own: the entries are the sidebar's second
  // level now, and the shell's page head names the entry, so the page is that
  // entry's own content across the whole reading column.
  //
  // Every page sets its rhythm HERE rather than relying on the shell's
  // `.wrap > .card + .card` default, because that rule matches a card following a
  // card and settings pages are no longer stacks of cards: the merge left several
  // holding a `<form>`, a `<section>`, a flex wrapper or a bare heading-plus-table.
  // Where the rule missed, the gap was ZERO and two surfaces read as one. Owning
  // it once is the difference between every page spacing correctly and every
  // page having to remember to. Width is owned the same way and is the same for
  // all of them, so there is nothing here to branch on.
  if (target.kind === "home") {
    return (
      <div className="wrap">
        <SettingsHome reach={reach} />
      </div>
    );
  }

  if (boundary) {
    return (
      <div className="wrap">
        <SettingsBoundary kind={boundary} />
      </div>
    );
  }
  if (active === undefined) return null;

  return (
    <div className="wrap">
      {/* Unsaved drafts in here are held by the guard above the routed screen
          (App.tsx), not by this screen. A guard installed HERE could only see
          moves between settings entries: it unmounts with the screen, so a draft
          was safe from one tab to the next and still discarded without a word the
          moment the reader clicked Contacts. The cards below claim through
          `useUnsavedGuard` and need to know nothing about where the answer is
          asked. */}
      <div className="settings-stack arrive-stack">
        {/* Said ONCE, at the top, and only for a page with NO open control —
            the `looksUp` half of the partition: a reader who can change nothing
            would otherwise infer it from a screenful of disabled cards, and a
            banner over a control that works is worse than silence. */}
        {readOnlyPage && (
          <Callout kind="standing" title={t("settings.readOnlyPageTitle")}>
            {t("settings.readOnlyPage")}
          </Callout>
        )}
        <ReviewActionsContext value={reviewActions ?? inheritedReviewActions}>
          {tabContent(active.id, route)}
        </ReviewActionsContext>
      </div>
    </div>
  );
}
