import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { operations } from "../api/schema";
import { Button, Disclosure } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../design-system/confirmphrase";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { problemMessageOf, unwrap } from "../lib/problem";
import { navigateReplacing, type Route } from "../lib/router";
import { useMe } from "../lib/session";
import { AccountCard } from "./account-settings";
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
import { CaptureActivityTab } from "./capture-activity";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import {
  CaptureSettingsCard,
  MailSyncCard,
  WebsiteReadingCard,
} from "./capture-settings";
import { CompanyContextCard } from "./company-context";
import { ConnectedAgentsCard } from "./connected-agents";
import { ConnectorsCard } from "./connectors";
import { TelegramConnectorsPanel } from "./connectors.telegram";
import { ConsumerMailDomainsCard } from "./consumer-mail-domains";
import { CustomFieldsAdmin } from "./customfields";
import { ExtensionAccessCard } from "./extension-access";
import { ExtensionUnitsCard } from "./extension-units";
import { FollowUpSettingsCard } from "./followupsettings";
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
import { LinkedInNetworkPanel } from "./linkedin-import";
import { MailSharingCard } from "./mail-sharing";
import { MeetingSettings } from "./meeting-settings";
import { NotificationSettingsCard } from "./notification-settings";
import { OAuthAppsCard } from "./oauth-apps-card";
import { OfferTemplatesAdmin } from "./offertemplates";
import { OvernightGrantCard } from "./overnight-grant";
import { OwnDomainsCard } from "./own-domains";
import { ProductsAdmin } from "./products";
import { PublicOriginCard } from "./public-origin";
import { FxRatesCard, ModelCostsCard } from "./rates";
import { RecordRolesCard } from "./recordroles";
import { ReviewTemplatesCard } from "./reviewtemplates";
import { RolesSettings } from "./roles-settings";
import { JurisdictionCard } from "./settings.jurisdiction";
import { PipelinesCard } from "./settings.pipelines";
import { PrivacyLanes } from "./settings.privacy";
import { StageAutomationCard } from "./settings.stageautomation";
import { SystemHealthPage } from "./settings.systemhealth";
import { AgentToolsCard } from "./settings-agenttools";
import { AuditLogCard } from "./settings-audit";
import { AutonomyTiersCard } from "./settings-autonomytiers";
import { PassportCard } from "./settings-passports";
import { SignInMethodsCard } from "./sign-in-methods";
import { SignatureTemplateCard } from "./signaturetemplatecard";
import { TagVocabularyCard } from "./tagadmin";
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
      return <AccountCard />;
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
      // The apps come first: one OAuth client serves mail, calendar and sign-in,
      // so no provider reaches the sign-in screen before its app exists.
      return (
        <>
          <OAuthAppsCard />
          <SignInMethodsCard />
          <AgentConnectionsCard />
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
    case "jurisdiction":
      return <JurisdictionCard />;
    case "audit":
      // Its own page: it answers to `audit_log`, where the privacy page answers
      // to consent and retention, and one reader can hold one without the other.
      return <AuditLogCard />;
    case "system-health":
      return (
        <>
          <SystemHealthPage />
          <PublicOriginCard />
        </>
      );
    case "extensions":
      return <ExtensionAccessCard />;
    case "reset":
      // Its own page: a reindex, a queue reading and "empty the installation"
      // were three verbs on one screen, separated by nothing.
      return <ResetDataCard />;
  }
}

// What YOU are connected to: the mail connector list is scoped to the calling
// human server-side (capture is per-user, RC-8) and both LinkedIn surfaces read
// `/me`, so this is personal and needs no grant. What those mailboxes brought in
// and what is held back is on Capture activity.
function ConnectionsTab() {
  return (
    <>
      <ConnectorsCard />
      {/* Directly under the mailboxes, because it changes what COUNTS as
          correspondence: mail among a seat's own addresses is never a
          conversation. Per seat, not per mailbox, so a card of its own. */}
      <OwnerIdentitiesCard />
      {/* The same decision from the other side: the cards above say what
          Margince may READ, this one whether it may act on it overnight. */}
      <OvernightGrantCard />
      {/* No review queue here: a match a human must judge is a proposal, and
          proposals live in the approvals inbox. */}
      <LinkedInNetworkPanel />
      {/* Last, and only when the installation composed a unit whose
          credential is the member's OWN (a `user`-scoped secret). */}
      <ExtensionUnitsCard scope="user" />
    </>
  );
}

// What the INSTALLATION is wired to: one shared contact-data credential, the
// outbound subscriptions and the workspace's Telegram bot. Each is shared by
// everybody, which is why they sit under the company heading.
function IntegrationsTab() {
  return (
    <>
      <ProviderCard />
      <WebhooksCard />
      <TelegramConnectorsPanel />
      {/* The other half of the units split: a unit whose secret is
          `workspace`-scoped holds the INSTALLATION's credential, like the cards
          above it, so it is offered here and not on a member's own
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
// The GRANT, not the admin role: compose/datareset.go asks
// auth.Require(system_reset, delete), so a role edited to carry the verb
// reaches the control and one that lost it does not.
//
// The phrase is me.workspace_name, the Name setting the server compares
// against, and the confirm sends that name rather than the typed text, so a
// stray space cannot become a 422. A rename in another tab still ends in one,
// shown verbatim in the dialog.
//
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
  const phraseInput = useRef<HTMLInputElement>(null);
  // What the last reset actually cleared — null until one has run, so the
  // danger zone stays quiet on first render rather than implying a result
  // nobody triggered.
  const [summary, setSummary] = useState<ResetSummary | null>(null);
  const queryClient = useQueryClient();

  const reset = useMutation({
    mutationFn: async (confirmation: string) => {
      // The summary always describes the latest attempt, never a prior one:
      // clearing here means a retry's error can never leave a previous
      // success sitting on screen, and an in-flight retry shows no summary.
      setSummary(null);
      return unwrap(
        await api.POST("/admin/reset-data", {
          body: { confirmation },
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
        confirmDisabled={
          !reset.isPending && !phraseMatches(typed, workspaceName)
        }
        onConfirm={() => reset.mutate(workspaceName)}
        pending={reset.isPending}
        error={reset.error ? problemMessageOf(reset.error, t) : null}
        initialFocusTo={() => phraseInput.current}
      >
        <p>{t("settings.resetDataConfirmBody")}</p>
        <ConfirmPhraseBox
          prompt={t("settings.resetDataConfirmName")}
          phrase={workspaceName}
          value={typed}
          onChange={setTyped}
          inputRef={phraseInput}
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
