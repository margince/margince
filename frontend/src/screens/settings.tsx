import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { operations } from "../api/schema";
import { THEME_LABEL_KEYS } from "../app/account";
import { useCanWrite } from "../app/capability";
import { navigateReplacing, type Route } from "../app/router";
import { setThemeChoice, THEME_CHOICES, useThemeChoice } from "../app/theme";
import { Avatar, Button, Disclosure, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { LOCALES, type Locale, localeNameKey, useLocale, useT } from "../i18n";
import { AcquisitionSourcesCard } from "./acquisitionsources";
import { AgentConnectionsCard } from "./agent-token-settings";
import { AiBudgetCard } from "./ai-admin";
import { ModelPricesCard } from "./ai-price-sync";
import { AiProviderKeysCard } from "./ai-provider-keys";
import { AiRoutingCard } from "./ai-routing";
import { AiTasksCard } from "./ai-tasks";
import { AiCallsCard } from "./aicalls";
import { AiUsageCard } from "./aiusage";
import { AutomationsAdmin } from "./automations";
import { AutonomySettingsCard } from "./autonomy-settings";
import { BlockedDomainsCard } from "./blocked-domains";
import { BriefDeliveryRows } from "./briefdelivery";
import { CaptureActivityTab } from "./capture-activity";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import { CaptureSendersCard } from "./capture-senders";
import {
  CaptureSettingsCard,
  MailSyncCard,
  WebsiteReadingCard,
} from "./capture-settings";
import {
  problemMessageOf,
  QueryGate,
  unwrap,
  useLogout,
  useMe,
} from "./common";
import { CompanyContextCard } from "./company-context";
import { ConnectedAgentsCard } from "./connected-agents";
import { ConnectorsCard } from "./connectors";
import { ConsumerMailDomainsCard } from "./consumer-mail-domains";
import { CustomFieldsAdmin } from "./customfields";
import { ExtensionAccessCard } from "./extension-access";
import { ExtensionUnitsCard } from "./extension-units";
import { FollowUpSettingsCard } from "./followupsettings";
import { HeldThreadsCard } from "./held-threads";
import { ImportCard } from "./import";
import { InstallationSettingsCard } from "./installation-settings";
import { ProviderCard } from "./integrations-provider";
import { KnowledgeCard } from "./knowledge";
import {
  LeadDisqualifyReasonsCard,
  LeadHandlingCard,
  LeadSourcesCard,
} from "./leadvocab";
import { LicenseCard } from "./license";
import { LinkedInImportCard } from "./linkedin-import";
import { LinkedInReachCard } from "./linkedin-reach";
import { MailSharingCard, MailSharingPostureRow } from "./mail-sharing";
import { MeetingSettings } from "./meeting-settings";
import { NotificationSettingsCard } from "./notification-settings";
import { OAuthAppCard } from "./oauth-app";
import { OfferTemplatesAdmin } from "./offertemplates";
import { OvernightGrantCard } from "./overnight-grant";
import { OwnDomainsCard } from "./own-domains";
import { PasswordSettingRow } from "./passwordcard";
import { ProductsAdmin } from "./products";
import { FxRatesCard, ModelCostsCard } from "./rates";
import { RecordRolesCard } from "./recordroles";
import { ReviewTemplatesCard } from "./reviewtemplates";
import { RolesSettings } from "./roles-settings";
import { PipelinesCard } from "./settings.pipelines";
import { PrivacyLanes } from "./settings.privacy";
import { StageAutomationCard } from "./settings.stageautomation";
import { SystemHealthPage } from "./settings.systemhealth";
import { AgentToolsCard } from "./settings-agenttools";
import { AuditLogCard } from "./settings-audit";
import { AutonomyTiersCard } from "./settings-autonomytiers";
import {
  DisplayNameSettingRow,
  GreetingNameSettingRow,
} from "./settings-names";
import { PassportCard } from "./settings-passports";
import { SignatureSettingRow } from "./settingssignaturerow";
import { SignInMethodsCard } from "./sign-in-methods";
import { SignatureTemplateCard } from "./signaturetemplatecard";
import { TagVocabularyCard } from "./tagadmin";
import { ThisDevicePanel } from "./thisdevice";
import { TeamsCard } from "./users-access";
import { UsersAdminCard } from "./users-admin";
import { VoiceDnaCard } from "./voice-dna";
import { WebhooksCard } from "./webhooks";
import "./settings.css";

import { ProvidersStat } from "./ai-settings";
import type { SettingsPageId } from "./settingscatalog";
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
      return (
        <>
          <AccountCard />
          <ThisDevicePanel />
        </>
      );
    case "meetings":
      return <MeetingSettings />;
    case "voice":
      return <VoiceDnaCard />;
    case "agents":
      return <AgentsTab />;
    // What the product may send this reader, and where. Beside the brief and
    // weekly nudges on Account rather than merged into them: those two rows are
    // about a digest the reader subscribes to, and these are about every notice
    // the product raises whether or not anybody asked for it.
    case "notifications":
      return <NotificationSettingsCard />;
    case "connections":
      return <ConnectionsTab />;
    // Beside `connections` and after it on purpose: that page says what you are
    // connected to, this one says what those connections did with your mail.
    case "capture-activity":
      return <CaptureActivityTab />;

    // ---- company ----
    case "company":
      // The installation's own facts, then the money, then the company profile
      // the AI reads, then the follow-up window. The currency pair stays
      // ADJACENT: the base currency is declared in InstallationSettingsCard and
      // every rate below converts to it.
      return (
        <>
          <InstallationSettingsCard />
          <FxRatesCard />
          <CompanyContextCard />
          <FollowUpSettingsCard />
          <SignatureTemplateCard />
        </>
      );
    case "authentication":
      // Split from the company profile, which is a different question with a
      // different reader: what the company IS, versus who may sign in to
      // it. The vendor OAuth apps sit with the sign-in methods because the same
      // OAuth client now serves sign-in as well as mailbox connection — filing
      // them under Capture said they belonged to one of the two.
      return (
        <>
          <SignInMethodsCard />
          <AgentConnectionsCard />
          <OAuthAppCard provider="google" />
          <OAuthAppCard provider="microsoft" />
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
          {/* The sharing rule first, because it is the widest thing on the page:
              whether captured mail is one seat's or the whole workspace's. It
              used to sit on the reader's own Connections page, where "Only you"
              was written over a switch that binds everybody. */}
          <MailSharingCard />
          {/* How often the mailboxes under that rule are read, before what is
              done with what they bring in. */}
          <MailSyncCard />
          {/* Then which domains are OURS, then what to do with mail from the
              rest, then which of the rest are consumer mailboxes — the posture,
              then the two judgements that read it. */}
          <OwnDomainsCard />
          <CaptureSettingsCard />
          <WebsiteReadingCard />
          <ConsumerMailDomainsCard />
          {/* Last, because it is the OUTCOME of the three above rather than a
              fourth rule: which domains ended up refused a company, and whether
              a machine or a human decided it. */}
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
    // The five-tab strip that used to hold these is gone. Its tabs shared ONE
    // address, which is why the routing card had to report its draft up to a
    // page that owned a confirm dialog: the app's unsaved guard watches
    // addresses and could not see a move between them. Each is an address now,
    // so the card claims the guard itself and the dialog is deleted.
    case "models":
      return (
        <>
          {/* Which vendors are answering, above the bindings that name them.
              It was a header reading on the old combined page and belongs with
              the lanes it qualifies: a binding to a vendor holding no key is
              the thing an operator came here to fix. */}
          <ProvidersStat />
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
          {/* The allowance with the month's spend in it, then where it went, then
              the prices the estimate is drawn from. */}
          <AiBudgetCard />
          <AiUsageCard />
          <ModelPriceDetails />
        </>
      );
    case "model-calls":
      return <AiCallsCard />;

    // ---- governance ----
    case "privacy":
      return <PrivacyLanes />;
    case "audit":
      // Split from the privacy page it used to end. The trail proves the
      // surfaces there were honoured, but it answers to `audit_log` where they
      // answer to the consent registry and the retention policy — so a reader
      // could hold the audit grant and be refused the page carrying it.
      return <AuditLogCard />;
    case "system-health":
      return <SystemHealthPage />;
    case "extensions":
      return <ExtensionAccessCard />;
    case "reset":
      // Its own page, and the reason is the ordering the old Maintenance page
      // could not express: a reindex, a queue reading and "empty the
      // installation" were three verbs on one screen, ascending in consequence
      // and separated by nothing.
      return <ResetDataCard />;
  }
}

