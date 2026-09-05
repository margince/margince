import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components, operations } from "../api/schema";
import { dotTier } from "../app/autonomy";
import { useCanWrite, useHoldsAdminRole } from "../app/capability";
import { isEntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { navigateReplacing, type Route } from "../app/router";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Avatar,
  Badge,
  Button,
  Checkbox,
  Disclosure,
  EmptyState,
  Field,
  Modal,
  Skeleton,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody, PanelPlate } from "../design-system/panel";
import {
  PassportSelect,
  ScopeChips,
  scopeChipLabel,
} from "../design-system/passportselect";
import { FieldGuard, RoleBadge } from "../design-system/rbac";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { type Toast, useToast } from "../design-system/toast";
import {
  AutonomyDot,
  EvidenceChip,
  FieldDiff,
  PassportChip,
  toEvidence,
} from "../design-system/trust";
import { stable } from "../format/collate";
import { formatDate, formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LOCALES, type Locale, localeNameKey, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { AiSettingsTab } from "./ai-settings";
import { ActorTag } from "./audit";
import { AutonomySettingsCard } from "./autonomy-settings";
import { BlockedDomainsCard } from "./blocked-domains";
import { BriefDeliveryRows } from "./briefdelivery";
import { CaptureActivityTab } from "./capture-activity";
import { OwnerIdentitiesCard } from "./capture-owner-identities";
import { CaptureSendersCard } from "./capture-senders";
import { CaptureSettingsCard } from "./capture-settings";
import {
  LoadMoreButton,
  problemMessageOf,
  QueryGate,
  resetToSignedOut,
  throwProblem,
  useLogout,
  useMe,
} from "./common";
import { CompanyContextCard } from "./company-context";
import { ConnectedAgentsCard } from "./connected-agents";
import { ConnectorsCard } from "./connectors";
import { ConsumerMailDomainsCard } from "./consumer-mail-domains";
import { CreateAction, type CreateField, CreateRecordModal } from "./create";
import { CustomFieldsAdmin } from "./customfields";
import { EditAction } from "./edit";
import { EmbedReindexCard } from "./embedreindex";
import { EntityRef } from "./entityref";
import { ExtensionAccessCard } from "./extension-access";
import { ExtensionUnitsCard } from "./extension-units";
import { HeldThreadsCard } from "./held-threads";
import { ImportCard } from "./import";
import { InstallationSettingsCard } from "./installation-settings";
import { ProviderCard } from "./integrations-provider";
import { JobHealthCard } from "./jobhealth";
import { KnowledgeCard } from "./knowledge";
import {
  LeadDisqualifyReasonsCard,
  LeadHandlingCard,
  LeadSourcesCard,
} from "./leadvocab";
import { LicenseCard } from "./license";
import { LinkedInImportCard } from "./linkedin-import";
import { LinkedInReachCard } from "./linkedin-reach";
import { SEARCH_DEBOUNCE_MS } from "./listquery";
import { MailSharingCard } from "./mail-sharing";
import { OAuthAppCard } from "./oauth-app";
import { OfferTemplatesAdmin } from "./offertemplates";
import { OverlayCard } from "./overlay";
import { MirrorUserMapCard } from "./overlay-usermap";
import { OvernightGrantCard } from "./overnight-grant";
import { OwnDomainsCard } from "./own-domains";
import { PasswordSettingRow } from "./passwordcard";
import { ConsentPurposesCard, PrivacyInboxCard } from "./privacy";
import { ProductsAdmin } from "./products";
import { FxRatesCard } from "./rates";
import { RestrictedRecordsCard } from "./restrictedrecords";
import { RetentionCard } from "./retention";
import { SignInMethodsCard } from "./sign-in-methods";
import { TagVocabularyCard } from "./tagadmin";
import { TeamsCard } from "./users-access";
import { UsersAdminCard } from "./users-admin";
import { VoiceDnaCard } from "./voice-dna";
import { WebhooksCard } from "./webhooks";
import "./settings.css";

// The catalog, the addresses and the visibility predicate moved to
// ./settingsnav so `src/app/**` can read them without pulling in every card.
// Re-exported here because this module's own consumers — the tests, the stories,
// the testkit — ask for both halves, and splitting their imports would be churn
// that proves nothing.
import {
  ADMIN_SEGMENT,
  SETTINGS_SCREEN,
  SETTINGS_TABS,
  type SettingsTabId,
  settingsAddress,
  settingsRouteTab,
  useSettingsEntryVisibility,
  useSettingsSection,
  useVisibleSettingsTabs,
} from "./settingsnav";

// Re-exported so this module's own consumers — the tests, the stories, the
// testkit — keep asking one module for both halves. Splitting their imports
// would be churn that proves nothing about the split that matters, which is
// `src/app/**` no longer reaching the cards.
export {
  ADMIN_SEGMENT,
  SETTINGS_SCREEN,
  SETTINGS_TABS,
  type SettingsTabId,
  settingsAddress,
  settingsRouteTab,
  useSettingsEntryVisibility,
  useSettingsSection,
};

// Settings governance surface (B-EP09.13b): renders FROM the live seams —
// /me (identity + effective roles), passports (mint + the metadata list,
// token shown once and never re-disclosed), consent purposes (DOI flags),
// the privacy inbox (DSRs + statutory deadlines), the attributable
// audit-log view with live filters — plus the locked autonomy-tier table
// and the automations the installation runs unattended. EP09 renders
// governance; it never authors policy.

export function tabContent(id: SettingsTabId): ReactNode {
  switch (id) {
    case "account":
      return (
        <>
          <AccountCard />
          {/* Under the identity because it is a statement about this reader
              rather than about the workspace: which kinds of proposal stop
              asking them. No admin card belongs on this tab, and this one is
              not an exception — nobody else sets it. */}
          <AutonomySettingsCard />
        </>
      );
    case "voice":
      return <VoiceDnaCard />;
    case "agents":
      return <AgentsTab />;
    case "general":
      // The installation's own facts, then the money, then the company profile
      // the AI reads. The currency pair stays ADJACENT and nothing is allowed
      // between them: the base currency is declared in the second card of
      // InstallationSettingsCard and every rate below converts to it, and
      // before they were merged the lock reason was explained on one tab while
      // the consequence landed on another.
      //
      // The vendor OAuth apps go LAST for that reason, not because it matters least.
      // It is here at all because the same OAuth client now serves sign-in as
      // well as mailbox connection, so filing it under Capture said it belonged
      // to one of the two.
      return (
        <>
          <InstallationSettingsCard />
          <FxRatesCard />
          <CompanyContextCard />
          <SignInMethodsCard />
          <OAuthAppCard provider="google" />
          <OAuthAppCard provider="microsoft" />
        </>
      );
    case "extensions":
      // Its own entry rather than a third card under Users & teams: that page
      // answers who holds which role, and this one answers what an installed
      // unit may reach — a question about the installation's software, not
      // about its people. They shared a page while the extension tier had one
      // unit and no page of its own.
      return <ExtensionAccessCard />;
    case "users":
      return (
        <>
          <UsersAdminCard />
          {/* Teams are share targets and a way to address a group of users.
              Membership alone still grants nothing to most roles, but Team
              Lead is team-scoped (RowScopeTeam) — so this card is also where
              an admin decides whose records a Team Lead's membership hands
              over, not merely a way to address a group. */}
          <TeamsCard />
        </>
      );
    case "connections":
      return <ConnectionsTab />;
    // Beside `connections` and after it on purpose: that tab says what you are
    // connected to, this one says what those connections did with your mail.
    case "capture-activity":
      return <CaptureActivityTab />;
    case "integrations":
      return <IntegrationsTab />;
    case "capture":
      return (
        <>
          {/* Which domains are OURS, then what to do with mail from the rest,
              then which of the rest are consumer mailboxes — the posture, then
              the two judgements that read it. Before this they sat on two
              different tabs that shared the word "Capture" and neither said
              which one the other meant. */}
          <OwnDomainsCard />
          <CaptureSettingsCard />
          <ConsumerMailDomainsCard />
          {/* Last, because it is the OUTCOME of the three above rather than a
              fourth rule: which domains ended up refused a company, and whether
              a machine or a person decided it. An operator hunting a company
              that never arrived reads down the page and finishes here. */}
          <BlockedDomainsCard />
        </>
      );
    case "data-model":
      return <DataModelTab />;
    case "knowledge":
      return <KnowledgeCard />;
    case "ai":
      return <AiSettingsTab />;
    case "privacy":
      return (
        <>
          <ConsentPurposesCard />
          {/* The retention ladder sits under the purpose catalogue and above
              the DSR inbox: what the installation keeps by default, before the
              requests that override it case by case. Admin/ops in substance, and
              it says so without the grant rather than vanishing — every card on
              this page now behaves the same way, which is the point: three
              different answers to one denial on one page is what made it
              unreadable. */}
          <RetentionCard />
          {/* What the ladder's statutory floor is holding right now, under the
              ladder that explains why: an erasure that met a Handelsbrief
              restricted it rather than destroying it, and the controller has
              to be able to see that without opening the audit trail. */}
          <RestrictedRecordsCard />
          <PrivacyInboxCard />
          {/* Last, and on the same page: the trail is what proves the three
              surfaces above it were honoured. It gates itself on the admin
              role, which the purpose registry above does not. */}
          <AuditLogCard />
        </>
      );
    case "license":
      return <LicenseCard />;
    case "maintenance":
      return (
        <>
          {/* Three operational verbs, in ascending order of consequence: a
              reindex that costs tokens, a read of what the background system is
              holding, and a reset that empties the installation. They hid beside
              the custom-field editor before, which put "define a field" and
              "delete everything" on one page. Job health had no surface at all —
              an operator watching a stalled queue had nothing to look at. */}
          <ImportCard />
          <EmbedReindexCard />
          <JobHealthCard />
          <ResetDataCard />
        </>
      );
  }
}

// What YOU are connected to. Every surface here reads a per-user seam — the
// connector list is scoped to the calling human server-side (capture is per-user,
// RC-8), and both LinkedIn surfaces read `/me`. So this belongs to the personal
// group, and needs no grant: a mailbox nobody else can see is not organization
// configuration, and the entry that used to hold both kinds could not say so.
function ConnectionsTab() {
  return (
    <>
      {/* The rule first, then the mailboxes that live under it: sharing is a
          workspace posture every user works under, not a property of any one
          connection below. */}
      <MailSharingCard />
      <ConnectorsCard />
      {/* Directly under the mailboxes and before what they brought in, because
          it changes what COUNTS as correspondence: an address declared here is
          the same person, so mail among them is not a conversation with anybody
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

// What the INSTALLATION is wired to: one shared contact-data credential, the
// outbound subscriptions, the incumbent CRM it mirrors, and who each of its users
// is over there. All four are workspace-wide — a key everybody spends from, a webhook everybody's writes
// fire, a system-of-record flip that re-points every read — which is why they
// sit under the organization heading and the personal connections do not.
function IntegrationsTab() {
  return (
    <>
      <ProviderCard />
      <WebhooksCard />
      {/* Everything overlay — connect, live sync/budget health (OverlayCard
          renders OverlayLiveSection itself once a connection is active or in
          error, so it is not rendered a second time here), and the user
          mapping. Deliberately NOT gated on useSorMode() === "overlay": a
          workspace is native until an overlay is connected, so mode-gating
          would hide the only surface that can connect one. In native mode
          OverlayCard renders its connect form and the rest stays quiet. */}
      <OverlayCard />
      <MirrorUserMapCard />
      {/* The other half of the units split: a unit whose secret is
          `workspace`-scoped holds the INSTALLATION's credential, like the four
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
function DataModelTab() {
  return (
    <>
      <CustomFieldsAdmin />
      <TagVocabularyCard />
      <PipelinesCard />
      <LeadSourcesCard />
      <LeadDisqualifyReasonsCard />
      <LeadHandlingCard />
      <ProductsAdmin />
      <OfferTemplatesAdmin />
    </>
  );
}

/**
 * The route segment every Admin settings entry sits under.
 *
 * `#/settings/admin/privacy` rather than `#/settings/privacy`, so the address
 * says which half of settings a reader is in — the same thing the panel heading
 * says, and the thing a link pasted into a channel could not say before. The
 * personal entries keep their own bare addresses: they are what a reader who
 * types `#/settings/voice` means, and moving them too would break every
 * bookmark to buy symmetry nobody reads.
 *
 * A legacy `#/settings/privacy` still resolves — see `settingsRouteTab` — and
 * is rewritten to the address above, so nothing that exists today lands
 * nowhere.
 */

export function SettingsScreen({ route }: Readonly<{ route: Route }>) {
  const { tab, legacy } = settingsRouteTab(route);
  const { active } = useVisibleSettingsTabs(tab);
  // A legacy admin address is answered AND rewritten: the reader gets the page
  // they asked for, and the URL bar then says where that page lives, so the
  // link they copy from it is the current one. Replaced rather than pushed, or
  // Back would land on the address that redirects and trap them there.
  //
  // Keyed on the entry the route RESOLVED to rather than on the segment it
  // carried, so a rewrite never invents an address: a rep following a link to
  // an admin page falls back to their first visible entry, and this rewrites to
  // THAT, which is where they actually are.
  useEffect(() => {
    if (legacy) {
      navigateReplacing(settingsAddress(active.id));
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
  return (
    <div className="wrap">
      {/* Unsaved drafts in here are held by the guard above the routed screen
          (App.tsx), not by this screen. A guard installed HERE could only see
          moves between settings entries: it unmounts with the screen, so a draft
          was safe from one tab to the next and still discarded without a word the
          moment the reader clicked Contacts. The cards below claim through
          `useUnsavedGuard` and need to know nothing about where the answer is
          asked. */}
      <div className="settings-stack arrive-stack">{tabContent(active.id)}</div>
    </div>
  );
}

// This person's own agent authority: what an agent may do unattended, the
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
      <AutonomyCard />
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
        <Button
          small
          disabled={logout.isPending}
          onClick={() => logout.mutate()}
        >
          {t("auth.signOut")}
        </Button>
      }
    >
      <PanelBody className="form-stack">
        <QueryGate query={query} pendingLabel={t("settings.accountCard")}>
          {(me) => (
            <div className="settings-identity">
              {/* Both halves are required on the wire, so the `?? ""` is not a
                  default — it is the promise that a server answering with
                  neither costs the reader an unnamed chip rather than the whole
                  page: this block renders inside the app shell, and a throw here
                  takes the navigation down with it. */}
              {/* The address is the tint's key, so this reader keeps the same
                  colour here as in the rail's account block and as their turns
                  in the onboarding transcript — and keeps it after they change
                  their display name. */}
              <Avatar
                identity={me.user.email || undefined}
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
                <span className="t-small">{me.user.email}</span>
                <span className="t-small">{me.workspace_name}</span>
              </div>
            </div>
          )}
        </QueryGate>
        <SettingList>
          {/* The credential first, because it is the one row that decides
              whether the other two are reachable at all. The row and its
              three-field form live in passwordcard.tsx, exported as a ROW
              precisely so this page can place it among its own. */}
          <PasswordSettingRow />
          <SignatureSettingRow toast={toast} />
          <LanguageSettingRow />
          <BriefDeliveryRows />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

// The sign-off appended below every message this member sends, as one row.
//
// It lives beside identity rather than under the composer because it is who
// the sender IS, not something about one mail: a signature written per message
// would be a different signature every time, which is the opposite of what one
// is for. Plain text, because the transport sends text/plain — markup here
// would arrive as tags in the message.
//
// A textarea committed with a Save button is the settings page's MODAL case,
// not its row case: the row states what the setting is and what it currently
// says, and the verb opens the form. The row's answer is the sign-off's first
// line, which is the part a reader recognises their own signature by.
function SignatureSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  const [body, setBody] = useState<string | null>(null);
  const signature = useQuery({
    queryKey: ["me-email-signature"],
    queryFn: async () => {
      const { data, error } = await api.GET("/me/email-signature");
      if (error) {
        throwProblem(error);
      }
      return data ?? { body: "" };
    },
  });
  const save = useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/email-signature", {
        body: { body: next },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (saved) => {
      // Hand the edit back to the server's answer. It trims what it stores, so
      // a member who typed trailing spaces would otherwise keep seeing them
      // over a row that no longer has them — with Save still lit, offering to
      // save a difference that exists only in the browser.
      setBody(saved?.body ?? "");
      queryClient.invalidateQueries({ queryKey: ["me-email-signature"] });
      // Committing the edit is what the dialog was opened for, so a save closes
      // it — and the toast is what says the write landed, on the page the
      // reader is handed back to.
      setOpen(false);
      toast.show(t("settings.saved"));
    },
  });

  // The saved value until the member types; theirs from then on. Reading state
  // straight from the query would discard every keystroke the moment a refetch
  // landed underneath them.
  const stored = signature.data?.body ?? "";
  const shown = body ?? stored;
  // The same comparison the Save button already made, now also the claim that
  // stops a sidebar click throwing the draft away.
  const dirty = shown !== stored;
  useUnsavedGuard(dirty);
  // The first line, because a sign-off is several lines and only the first one
  // identifies it. `.split` on a string always yields at least one element, so
  // the empty signature reads as the empty string and the row says so instead —
  // but only once the read has answered: "no sign-off set" while the request is
  // still out is a claim about the member's signature made before anybody knows
  // what it is.
  const firstLine = stored.split("\n")[0].trim();
  const answer = signature.isPending
    ? undefined
    : firstLine === ""
      ? t("settings.signatureNone")
      : firstLine;

  // Leaving discards the draft rather than keeping it: the reader closed the
  // form, and a sign-off half-typed into a dialog nobody reopened is not an
  // edit anybody is coming back to.
  const close = () => {
    setBody(null);
    save.reset();
    setOpen(false);
  };

  return (
    <>
      <SettingRow
        label={t("settings.signature")}
        description={t("settings.signatureSub")}
        value={answer}
        control={
          <Button small variant="ghost" onClick={() => setOpen(true)}>
            {t("settings.signatureEdit")}
          </Button>
        }
      />
      {open && (
        <Modal open onClose={close} labelledBy={titleId}>
          {/* A real form, so Enter from the field commits it — and the Save
              button keeps the semantics it had as a card action: nothing is
              written until it is pressed. */}
          <form
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              if (dirty && !save.isPending) save.mutate(shown);
            }}
          >
            <h2 className="t-h3 modal-title" id={titleId}>
              {t("settings.signature")}
            </h2>
            {save.isError && (
              <Callout tone="danger" live="alert">
                {problemMessageOf(save.error, t)}
              </Callout>
            )}
            <Field label={t("settings.signatureLabel")}>
              {(control) => (
                <Textarea
                  {...control}
                  rows={5}
                  value={shown}
                  placeholder={t("settings.signaturePlaceholder")}
                  onChange={(event) => setBody(event.target.value)}
                />
              )}
            </Field>
            <p className="t-caption">{t("settings.signatureHint")}</p>
            <div className="form-actions">
              <Button small variant="ghost" onClick={close}>
                {t("settings.signatureCancel")}
              </Button>
              <Button
                small
                type="submit"
                variant="primary"
                disabled={!save.isPending && !dirty}
                pending={save.isPending}
                busyLabel={t("settings.signatureSaving")}
              >
                {t("record.save")}
              </Button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

/**
 * The language this installation speaks to this reader in.
 *
 * Appearance used to stand beside it and is now picked from the ACCOUNT MENU:
 * it is the setting a reader changes most often and from wherever they happen
 * to be standing, so it lives where they already are rather than three screens
 * away. What is left is one dropdown, which is one row — the locale context is
 * where the answer lives, so nothing here is a second source of truth.
 */
function LanguageSettingRow() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  const queryClient = useQueryClient();
  // The choice is written to the seat so it follows this person to their next
  // browser; `setLocale` still keeps its local copy, which is what renders
  // before the request lands and what a signed-out reader is left with.
  //
  // A failed write does NOT revert the language. The page is already in the
  // language they asked for, and yanking it back would be a worse answer to a
  // dropped request than letting the next sign-in re-ask the server.
  const remember = useMutation({
    mutationFn: async (next: Locale) => {
      const { error } = await api.PUT("/me/locale", { body: { locale: next } });
      if (error) {
        throwProblem(error, t);
      }
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

const PASSPORT_SCOPES = ["read", "draft", "write", "send", "enrich"] as const;

// The scope's wire token is what the server reads; a person choosing what
// authority to hand their agent needs the sentence. Composed rather than
// switched, and annotated so an added scope is a missing-key compile error
// rather than a checkbox that quietly labels itself `enrich` in every
// language.
function scopeLabelKey(scope: (typeof PASSPORT_SCOPES)[number]): MessageKey {
  return `passport.scope.${scope}`;
}

function PassportCard() {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [label, setLabel] = useState("");
  const [scopes, setScopes] = useState<Set<string>>(new Set(["read", "draft"]));
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [minting, setMinting] = useState(false);
  const revokingRow = useRef<HTMLElement | null>(null);
  // Where the minted token lands. It is a live region that is ALWAYS mounted
  // for the drawer's whole life rather than one that appears with the token in
  // it: a region inserted at the same moment as its content is not reliably
  // announced, and this token is shown exactly once in its life.
  const tokenRegion = useRef<HTMLDivElement | null>(null);
  const mintTitleId = useId();
  const mintScopeHintId = useId();

  // Metadata only — the wire schema carries no token (PassportSummary),
  // so this list cannot re-disclose one.
  const list = useQuery({
    queryKey: ["passports"],
    queryFn: async () => {
      const { data, error } = await api.GET("/passports");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const mint = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.POST("/passports", {
        body: {
          label: label.trim() || null,
          scopes: [...scopes] as (
            | "read"
            | "draft"
            | "write"
            | "send"
            | "enrich"
          )[],
        },
      });
      // An expired session must not read as a mint that did nothing. The `me`
      // probe is cached for five minutes, so without this the screen keeps
      // believing it is signed in and the button simply fails in silence —
      // which is how the OAuth consent screen's empty-passport guide became an
      // inescapable loop: it sends the human here to mint, the mint 401s
      // without saying so, and returning finds no passport and shows the same
      // guide again.
      //
      // Keyed on the STATUS rather than on `error` being truthy: a non-2xx
      // with no body leaves `error` undefined in this client (compose.tsx
      // documents the same shape), and that response would otherwise pass as
      // a success and then render an undefined token.
      if (response.status === 401) {
        // useLogout's order, for its reason: drop every other cached answer
        // BEFORE resetting the probe. AuthGate restores this route after the
        // next sign-in, and whoever signs in then must not be shown the
        // previous session's passport list out of cache.
        await resetToSignedOut(queryClient);
      }
      if (error) {
        throwProblem(error);
      }
      if (!response.ok) {
        // A refusal that carried no problem document still has to refuse.
        throwProblem({
          type: "about:blank",
          title: response.statusText || "Request failed",
          status: response.status,
        });
      }
      return data;
    },
    onSuccess: () => list.refetch(),
  });

  // AS-2 kill-switch: revoke is a hard DELETE, never a soft toggle in this
  // client — ConfirmModal guards it so a stray click can't kill a live
  // agent's credential.
  const revoke = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/passports/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // Refetch BEFORE closing, so the row focus returns to is already carrying
      // the revoked state it is meant to announce. Closing first restores focus
      // against the pre-revoke DOM and reads the row back unchanged.
      await list.refetch();
      setConfirmId(null);
    },
  });

  // The token arrives asynchronously, is shown once, and is never re-disclosed:
  // announcing it is not enough on its own, because a reader whose focus is
  // still on the Mint button has to hunt for what they just made. Focus moves
  // to the region holding it, which is also what makes the copy gesture reach
  // it from the keyboard.
  const minted = mint.isSuccess;
  useEffect(() => {
    if (minted) {
      tokenRegion.current?.focus();
    }
  }, [minted]);

  // Closing resets the whole attempt, so re-opening starts clean rather than
  // showing the previous mint's token or its refusal. The scope defaults come
  // back with it — the drawer is not a form somebody left half-filled, it is a
  // new passport each time.
  const closeMint = useCallback(() => {
    // Refused while the request is outstanding, and this is about losing a
    // credential rather than about tidiness. `mint.reset()` detaches the
    // observer; it does not cancel the POST. A drawer closed mid-flight
    // therefore still creates a passport on the server, and its token — shown
    // exactly once, never re-served — goes with the drawer. There is no way
    // back: the list carries metadata only.
    if (mint.isPending) {
      return;
    }
    setMinting(false);
    setLabel("");
    setScopes(new Set(["read", "draft"]));
    mint.reset();
  }, [mint]);

  return (
    // The card LISTS what exists; minting is a drawer. It used to be one flex
    // row holding a label, a name field, five scope ticks and the submit — eight
    // controls on one line with a single 8px gap between all of them, so nothing
    // said where the field ended and the choices began. A form that wide is not
    // a row in a settings card; it is a form, and the product already has the
    // surface for one.
    <Panel
      title={t("settings.passports")}
      // The card's one create verb, in the header band beside the title rather
      // than as a trailing row: a row whose label reads "Mint a new passport"
      // beside a button reading "New passport" says the same thing twice, and it made
      // a third interval out of what is not a decision the list holds. The verb
      // names the THING it creates and the drawer's submit names the act —
      // two buttons reading "Mint passport" are one name for two acts, for a
      // reader and for a name-based query alike.
      //
      // Ghost, which is Button's default: the primary it used to carry was the
      // only one on the page, and a page with one loud button reads as if the
      // other three cards had nothing to offer.
      titleAction={
        <Button small onClick={() => setMinting(true)}>
          {t("settings.mintOpen")}
        </Button>
      }
    >
      <PanelBody>
        {/* The card's prose, and BOTH sentences of it, above the rows: what a
            passport is, and how it differs from a connection's own credential.
            The second sentence used to be a `panel-foot` band under the list,
            which gave one card three
            different intervals — a body, a row list, and a ruled band — where
            its neighbours have two. Every card on this page now reads the same
            way: title, prose, rows. No `form-stack` either: the paragraph's own
            margin is the interval to the list, and the flex gap on top of it
            made this card's prose sit 28px off its rows against the 16px the
            connected-agents card next to it keeps. */}
        <p className="settings-panel-sub">{t("settings.passportsSub")}</p>
        <p className="settings-panel-sub">{t("settings.passportsLendHint")}</p>
        <SettingList>
          {/* Only what this human MINTED, each credential its own row: the name
              on the left, what it currently IS on the right — masked token,
              scopes, dates, and the verb that kills it. A row carrying a
              connection was issued by the token exchange to a client — it
              belongs to ConnectedAgentsCard, and listing it here put a raw DCR
              client id among the names the human chose. `connection` is the
              server's own statement of which kind a row is; the `oauth:` label
              prefix is display text and decides nothing.
              The rows are handed to the enclosing list as its own children
              rather than wrapped in a list of their own: the hairline between
              two credentials belongs to the card that holds both. */}
          <QueryGate
            pendingLabel={t("settings.passports")}
            query={list}
            empty={(page) =>
              page.data.every((passport) => passport.connection != null)
            }
          >
            {(page) =>
              page.data
                .filter((passport) => passport.connection == null)
                .map((passport) => (
                  <PassportRow
                    key={passport.id}
                    passport={passport}
                    locale={locale}
                    onRevoke={(row) => {
                      revokingRow.current = row;
                      setConfirmId(passport.id);
                    }}
                  />
                ))
            }
          </QueryGate>
        </SettingList>
      </PanelBody>
      <Modal
        open={minting}
        onClose={closeMint}
        labelledBy={mintTitleId}
        placement="right"
      >
        <h2 className="t-h2" id={mintTitleId}>
          {t("settings.mint")}
        </h2>
        {/* The token region is mounted for the whole life of the drawer rather
            than appearing with the token in it: a live region inserted at the
            same moment as its content is not reliably announced, and this token
            is shown exactly once. */}
        <div
          className="passport-token"
          ref={tokenRegion}
          tabIndex={-1}
          role="status"
        >
          {mint.isSuccess && (
            <PanelPlate>
              <p className="t-label">{t("settings.tokenOnce")}</p>
              <p className="t-mono passport-token-value">{mint.data.token}</p>
            </PanelPlate>
          )}
        </div>
        {mint.isSuccess ? (
          // The drawer does NOT close itself on success. Closing would take the
          // one and only sight of the credential with it, and a reader who was
          // still reading has no way back — the list carries metadata and the
          // server will not re-disclose a token.
          <div className="form-actions">
            <Button small variant="primary" onClick={closeMint}>
              {t("settings.mintDone")}
            </Button>
          </div>
        ) : (
          <form
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              if (scopes.size > 0 && !mint.isPending) mint.mutate();
            }}
          >
            <Field label={t("settings.passportLabel")}>
              {(control) => (
                <TextInput
                  {...control}
                  value={label}
                  onChange={(event) => setLabel(event.target.value)}
                />
              )}
            </Field>
            {/* A fieldset with a legend, which is what five checkboxes that
                belong together ARE. Loose siblings beside a text input said
                nothing about what they were choices FOR, and the accessible
                group had no name at all. `.field-multiselect` is the house
                spelling — `create.tsx` has used it for exactly this since it
                was written. */}
            <fieldset
              className="field-multiselect"
              aria-describedby={mintScopeHintId}
            >
              <legend className="t-label">
                {t("settings.passportScopes")}
              </legend>
              <p id={mintScopeHintId} className="t-caption">
                {t("settings.passportScopesHint")}
              </p>
              {PASSPORT_SCOPES.map((scope) => (
                <Checkbox
                  key={scope}
                  className="t-label"
                  checked={scopes.has(scope)}
                  onChange={(event) => {
                    const next = new Set(scopes);
                    if (event.target.checked) {
                      next.add(scope);
                    } else {
                      next.delete(scope);
                    }
                    setScopes(next);
                  }}
                  label={t(scopeLabelKey(scope))}
                />
              ))}
            </fieldset>
            {/* Beside the button that produced it, in the danger tone. It used
                to render two blocks below, past the token region, as a caption
                with no live role — so a refused mint announced nothing and sat
                where nothing had been pressed. */}
            {mint.isError && (
              <Callout tone="danger" live="alert">
                {problemMessageOf(mint.error, t)}
              </Callout>
            )}
            <div className="form-actions">
              <Button small disabled={mint.isPending} onClick={closeMint}>
                {t("settings.mintCancel")}
              </Button>
              <Button
                small
                type="submit"
                variant="primary"
                // A passport with no scope is a credential that can do nothing,
                // so the button says why it is refused rather than sitting pale
                // with nothing to offer. The sentence refuses the press on its
                // own, so there is no `disabled` beside it saying the same
                // thing in a spelling that carries no explanation.
                // And only while nobody is minting: `reason` outranks `pending`
                // in Button, so a scope cleared after the press would take the
                // spinner and `aria-busy` off a request still in flight — the
                // reader would be told the press was refused while the write
                // they made is running.
                reason={
                  scopes.size === 0 && !mint.isPending
                    ? t("settings.passportScopesRequired")
                    : undefined
                }
                pending={mint.isPending}
                busyLabel={t("settings.minting")}
              >
                {t("settings.mint")}
              </Button>
            </div>
          </form>
        )}
      </Modal>
      <ConfirmModal
        open={confirmId != null}
        onClose={() => {
          setConfirmId(null);
          revoke.reset();
        }}
        title={t("settings.revoke")}
        confirmLabel={t("settings.revoke")}
        onConfirm={() => confirmId && revoke.mutate(confirmId)}
        pending={revoke.isPending}
        error={revoke.error ? problemMessageOf(revoke.error, t) : null}
        // The revoked passport's own row, which survives the DELETE as a
        // struck-through entry carrying the "revoked" badge — so focus lands on
        // the outcome, at the place the reader was working. The Revoke button
        // they pressed cannot be the target: it is what the badge replaced.
        returnFocusTo={() => revokingRow.current}
      >
        <p>{t("settings.revokeConfirm")}</p>
      </ConfirmModal>
    </Panel>
  );
}

type PassportSummary = components["schemas"]["PassportSummary"];

// One minted passport as one row: the name the human gave it on the left, what
// the credential currently IS on the right, and the verb that kills it at the
// same x as every other answer on the page. The revoked state is STRUCK, never
// dimmed: dimming drops the row under the AA contrast floor (B-EP09.21), and it
// is the revoked row a reader most needs to read.
//
// The row is wrapped rather than bare because the revoke confirm hands focus
// back HERE — the button it was opened from is gone by then — and the wrapper is
// what carries `data-passport` for the resolver and the -1 tab index that makes
// it reachable by focus() without joining anybody's Tab order.
function PassportRow({
  passport,
  locale,
  onRevoke,
}: Readonly<{
  passport: PassportSummary;
  locale: Locale;
  onRevoke: (row: HTMLElement | null) => void;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  const revoked = passport.revoked_at != null;
  return (
    <div data-passport={passport.id} tabIndex={-1}>
      <SettingRow
        label={
          <span className={revoked ? "passport-struck" : undefined}>
            {passport.label}
          </span>
        }
        // The credential's lifetime reads on the LEFT, under the name: it
        // qualifies which passport this is rather than answering what it is set
        // to, and six facts crowded into the right column left the name with
        // three characters of width.
        description={
          <span className="settings-run">
            <span>
              {t("settings.created", {
                date: formatDate(passport.created_at, locale, recordZone),
              })}
            </span>
            {/* A credential's lifetime is a personal deadline, so it reads on
                the viewer's own calendar — the same zone-by-purpose split the
                consent screen makes. created_at above stays the fixed record
                zone. */}
            {passport.expires_at && (
              <span>
                {t("settings.expires", {
                  date: formatDate(passport.expires_at, locale, viewerZone()),
                })}
              </span>
            )}
          </span>
        }
        value={
          <span className="settings-run">
            {/* The credential exists but is withheld by design (shown once at
                mint) — masked reads as "withheld", not absent. */}
            <span className="t-label">{t("settings.token")}</span>
            <FieldGuard mode="masked" />
            <ScopeChips
              labels={passport.scopes.map((scope) => scopeChipLabel(t, scope))}
            />
          </span>
        }
        control={
          revoked ? (
            <Badge tone="danger">{t("settings.revoked")}</Badge>
          ) : (
            <Button
              small
              variant="danger"
              // The row is remembered from the CLICK rather than from
              // `confirmId`: the focus resolver runs as the dialog closes, by
              // which time that state is already back to null and there is
              // nothing left to look the row up by.
              onClick={(event) =>
                onRevoke(
                  event.currentTarget.closest<HTMLElement>("[data-passport]"),
                )
              }
            >
              {t("settings.revoke")}
            </Button>
          )
        }
      />
    </div>
  );
}

// The read-only tool console (IT-1): the same governed surface an MCP client
// sees — GET /agent-tools, with an optional passport selector that strikes
// through any row the selected passport's granted scopes don't cover. No passport
// picked means every row reads as reachable (the unfiltered inventory).
function AgentToolsCard() {
  const t = useT();
  const { locale } = useLocale();
  const [passportId, setPassportId] = useState<string>("");
  const tools = useQuery({
    queryKey: ["agent-tools"],
    queryFn: async () => {
      const { data, error } = await api.GET("/agent-tools");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const passports = useQuery({
    queryKey: ["passports"],
    queryFn: async () => {
      const { data, error } = await api.GET("/passports");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  // Live, and minted by the human themselves. A connection's credential is
  // neither: it is minted fresh by the token exchange from whatever the human
  // ticked on the consent screen, so it was never a standalone passport a
  // human picked from a list — offering one here would offer a choice that
  // doesn't exist, and would put a raw DCR client id back in front of a reader
  // the rest of this change just took it away from.
  const mintedPassports = (passports.data?.data ?? []).filter(
    (p) => p.revoked_at == null && p.connection == null,
  );
  // Whether this human has ever minted one — a REVOKED one still counts, so
  // the row a human just revoked does not yank the selector out from under
  // them (the test below pins that transition). A connection's credential
  // never counts, revoked or not: a human who only ever connected agents,
  // never minted, has no selector to show.
  const everMintedAPassport = (passports.data?.data ?? []).some(
    (p) => p.connection == null,
  );
  // The filter follows the selector: a passport revoked while it was the
  // chosen scope drops out of the options, and the <select> then shows "all
  // passports" — so the inventory must read as unfiltered too, rather than
  // stay quietly scoped to a credential no longer on offer.
  const scopeId = mintedPassports.some((p) => p.id === passportId)
    ? passportId
    : "";
  const grantedScopes = new Set(
    mintedPassports.find((p) => p.id === scopeId)?.scopes ?? [],
  );

  return (
    <Panel title={t("tools.title")}>
      {/* No `form-stack`: the description's own margin is the interval to the
          rows, and the flex gap on top of it gave this card 28px where the
          card above it has 16. */}
      <PanelBody>
        <p className="settings-panel-sub">{t("tools.sub")}</p>
        <SettingList>
          {/* The dial FIRST, then the inventory it narrows — the posture before
              the judgements that read it. Absent, not disabled, while this human
              has minted nothing: a selector offering one choice is a control
              with nothing behind it. `PassportSelect` carries its own accessible
              name ("All passports"), the way `Switch` does, so the row hands it
              no ARIA of its own. */}
          {everMintedAPassport && (
            <SettingRow
              label={t("tools.scopeLabel")}
              control={
                <PassportSelect
                  options={mintedPassports.map((p) => ({
                    id: p.id,
                    label: t("tools.scopedTo", { label: p.label }),
                    scopes: p.scopes,
                  }))}
                  value={scopeId}
                  onChange={setPassportId}
                  allowEmpty
                  emptyLabel={t("tools.scopeAll")}
                  ariaLabel={t("tools.scopeAll")}
                />
              }
            />
          )}
          {/* The inventory is a REFERENCE, and it is 68 tools long: each row
              carries the tool's name, what it is for, and the text an agent
              selects it by, which is the promise this console makes — and read
              open it measured 14,000px, so the card was a page-long wall in
              front of the two rows above it that a reader actually sets. So it
              is the card's secondary half and it is closed: the rule this page
              follows for anything advanced or diagnostic.

              One row per governed tool, handed to the list inside as its own
              children so the hairline between two tools comes from the list
              that holds both. */}
          <QueryGate
            query={tools}
            empty={(data) => data.data.length === 0}
            pendingLabel={t("tools.title")}
          >
            {(data) => (
              <Disclosure
                summary={t("tools.inventory", {
                  count: formatNumber(data.data.length, locale),
                })}
              >
                <SettingList>
                  {data.data.map((tool) => (
                    <ToolRow
                      key={tool.name}
                      tool={tool}
                      reachable={
                        !scopeId ||
                        tool.required_scope == null ||
                        grantedScopes.has(tool.required_scope)
                      }
                    />
                  ))}
                </SettingList>
              </Disclosure>
            )}
          </QueryGate>
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

type AgentTool = components["schemas"]["AgentTool"];

// One governed tool as one row, in the same two columns as every other row on
// this page: the tool's NAME is the label, what it is for and what an agent
// selects it by read under it, and the governance it runs under answers on the
// right.
//
// The name and its written title used to share the label's line, so each row
// read as a pair of unrelated strings — "account_coverage  Relationship coverage
// on a deal" — beside cards whose rows are a label with a description beneath.
// The title is a statement ABOUT the tool, so it goes where this page puts those.
//
// Struck, not dimmed: dimming the row to 0.4 took the whole row under the AA
// contrast floor (B-EP09.21) — including the words that are supposed to be the
// text equivalent of the dimming, so the one part a reader needs most became the
// hardest to read. The strikethrough wraps the NAME and its display title and
// nothing else: the badges state the tool's governance, which is true either
// way, and the unreachable line is the explanation.
//
// That line is prose and therefore NOT a control: it used to sit in the right
// column beside the badges, which is what left the answers on this card at three
// different widths. It says something about the tool, so it reads with the rest
// of what this row says about it.
//
// Wrapped rather than bare because `data-tool` is how the console's own coverage
// reads one tool's row out of the inventory.
function ToolRow({
  tool,
  reachable,
}: Readonly<{ tool: AgentTool; reachable: boolean }>) {
  const t = useT();
  const struck = reachable ? undefined : "tool-out-of-scope";
  return (
    <div data-tool={tool.name}>
      <SettingRow
        label={
          <span
            className={["t-mono", "tool-name", struck]
              .filter(Boolean)
              .join(" ")}
          >
            {tool.name}
          </span>
        }
        description={
          <span className="tool-row-text">
            {tool.title && <span className={struck}>{tool.title}</span>}
            {/* The text an agent actually selects on. This console promises the
                surface an MCP client sees, and the name alone was never that. */}
            <span>{tool.description}</span>
            {!reachable && <span>{t("tools.unreachable")}</span>}
          </span>
        }
        control={
          <span className="settings-run">
            <AutonomyDot tier={dotTier(tool.tier)} />
            {tool.required_scope && <Badge>{tool.required_scope}</Badge>}
            {tool.egress && <Badge tone="warn">{t("tools.egress")}</Badge>}
          </span>
        }
      />
    </div>
  );
}

// The danger-zone reset action: wipes the installation back to its first-boot
// state. Double-gated client-side — the admin role AND the server-driven
// `data_reset_available` flag on /me (never VITE_UI_PREVIEW_RESET, which is the
// unrelated password-reset link) — so the affordance is invisible unless the
// deployment armed the capability; the server gates the endpoint on that same
// value and 404s it otherwise, regardless of what this card renders.
//
// This is admin-ONLY, and narrower than the Maintenance entry
// that hosts it — that entry opens on the embedding_reindex read, so an ops seat
// reaches it for the search index and simply finds no reset control. The
// server's auth.RequireAdmin on /admin/reset-data admits only the literal
// "admin" role (mirrors users-admin.tsx's isAdmin check), so neither a manager
// nor an ops user may see a button that can only 403. The organization's name
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
  const isAdmin = useHoldsAdminRole();
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
      const { data, error } = await api.POST("/admin/reset-data", {
        body: { confirmation: typed },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
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

  if (!isAdmin || !me.data?.data_reset_available) {
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
        <p className="settings-panel-sub">{t("settings.dangerZoneSub")}</p>
        <SettingList>
          {/* One row, because there is one act: what it does on the left, the
              verb that does it on the right. This verb opens the question and
              the dialog's confirm answers it, so each is named for its own act
              — a destructive button and the button that asks again about it
              must not read the same while both are on screen. */}
          <SettingRow
            label={t("settings.resetDataLabel")}
            description={t("settings.resetDataDesc")}
            control={
              <Button small variant="danger" onClick={() => setOpen(true)}>
                {t("settings.resetDataButton")}
              </Button>
            }
          />
        </SettingList>
        {summary && (
          <p className="t-caption settings-danger-result" role="status">
            {t("settings.resetDataResult", {
              tables: formatNumber(summary.tables_cleared, locale),
              jobs: formatNumber(summary.jobs_deleted, locale),
              streams: formatNumber(summary.streams_purged, locale),
              keys: formatNumber(summary.cache_keys_deleted, locale),
              objects: formatNumber(summary.objects_deleted, locale),
            })}
          </p>
        )}
        {summary?.drain_timed_out && (
          <p className="t-caption settings-danger-warning" role="alert">
            {t("settings.resetDataDrainWarning")}
          </p>
        )}
      </PanelBody>
      <ConfirmModal
        open={open}
        onClose={() => {
          // Don't let Escape/backdrop dismiss the dialog mid-request: closing
          // re-enables the outer button while the first destructive POST is
          // still in flight (reset.reset() clears mutation state but cannot
          // abort the sent request), which would allow a second reset.
          if (reset.isPending) {
            return;
          }
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
          <p className="t-caption">
            {t("settings.resetDataConfirmName")}{" "}
            {/* userSelect:all lets one click select the whole name to copy */}
            <code style={{ userSelect: "all", fontWeight: 600 }}>
              {workspaceName}
            </code>
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

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

// The 3 shared scalar fields between create and edit pipeline forms.
function pipelineFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "name", label: "pipeline.name", required: true },
    {
      key: "is_default",
      label: "pipeline.default",
      type: "select",
      required: true,
      options: [
        { value: "false", label: t("pipeline.notDefault") },
        { value: "true", label: t("pipeline.default") },
      ],
    },
    { key: "position", label: "pipeline.position", type: "number" },
  ];
}

// Coerces a form value (CreateAction's values are strings; EditAction's
// update callback widens to Record<string, unknown> so a screen COULD prefill
// non-string values) down to the trimmed string this form always produces —
// mirrors deals.tsx's mapDealUpdate `str` helper, keeping both create's and
// edit's transports on the one map function without an `as` cast.
function str(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}

function mapPipelineBody(v: Record<string, unknown>) {
  return {
    name: str(v.name),
    is_default: v.is_default === "true",
    position: v.position ? Number(str(v.position)) : 0,
  };
}

// Narrows the form's free-text semantic value into the Stage enum WITHOUT a
// cast (mirrors deals.tsx's forecastCategory) — an unrecognized value falls
// back to "open" rather than shipping a bad literal to the wire.
function stageSemantic(v: unknown): Stage["semantic"] {
  switch (v) {
    case "won":
      return "won";
    case "lost":
      return "lost";
    default:
      return "open";
  }
}

// UpdateStageRequest carries no pipeline_id (a stage never moves pipelines
// via this form) while CreateStageRequest requires one — so this returns
// only the fields the two requests share, and the create transport adds
// pipeline_id on top.
function mapStageBody(v: Record<string, unknown>) {
  return {
    name: str(v.name),
    position: v.position ? Number(str(v.position)) : 0,
    semantic: stageSemantic(v.semantic),
    win_probability: v.win_probability ? Number(str(v.win_probability)) : 0,
  };
}

function stageFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "name", label: "stage.name", required: true },
    { key: "position", label: "pipeline.position", type: "number" },
    {
      key: "semantic",
      label: "stage.semantic",
      type: "select",
      required: true,
      options: [
        { value: "open", label: t("stage.semOpen") },
        { value: "won", label: t("stage.semWon") },
        { value: "lost", label: t("stage.semLost") },
      ],
    },
    { key: "win_probability", label: "stage.winProb", type: "number" },
  ];
}

// Localized badge for a stage's semantic — open/won/lost each render as a
// short label rather than the raw enum value.
function stageSemanticLabel(
  semantic: Stage["semantic"],
  t: ReturnType<typeof useT>,
): string {
  if (semantic === "won") {
    return t("stage.semWon");
  }
  if (semantic === "lost") {
    return t("stage.semLost");
  }
  return t("stage.semOpen");
}

// Tone-less Badge shares the card-inset background it sits on (both resolve
// to var(--bgCard)) — the semantic pill needs an explicit tone to be visible.
function stageSemanticTone(
  semantic: Stage["semantic"],
): "success" | "danger" | "accent" {
  switch (semantic) {
    case "won":
      return "success";
    case "lost":
      return "danger";
    default:
      return "accent"; // open
  }
}

// The bespoke per-pipeline "new stage" trigger: CreateAction's testid
// (`new-record`) can't disambiguate multiple pipelines on one screen, so
// this composes the same Button + CreateRecordModal pieces directly rather
// than adding new form infra.
function StageCreate({ pipelineId }: Readonly<{ pipelineId: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (values: Record<string, string>) => {
      const { data, error } = await api.POST("/stages", {
        body: { ...mapStageBody(values), pipeline_id: pipelineId },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
    },
  });
  return (
    <>
      <Button
        small
        data-testid={`new-stage-${pipelineId}`}
        onClick={() => setOpen(true)}
      >
        {t("stage.new")}
      </Button>
      <CreateRecordModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("stage.new")}
        fields={stageFields(t)}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        onSubmit={(values) => mutation.mutate(values)}
      />
    </>
  );
}

// The removal half of the bounded stage surface. Both refusals are the
// server's — a stage still holding deals, and the terminal won/lost pair —
// so this asks and then shows what it was told rather than pre-judging
// from the row: the refusal names the deals standing in the way, which is
// the part an admin acts on, and a board read a minute ago would name the
// wrong ones.
function StageRemove({
  stage,
  returnFocusTo,
}: Readonly<{ stage: Stage; returnFocusTo: () => HTMLElement | null }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  // Removal is pipeline:delete, not the pipeline:update everything else on
  // this card runs on — the server gates it that way, so a principal who
  // may add and rename stages but not remove one is not shown a control
  // that could only ever answer 403. Read before the early return: the
  // hooks a render performs must not depend on the answer.
  const canRemove = useCanWrite("pipeline", "delete");
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE("/stages/{id}", {
        params: { path: { id: stage.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // The refetched pipelines FIRST, then the dialog: closing it hands
      // focus back to a list that must no longer hold this row.
      await queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      setOpen(false);
    },
  });
  // A refusal is about the workspace's state, not the dialog's: reopening
  // must ask again rather than reprint what the last attempt was told.
  const close = () => {
    remove.reset();
    setOpen(false);
  };
  if (!canRemove) {
    return null;
  }
  return (
    <>
      {/* Ghost, not danger. The dialog behind it is where the danger lives —
          its confirm is the red one — and a trigger that shouts as loudly as
          the act it only ASKS about put six solid red buttons in one pipeline,
          which is the shout a reader stops reading. */}
      <Button
        small
        data-testid={`remove-stage-${stage.id}`}
        onClick={() => setOpen(true)}
      >
        {t("stage.remove")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("stage.removeTitle")}
        confirmLabel={t("stage.removeConfirm")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => remove.mutate()}
        // The stage list, not the trigger: a successful removal unmounts
        // the row this button lives in, so there is nothing to hand focus
        // back to (design-system/confirmmodal).
        returnFocusTo={returnFocusTo}
      >
        <p className="t-small">{t("stage.removeBody", { name: stage.name })}</p>
      </ConfirmModal>
    </>
  );
}

function StageRow({
  stage,
  canEdit,
  t,
  returnFocusTo,
}: Readonly<{
  stage: Stage;
  canEdit: boolean;
  t: ReturnType<typeof useT>;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const { locale } = useLocale();
  return (
    // The four tracks (name, semantic badge, probability, edit) live in
    // settings.css, where they can have a phone breakpoint. Inline, the three
    // fixed tracks plus the Edit button left about 60px for a stage name that
    // could not wrap, and it painted straight over the badge beside it.
    <li className="stage-row">
      <span className="stage-name">{stage.name}</span>
      <Badge tone={stageSemanticTone(stage.semantic)}>
        {stageSemanticLabel(stage.semantic, t)}
      </Badge>
      <span className="t-mono t-small">
        {formatNumber(stage.win_probability, locale)}%
      </span>
      {/* Each control carries its own verb — editing a stage is
          pipeline:update, removing one is pipeline:delete — so a role
          holding one without the other still sees the one it may use. */}
      <span className="stage-verbs">
        {canEdit && (
          <EditAction<Stage>
            label={t("stage.edit")}
            savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
            invalidate="pipelines"
            recordKey="stage"
            record={{
              id: stage.id,
              name: stage.name,
              position: String(stage.position),
              semantic: stage.semantic,
              win_probability: String(stage.win_probability),
            }}
            fields={stageFields(t)}
            update={async (values) => {
              const { data, error } = await api.PATCH("/stages/{id}", {
                params: { path: { id: stage.id } },
                body: mapStageBody(values),
              });
              if (error) {
                throwProblem(error);
              }
              return data;
            }}
          />
        )}
        <StageRemove stage={stage} returnFocusTo={returnFocusTo} />
      </span>
    </li>
  );
}

// One pipeline as one row: its name on the left, and under it what the pipeline
// IS — default or not — the verbs that change it, and the stage ladder itself.
//
// Stacked, because the ladder IS the subject rather than an answer to a question
// that fits beside it: three to six stages, each carrying a name, a semantic
// badge, a probability and two verbs of its own. The pipeline's own name is the
// row's LABEL, which is what puts it at the same x as every other naming on the
// page — it used to be an inner heading, drawn one step larger than the card
// title above it.
function PipelineRow({
  pipeline,
  canEdit,
  t,
}: Readonly<{
  pipeline: Pipeline;
  canEdit: boolean;
  t: ReturnType<typeof useT>;
}>) {
  const stageList = useRef<HTMLUListElement>(null);
  const stages = [...(pipeline.stages ?? [])].sort(
    (a, b) => a.position - b.position,
  );
  return (
    <SettingRow
      label={pipeline.name}
      layout="stack"
      control={
        <div className="form-stack settingrow-measure">
          {/* What this pipeline IS, and the verbs that change it, above the
              ladder they act on. */}
          <div className="pipeline-standing">
            <Badge tone={pipeline.is_default ? "success" : undefined}>
              {pipeline.is_default
                ? t("pipeline.default")
                : t("pipeline.notDefault")}
            </Badge>
            {canEdit && (
              <>
                <EditAction<Pipeline>
                  label={t("pipeline.edit")}
                  savedMessage={(saved) =>
                    t("record.saveDone", { name: saved.name })
                  }
                  invalidate="pipelines"
                  recordKey="pipeline"
                  record={{
                    id: pipeline.id,
                    name: pipeline.name,
                    is_default: String(pipeline.is_default),
                    position: String(pipeline.position),
                  }}
                  fields={pipelineFields(t)}
                  update={async (values) => {
                    const { data, error } = await api.PATCH("/pipelines/{id}", {
                      params: { path: { id: pipeline.id } },
                      body: mapPipelineBody(values),
                    });
                    if (error) {
                      throwProblem(error);
                    }
                    return data;
                  }}
                />
                <StageCreate pipelineId={pipeline.id} />
              </>
            )}
          </div>
          {/* tabIndex -1 so a removal can hand focus to the list it changed:
              the row's own Remove button is gone by then, and focus dropped to
              <body> leaves a screen-reader user at the top of the document. */}
          <ul ref={stageList} tabIndex={-1} className="stage-rows">
            {stages.map((stage) => (
              <StageRow
                key={stage.id}
                stage={stage}
                canEdit={canEdit}
                t={t}
                returnFocusTo={() => stageList.current}
              />
            ))}
          </ul>
        </div>
      }
    />
  );
}

// D-8: Settings → Pipelines config. Reads via the SAME ["pipelines","all"]
// key the deals screen's plural selector uses (an array shape, distinct
// from DealScreen's single-pipeline ["pipelines"] cache entry) — any
// mutation here invalidates the ["pipelines"] prefix, so both shapes stay
// fresh. The list itself is readable by everyone; only the write affordances are
// gated, and the server stays the RBAC authority. Three of the five seeded roles
// hold pipeline READ and no write verb at all, so for most readers this card is
// the read-only case rather than an edge of it — which is why it states that
// posture once instead of leaving a reader to infer it from absent buttons.
export function PipelinesCard() {
  const t = useT();
  // Adding a pipeline is pipeline:create. Everything else here — renaming a
  // pipeline, adding a stage, editing one, reordering — is pipeline:update,
  // including the stage CREATE affordance: a stage is not its own RBAC object,
  // so adding one is an update to the pipeline that owns it.
  const canCreate = useCanWrite("pipeline", "create");
  const canEdit = useCanWrite("pipeline", "update");
  const query = useQuery({
    queryKey: ["pipelines", "all"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  return (
    <Panel
      title={t("settings.pipelines")}
      // Adding a pipeline is four inputs committed together, so the header
      // keeps the verb and the dialog keeps the form. It sits in the header
      // band rather than as a trailing row: a row's label would repeat the
      // button beside it, and a card-level create verb is the header's job
      // everywhere else on these pages. Absent without the create grant,
      // exactly as each Edit verb is — the read-only posture is stated once
      // below.
      titleAction={
        canCreate && (
          <CreateAction
            label={t("pipeline.new")}
            invalidate="pipelines"
            screen="settings"
            create={async (values) => {
              const { data, error } = await api.POST("/pipelines", {
                body: { ...mapPipelineBody(values), stages: [] },
              });
              if (error) {
                throwProblem(error);
              }
              return data;
            }}
            fields={pipelineFields(t)}
          />
        )
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("settings.pipelinesSub")}</p>
        {/* Said once, at the top, rather than annotating each absent control —
            the rule in design-system/README.md. A reader holding one of the two
            verbs can see for themselves which controls they got. */}
        {!canCreate && !canEdit && (
          <p className="settings-panel-sub">
            {t("settings.pipelinesReadOnly")}
          </p>
        )}
        <SettingList>
          <QueryGate
            pendingLabel={t("settings.pipelines")}
            query={query}
            empty={(pipelines) => pipelines.length === 0}
          >
            {(pipelines) =>
              pipelines.map((pipeline) => (
                <PipelineRow
                  key={pipeline.id}
                  pipeline={pipeline}
                  canEdit={canEdit}
                  t={t}
                />
              ))
            }
          </QueryGate>
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

// The tier table: informational, and the advance-stage row is locked — there
// is no toggle that could soften it (AC-settings). It reads AFTER the passports
// and the tools it governs, because nothing on it can be acted on: it is the
// reference for the tiers those rows are marked with. "Locked" is about the
// FLOOR, not about waiting every time: advancing resolves its tier per move and
// only stages when the move closes the deal, and that floor is what no toggle
// reaches.
function AutonomyCard() {
  const t = useT();
  return (
    <Panel title={t("settings.autonomy")}>
      {/* No `form-stack`: the description's own margin is the interval to the
          rows. See PassportCard — the flex gap on top of that margin is what
          gave the cards on this page two different intervals. */}
      <PanelBody>
        <p className="settings-panel-sub">{t("settings.autonomySub")}</p>
        {/* Four rows in the page's own language, even though none of them is
            settable: what the tier COVERS reads left as prose, and the tier it
            runs at — the dot, and on the locked row the badge saying the answer
            cannot move — sits at the same x as every answer on this page. A
            reader coming from the tool inventory above is matching dots, which
            is why sending carries the green one: a person's grant of the `send`
            scope IS the approval, so a funded send does not stage a second. */}
        <SettingList>
          <SettingRow
            label={t("settings.tierRead")}
            control={<AutonomyDot tier="auto" />}
          />
          <SettingRow
            label={t("settings.tierSend")}
            control={<AutonomyDot tier="auto" />}
          />
          <SettingRow
            label={t("settings.tierWait")}
            control={<AutonomyDot tier="confirm" />}
          />
          <SettingRow
            label={t("settings.tierAdvance")}
            // The dot and the badge are ONE answer — the tier, and the fact that
            // it cannot move — so they travel as a run at the page's own 8px chip
            // gap. Handed to the control column loose, they took its 12px flex
            // gap instead, which put the one row on this page carrying two chips
            // at a different interval from every tool row above it.
            control={
              <span className="settings-run">
                <AutonomyDot tier="confirm" />
                <Badge tone="warn">{t("settings.locked")}</Badge>
              </span>
            }
          />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

type AuditLogEntry = components["schemas"]["AuditLogEntry"];

// The union of before/after keys for one row's diff — a key present on
// neither side is never shown, so the panel only ever displays fields the
// mutation actually touched.
function diffKeys(
  before: AuditLogEntry["before"],
  after: AuditLogEntry["after"],
): string[] {
  const keys = new Set<string>();
  for (const key of Object.keys(before ?? {})) {
    keys.add(key);
  }
  for (const key of Object.keys(after ?? {})) {
    keys.add(key);
  }
  // `stable`, because these are the record's OWN column names, rendered
  // untranslated a few lines below: the reader's UI language has no claim on
  // their order, and one audit row must not read in two orders on two machines
  // showing the same page.
  return [...keys].sort(stable);
}

// A key absent from an object (withheld/never set) reads the same as an
// explicit null through FieldDiff's honest empty marker (created/cleared) —
// this never fabricates a value for a key the side genuinely lacks.
function diffValue(
  rec: AuditLogEntry["before"] | AuditLogEntry["after"],
  key: string,
): string | null {
  const value = rec?.[key];
  if (value === null || value === undefined) {
    return null;
  }
  // Object/array field values (custom-field JSON, links, ...) need JSON
  // rendering — the bare String() coercion collapses them to "[object
  // Object]", which is neither readable nor honest about what changed.
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

// yyyy-mm-dd from a date input, read as a UTC instant: start-of-day for
// `from`, end-of-day for `to`, so the range is inclusive of the whole `to`
// day rather than silently truncating it at midnight.
function fromDateParam(date: string): string {
  return new Date(`${date}T00:00:00.000Z`).toISOString();
}
function toDateParam(date: string): string {
  return new Date(`${date}T23:59:59.999Z`).toISOString();
}

type AuditLogFilters = Readonly<{
  actor: string;
  entityType: string;
  entityId: string;
  action: string;
  from: string;
  to: string;
}>;

// The unfiltered question the view opens on. One object rather than six
// useState strings, so the filter row can be its own component that hands back
// a whole answer instead of taking six setters — and so the query key is that
// same answer, which cannot drift from what the request carries.
const UNFILTERED_AUDIT_LOG: AuditLogFilters = {
  actor: "",
  entityType: "",
  entityId: "",
  action: "",
  from: "",
  to: "",
};

// The six filters, declared once, so the accessible wiring is identical across
// the row. Each is a `Field` — a real <label> with `htmlFor`, so clicking the
// words focuses the control. It used to be a `t-label` span pointed at by
// aria-labelledby, on the reasoning that a real label would wrap every field
// onto its own line; that is not what happens. `.field` IS a flex column, and
// the grid around it decides the layout either way, so the span bought nothing
// and cost the click target.
const AUDIT_LOG_FILTER_FIELDS: readonly Readonly<{
  key: keyof AuditLogFilters;
  labelKey: MessageKey;
  // A calendar picker rather than free text — the two ends of the range.
  date?: boolean;
}>[] = [
  { key: "actor", labelKey: "settings.auditActor" },
  { key: "entityType", labelKey: "settings.auditEntity" },
  { key: "entityId", labelKey: "settings.auditEntityId" },
  { key: "action", labelKey: "settings.auditAction" },
  { key: "from", labelKey: "settings.auditFrom", date: true },
  { key: "to", labelKey: "settings.auditTo", date: true },
];

// Every filter is optional-if-blank, so this stays a flat spread rather than
// a chain of conditionals in the queryFn itself (kept the query builder under
// the cognitive-complexity gate).
function auditLogQueryParams(
  filters: AuditLogFilters,
  pageParam: string | null,
) {
  const { actor, entityType, entityId, action, from, to } = filters;
  return {
    limit: 20,
    ...(pageParam ? { cursor: pageParam } : {}),
    ...(actor.trim() ? { actor: actor.trim() } : {}),
    ...(entityType.trim() ? { entity_type: entityType.trim() } : {}),
    ...(entityId.trim() ? { entity_id: entityId.trim() } : {}),
    ...(action.trim() ? { action: action.trim() } : {}),
    ...(from ? { from: fromDateParam(from) } : {}),
    ...(to ? { to: toDateParam(to) } : {}),
  };
}

/**
 * The filters as a QUESTION, settled — which is a different thing from the
 * filters as they are being typed.
 *
 * The row updates on every keystroke, as it must; what waits is the query. The
 * filter object is the query key, so before this every character was its own
 * `GET /audit-log`: typing `agent:runner` asked the server twelve questions,
 * eleven of them about prefixes nobody wanted an answer to, and the answers
 * could land out of order. The shared list surface settles its own search on
 * this same constant, so a filter costs the same anywhere in the product.
 */
function useSettledAuditLogFilters(typed: AuditLogFilters): AuditLogFilters {
  const [settled, setSettled] = useState(typed);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(typed), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [typed]);
  return settled;
}

function AuditLogFilterFields({
  filters,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  return (
    // Six dials, each one a row of the page's own language: what it narrows on
    // the left, the box that narrows it on the right, at the x every other
    // answer on this page sits at. They were a grid of labelled cells before —
    // legible on its own, and a second layout for the same question in the one
    // card that has both.
    //
    // Each one applies on its own (debounced), so this is a list of rows and NOT
    // a form: there is nothing to submit, which is exactly why the six do not
    // belong in a dialog.
    <SettingList>
      {AUDIT_LOG_FILTER_FIELDS.map((field) => (
        <SettingRow
          key={field.key}
          label={t(field.labelKey)}
          control={(control) => (
            <TextInput
              {...control}
              className="settingrow-measure"
              type={field.date ? "date" : undefined}
              value={filters[field.key]}
              onChange={(event) =>
                onChange({ ...filters, [field.key]: event.target.value })
              }
            />
          )}
        />
      ))}
    </SettingList>
  );
}

function AuditLogRow({
  entry,
  meUserId,
}: Readonly<{ entry: AuditLogEntry; meUserId?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [expanded, setExpanded] = useState(false);
  const keys = diffKeys(entry.before, entry.after);
  const evidence = toEvidence(entry.evidence);

  return (
    // A log entry is not a settings decision, so it is not a SettingRow: it is
    // one line of a record, and the rows of the log rule between themselves
    // inside the one stacked row that holds the whole trail.
    <div className="audit-row">
      <div className="audit-row-head">
        {/* The organization's clock, the same one the record change history
            reads on: an audit entry is a fact in the shared book, and on the
            viewer's clock an entry at 18:00Z is 21 August to a reader in Berlin
            and 22 August to one in Ho Chi Minh City — two operators quoting the
            same line quote different days. */}
        <span className="t-small">
          {formatDateTime(entry.occurred_at, locale, recordZone)}
        </span>
        <ActorTag entry={entry} meUserId={meUserId} />
        <Badge tone="accent">{entry.action}</Badge>
        {entry.entity_id && isEntityKind(entry.entity_type) ? (
          <EntityRef kind={entry.entity_type} id={entry.entity_id} />
        ) : (
          <span className="t-mono t-small">
            {entry.entity_type}
            {entry.entity_id ? ` ${entry.entity_id}` : ""}
          </span>
        )}
        <Button
          small
          aria-expanded={expanded}
          aria-label={t("settings.auditExpand")}
          onClick={() => setExpanded((value) => !value)}
        >
          <ChevronDown aria-hidden size={14} className="expander-chevron" />
        </Button>
      </div>
      {expanded && (
        <div className="audit-row-diff">
          {keys.map((key) => (
            <div key={key} className="audit-diff-line">
              <span className="t-label">{key}</span>
              <FieldDiff
                oldValue={diffValue(entry.before, key)}
                newValue={diffValue(entry.after, key)}
              />
            </div>
          ))}
          {entry.passport_id && <PassportChip id={entry.passport_id} />}
          {entry.on_behalf_of && (
            <span className="t-small">
              {t("settings.auditOnBehalf")}{" "}
              <span className="t-mono">{entry.on_behalf_of}</span>
            </span>
          )}
          {entry.authorization_rule && (
            <span className="t-small">
              {t("settings.auditRule")}: {entry.authorization_rule}
            </span>
          )}
          {evidence && <EvidenceChip evidence={evidence} />}
        </div>
      )}
    </div>
  );
}

// The result half of the audit view: the answer to whatever the filter row
// currently asks. Keyset "load more" via the page cursor, and a filter change
// is a new question — the filters ARE the query key, so changing one restarts
// the cursor chain instead of appending to a stale one.

function AuditLogEntries({
  filters,
  meUserId,
}: Readonly<{ filters: AuditLogFilters; meUserId?: string }>): ReactNode {
  const t = useT();
  // The full trail is the admin's alone (AAD-ROLE-4/A91, enforced by
  // privacy.ListAuditLog), while the page it sits on opens for ops too — the
  // consent registry above is theirs. So the read is gated here rather than
  // merely rendered, and the fetch is disabled for anyone else: an ops seat
  // reaching this page must not issue a call that can only 403, and must not be
  // handed a red failure with a Retry that cannot succeed.
  const isAdmin = useHoldsAdminRole();
  const query = useInfiniteQuery({
    queryKey: ["audit-log", filters],
    enabled: isAdmin,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/audit-log", {
        params: { query: auditLogQueryParams(filters, pageParam) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });

  const entries = query.data?.pages.flatMap((page) => page.data) ?? [];

  // Honest state matrix (§3a): withheld, loading, error, empty, then the rows —
  // kept as sequential branches rather than a nested ternary in the JSX below.
  // The whole trail is the control of ONE stacked row now, so no branch wraps
  // itself in a `PanelBody`: the row it sits in already owns the inset.
  if (!isAdmin) {
    // Withheld rather than absent, and the card keeps its place: an absent trail
    // on a page that opens for ops would read as "nothing has happened here",
    // which is a different claim from "this is not yours to read". The same
    // choice the subject-request queue above it makes, for the same reason.
    return <EmptyState>{t("settings.auditAdminOnly")}</EmptyState>;
  }
  if (query.isPending) {
    return (
      <div className="audit-loading">
        <Skeleton width="60%" />
        <Skeleton width="90%" />
      </div>
    );
  }
  if (query.isError) {
    return (
      <EmptyState>
        <p>{t("common.error")}</p>
        <p className="t-mono audit-error-cause">
          {problemMessageOf(query.error, t)}
        </p>
        <Button small onClick={() => query.refetch()}>
          {t("common.retry")}
        </Button>
      </EmptyState>
    );
  }
  if (entries.length === 0) {
    return <EmptyState>{t("common.empty")}</EmptyState>;
  }
  return (
    <div className="audit-entries settingrow-measure">
      {entries.map((entry) => (
        <AuditLogRow key={entry.id} entry={entry} meUserId={meUserId} />
      ))}
      <LoadMoreButton query={query} />
    </div>
  );
}

// AC-settings-16: the attributable audit view — live filters over actor /
// entity_type / entity_id / action / from / to, keyset "load more" via the
// page cursor. ONE card, because every other filtered list in this product puts
// its dials inside the list's own surface: a card of its own titled "Filters"
// made six inputs a subject in the page outline, level with the trail they
// narrow, and left a reader scanning two cards to answer one question. Inside
// that one card the dials are the secondary half — a reader arrives to read what
// happened — so they sit in a disclosure rather than above the trail.
// Each entry expands into the before/after diff plus the agent attribution trail
// (passport, on-behalf-of human, authorization rule, grounding evidence) —
// collapsed by default so the flat scan stays fast.
export function AuditLogCard() {
  const t = useT();
  // The current user's id is what lets ActorTag read "You" rather than naming
  // the viewer back to themselves. It is the BARE user id; the wire spells a
  // human actor "human:<uuid>", and ActorTag owns that difference.
  const meUserId = useMe().data?.user?.id;
  const isAdmin = useHoldsAdminRole();
  const [filters, setFilters] = useState<AuditLogFilters>(UNFILTERED_AUDIT_LOG);
  // The row reads what is being typed; the entries read what has settled.
  const asked = useSettledAuditLogFilters(filters);
  return (
    <Panel title={t("settings.auditEntries")}>
      {/* No `form-stack`: the description's own margin is the interval to the
          rows, and the flex gap on top of it is the second spelling that made
          the settings cards disagree about that interval. */}
      <PanelBody>
        <p className="settings-panel-sub">{t("settings.auditSub")}</p>
        <SettingList>
          {/* The dials are the card's SECONDARY half — a reader arrives to read
              what happened, and narrows it second — so they sit in a
              disclosure, closed, rather than costing every visit six input
              boxes above the trail. Not a dialog: each filter applies on its
              own, and a dialog with nothing to submit would leave a narrowed
              list behind a closed door.
              Absent, not withheld, for a reader who may not read the trail: six
              inputs that narrow a list they cannot see are a control with
              nothing behind it. The TRAIL below stays and says why — absence
              there would claim nothing had happened. */}
          {isAdmin && (
            <Disclosure summary={t("settings.auditFilters")}>
              <AuditLogFilterFields filters={filters} onChange={setFilters} />
            </Disclosure>
          )}
          {/* The trail is the subject of this card rather than an answer beside
              a question, so it takes the row's whole width. */}
          <SettingRow
            label={t("settings.auditTrailLabel")}
            layout="stack"
            control={<AuditLogEntries filters={asked} meUserId={meUserId} />}
          />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}