// What YOU are connected to. Most surfaces here read a per-user seam — the
// mail connector list is scoped to the calling human server-side (capture is
// per-user, RC-8), and both LinkedIn surfaces read `/me`. So this belongs to the
// personal group, and needs no grant: a mailbox nobody else can see is not
// company configuration, and the entry that used to hold both kinds could
// not say so.
//
// It is not WHOLLY personal, which is why the catalog marks it `mixed`:
// ConnectorsCard's second panel is the workspace's Telegram bot, and the
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
      {/* Directly under the mailboxes and before what they brought in, because
          it changes what COUNTS as correspondence: an address declared here is
          the same contact, so mail among them is not a conversation with anybody
          and never becomes one. Per-seat rather than per-connection, which is
          why it is a card of its own and not a row inside connectors.tsx —
          those rows render once per mailbox and a seat's own addresses are one
          list however many mailboxes they connect. */}
      <OwnerIdentitiesCard />
      {/* Under the mailboxes, because it is what those mailboxes DID: every
          address they brought in, and what the classifier concluded about each.
          The posture rows above say what may be read; this says what was
          decided, which is the half a reader audits. */}
      <CaptureSendersCard />
      {/* And what those decisions are currently WITHHOLDING. The senders card
          above says what was decided about each correspondent; this says which
          threads are held back from the team right now, which is the question
          an outage makes urgent — every new thread lands pending and stays
          there until the classifier answers again. */}
      <HeldThreadsCard />
      {/* Directly under the mailboxes, because it is the same decision seen
          from the other side: those cards say what Margince may READ, this one
          says whether it may act on it overnight while nobody is watching. The
          rep was asked this once beside the very same connectors during
          onboarding — this is where that answer is found again. */}
      <OvernightGrantCard />
      <LinkedInImportCard />
      {/* No review queue here: a match a human must judge is a proposal, and
          proposals live in the approvals inbox. This shows what the import
          bought — which accounts the network reaches. */}
      <LinkedInReachCard />
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
      <WebhooksCard />
      {/* The other half of the units split: a unit whose secret is
          `workspace`-scoped holds the INSTALLATION's credential, like the two
          cards above it, so it is offered here and not on a member's own
          Connections page. Which page a unit lands on is its manifest's
          decision, never this file's. */}
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
export function SettingsScreen({ route }: Readonly<{ route: Route }>) {
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
  const readOnlyPage = reach.looksUp.some((page) => page.id === active?.id);
  // A legacy admin address is answered AND rewritten: the reader gets the page
  // they asked for, and the URL bar then says where that page lives, so the
  // link they copy from it is the current one. Replaced rather than pushed, or
  // Back would land on the address that redirects and trap them there.
  //
  // Keyed on the entry the route RESOLVED to rather than on the segment it
  // carried, so a rewrite never invents an address: a legacy link to a page the
  // reader may not open is left as typed, and the boundary answers it.
  useEffect(() => {
    if (legacy) {
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
        {tabContent(active.id, route)}
      </div>
    </div>
  );
}

// This contact's own agent authority: what an agent may do unattended, the
// credentials they have minted, the clients holding one, and the governed tools
// those credentials reach. Every seat gets it, ungated — a connection's
// authority comes from the human's own consent, so an admin-only surface here
// would mean only admins could connect a client.
function AgentsTab() {
  return (
    <>
      {/* The passports FIRST, because minting one is why a reader opens this
          page. The autonomy table used to stand above them: three locked,
          purely informational rows — nothing on it can be changed — sitting
          between the reader and the only control here. It is reference for the
          tiers the tools below are governed by, so it now reads after them. */}
      <PassportCard />
      {/* Directly after the passports, because it is the second half of one
          story: mint a passport for unattended use, or consent to a client
          that connects with its own fresh credential instead. */}
      <ConnectedAgentsCard />
      <AgentToolsCard />
      <AutonomyTiersCard />
      {/* Which kinds of proposal stop asking this reader. It sat on Account,
          under the identity, because it is a statement about them rather than
          about the company — but every other thing on this page is also
          theirs alone, and this is the page about agents deciding without them.
          Directly under the tier reference it is written in terms of. */}
      <AutonomySettingsCard />
    </>
  );
}

// Your account, as ONE card: who you are, and the three answers that belong to
// you — how you sign in, how you sign off, and which language the product
// speaks to you in.
//
// It used to be four panels with four header bands: Profile, Change password,
// Email signature, Preferences. Each held exactly one decision, so a reader
// auditing their own account read four titles to find three settings, and the
// answers sat at four different x. The identity block is the card's SUBJECT,
// drawn the way the product draws an identity everywhere else — the record
// page's own header block (composed.css `.record-head`): a mark, the name, the
// standing badges on the name's line, and the meta that qualifies it
// underneath. Everything below it is a decision ABOUT that subject, in the
// page's one row language.
function AccountCard() {
  const t = useT();
  const query = useMe();
  const logout = useLogout();
  // The card owns where an announcement lands, not the row that produced it: a
  // toast region standing between two rows in the list would take the
  // hairline the list draws between decisions.
  const toast = useToast();
  return (
    <Panel
      title={t("settings.accountCard")}
      actions={
        <Button disabled={logout.isPending} onClick={() => logout.mutate()}>
          {t("auth.signOut")}
        </Button>
      }
    >
      <PanelBody className="form-stack">
        <QueryGate query={query} pendingLabel={t("settings.accountCard")}>
          {(me) => (
            <div className="settings-identity">
              {/* Both halves are required on the wire, so the `|| ""` is not a
                  default — it is the promise that a server answering with
                  neither costs the reader an unnamed chip rather than the whole
                  page: this block renders inside the app shell, and a throw here
                  takes the navigation down with it. The chip is keyed on the
                  user id, like every other chip drawn for this seat. */}
              <Avatar
                identity={me.user.id}
                name={me.user.display_name || me.user.email || ""}
              />
              <div className="settings-identity-id">
                <div className="settings-identity-name">
                  <strong>{me.user.display_name || me.user.email}</strong>
                  {me.roles.map((role) => (
                    <RoleBadge key={role} roleKey={role} />
                  ))}
                </div>
                {/* The two lines that qualify the name: the address the session
                    is bound to, and the workspace it is bound to it IN. */}
                <span className="t-caption">{me.user.email}</span>
                <span className="t-caption">{me.workspace_name}</span>
              </div>
            </div>
          )}
        </QueryGate>
      </PanelBody>
      <SettingList bleed="settings">
        <DisplayNameSettingRow toast={toast} />
        <GreetingNameSettingRow toast={toast} />
        <PasswordSettingRow />
        <SignatureSettingRow toast={toast} />
        <LanguageSettingRow />
        <AppearanceSettingRow />
        <BriefDeliveryRows />
      </SettingList>
    </Panel>
  );
}

/**
 * The language this installation speaks to this reader in.
 *
 * Appearance stands beside it again, in the row below. It is ALSO in the account
 * menu — the setting a reader changes most often, from wherever they happen to
 * be standing — and the two are one state rather than two: both read
 * `useThemeChoice` and write `setThemeChoice`.
 *
 * One dropdown here, which is one row: the locale context is where the answer
 * lives, so nothing here is a second source of truth.
 */
/**
 * The appearance choice, in Settings as well as in the account menu.
 *
 * ONE state, two faces: both read `useThemeChoice` and write `setThemeChoice`
 * from app/theme.ts, so changing it in either place moves the other. The menu
 * keeps its shortcut — appearance is the setting a reader changes most often and
 * from wherever they are standing — and this is where somebody who came to
 * Settings looking for it finds it.
 */
function AppearanceSettingRow() {
  const t = useT();
  const choice = useThemeChoice();
  return (
    <SettingRow
      label={t("settings.appearance")}
      description={t("settings.appearanceHelp")}
      control={(control) => (
        <Select
          {...control}
          className="settingrow-measure"
          value={choice}
          // `Select` reports a string, narrowed back through the same list the
          // options were built from — no assertion, and nothing acted on that
          // the control was never offering.
          onChange={(next) => {
            const picked = THEME_CHOICES.find((option) => option === next);
            if (picked) {
              setThemeChoice(picked);
            }
          }}
          options={THEME_CHOICES.map((option) => ({
            value: option,
            label: t(THEME_LABEL_KEYS[option]),
          }))}
        />
      )}
    />
  );
}

function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  // The choice is written to the seat so it follows this colleague to their next
  // browser; `setLocale` still keeps its local copy, which is what renders
  // before the request lands and what a signed-out reader is left with.
  //
  // A failed write does NOT revert the language. The page is already in the
  // language they asked for, and yanking it back would be a worse answer to a
  // dropped request than letting the next sign-in re-ask the server.
  const remember = useMutation({
    mutationFn: async (next: Locale) => {
      unwrap(await api.PUT("/me/locale", { body: { locale: next } }), t);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  return (
    <SettingRow
      label={t("locale.switchLabel")}
      description={t("settings.languageHelp")}
      control={(control) => (
        <Select
          {...control}
          className="settingrow-measure"
          value={locale}
          // `Select` reports a string. The options are built from LOCALES,
          // so narrowing the answer through that same list is what makes
          // it a Locale — no assertion, and nothing acted on that the
          // control was never offering.
          onChange={(next) => {
            const picked = LOCALES.find((option) => option === next);
            if (picked) {
              setLocale(picked);
              remember.mutate(picked);
            }
          }}
          // Language names are proper nouns and deliberately not
          // translated, so every option here is in a different language
          // from the page around it — WCAG 2.2 AA 3.1.2, the same reason
          // the login footer's switcher carries `lang` on each name. Our
          // locale codes are BCP 47 language subtags, so the code IS the
          // value `lang` wants.
          options={LOCALES.map((option) => ({
            value: option,
            label: t(localeNameKey(option)),
            lang: option,
          }))}
        />
      )}
    />
  );
}

// The danger-zone reset action: wipes the installation back to its first-boot
// state. Double-gated client-side — `system_reset:delete` AND the server-driven
// `data_reset_available` flag on /me (never VITE_UI_PREVIEW_RESET, which is the
// unrelated password-reset link) — so the affordance is invisible unless the
// deployment armed the capability; the server gates the endpoint on that same
// value and 404s it otherwise, regardless of what this card renders.
//
// Two conditions of different kinds, and it needs both: the grant says this
// reader may wipe an installation that permits wiping, the flag says whether
// this one does. The compiled default is false in every posture.
//
// The GRANT, not the admin role. compose/datareset.go asks
// auth.Require(system_reset, delete) — it read auth.RequireAdmin while no
// object named the verb, and this comment outlived that. A role edited to carry
// the verb reaches the control and one that lost it does not, which the role
// name could not say either way. The page above it now asks the same thing, so
// a reader who gets here can use it. The company's name
// is not carried on MeResponse, so this never fetches or compares it
// client-side: the input just has to be non-empty to enable the confirm
// button, and the server is the sole judge of whether the typed text actually
// matches (a mismatch comes back as a 422, surfaced verbatim in the dialog).
// The full reset response — derived from the generated operation type
// (T6: no `as`, no hand-duplicated field list) so a wire change that adds or
// renames a counter fails typecheck here instead of silently going unshown.
type ResetSummary =
  operations["resetData"]["responses"][200]["content"]["application/json"];

function ResetDataCard() {
  const t = useT();
  const { locale } = useLocale();
  const me = useMe();
  // `system_reset:delete`, which is what POST /admin/reset-data asks for
  // (compose/datareset.go). The literal admin role guarded it while no object
  // named the verb; one does now, so a role edited to carry it reaches the
  // control and one that lost it does not — which the role name could not say.
  //
  // `useCanWrite`: the reset is a POST, and a read seat is refused every
  // mutation by the seat ceiling above RBAC (identity/admission.go) whatever
  // its grants say. Offering the danger zone to one would be a button that
  // types the workspace name and then 403s.
  const canSee = useCanWrite("system_reset", "delete");
  const workspaceName = me.data?.workspace_name ?? "";
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  // What the last reset actually cleared — null until one has run, so the
  // danger zone stays quiet on first render rather than implying a result
  // nobody triggered.
  const [summary, setSummary] = useState<ResetSummary | null>(null);
  const queryClient = useQueryClient();

  const reset = useMutation({
    mutationFn: async () => {
      // The summary always describes the latest attempt, never a prior one:
      // clearing here means a retry's error can never leave a previous
      // success sitting on screen, and an in-flight retry shows no summary.
      setSummary(null);
      return unwrap(
        await api.POST("/admin/reset-data", {
          body: { confirmation: typed },
        }),
      );
    },
    onSuccess: (data) => {
      setOpen(false);
      setTyped("");
      setSummary(data ?? null);
      // A reset wipes every domain table for the workspace — every cached
      // list/detail query is stale, not just the ones this card knows about.
      queryClient.invalidateQueries();
    },
  });

  if (!canSee || !me.data?.data_reset_available) {
    return null;
  }

  return (
    <Panel
      title={t("settings.dangerZone")}
      // The one card that announces its own danger: the red border is what
      // separates a destructive surface from the ordinary settings around it.
      className="settings-danger"
    >
      <PanelBody className="form-stack">
        <PanelIntro>{t("settings.dangerZoneSub")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        {/* This button and the dialog's confirm share the screen, so each is
            named for its own act. */}
        <SettingRow
          label={t("settings.resetDataLabel")}
          description={t("settings.resetDataDesc")}
          control={
            <Button variant="danger" onClick={() => setOpen(true)}>
              {t("settings.resetDataButton")}
            </Button>
          }
        />
      </SettingList>
      {summary && (
        <PanelBody>
          <p className="t-caption settings-danger-result" role="status">
            {t("settings.resetDataResult", {
              tables: formatNumber(summary.tables_cleared, locale),
              jobs: formatNumber(summary.jobs_deleted, locale),
              streams: formatNumber(summary.streams_purged, locale),
              keys: formatNumber(summary.cache_keys_deleted, locale),
              objects: formatNumber(summary.objects_deleted, locale),
            })}
          </p>
          {summary.drain_timed_out && (
            // ds:ignore a warning in --warningText, not a refusal
            <p className="settings-danger-warning" role="alert">
              {t("settings.resetDataDrainWarning")}
            </p>
          )}
        </PanelBody>
      )}
      <ConfirmModal
        open={open}
        onClose={() => {
          setOpen(false);
          setTyped("");
          reset.reset();
        }}
        title={t("settings.resetDataConfirmTitle")}
        confirmLabel={t("settings.resetDataConfirmButton")}
        confirmVariant="danger"
        // The typed confirmation gates whether the reset may START; the input
        // is still editable while it runs, and a reader who clears it mid-write
        // would otherwise re-arm the gate on a control that is already going.
        confirmDisabled={!reset.isPending && typed.trim() === ""}
        onConfirm={() => reset.mutate()}
        pending={reset.isPending}
        error={reset.error ? problemMessageOf(reset.error, t) : null}
      >
        <p>{t("settings.resetDataConfirmBody")}</p>
        {workspaceName ? (
          <p>
            {t("settings.resetDataConfirmName")}{" "}
            {/* userSelect:all lets one click select the whole name to copy */}
            <code style={{ userSelect: "all" }}>{workspaceName}</code>
          </p>
        ) : null}
        <TextInput
          aria-label={t("settings.resetDataConfirmLabel")}
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
        />
      </ConfirmModal>
    </Panel>
  );
}

function ModelPriceDetails() {
  const t = useT();
  return (
    <Disclosure summary={t("aiRouting.priceSheet")}>
      <ModelCostsCard />
    </Disclosure>
  );
}
