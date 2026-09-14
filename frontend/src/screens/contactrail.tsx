import { ChevronRight } from "lucide-react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { navigate } from "../app/router";
import { Avatar, Button } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { Panel, PanelBody } from "../design-system/panel";
import {
  omitted,
  type SectionState,
  SurfaceState,
} from "../design-system/surfacestate";
import { formatNumber, relativeDays } from "../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  translatePlural,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { useSorMode } from "./common";
import { ContactBillingRoles } from "./contactbillingroles";
import { ConsentAndChannels } from "./contactconsentpanel";
import { ContactDetails } from "./contactdetails";
import { Employers } from "./contactemployers";
import { daysSinceInbound, isQuiet } from "./contactquiet";
import { contactTabRoute } from "./contacttab";
import { CounterpartyHoldRow } from "./counterparty-hold";
import { TagsPanel } from "./tagspanel";

// The right rail (concept §5.11): SEPARATE panels, each answering one question
// about the contact — their fields, their companies, how the relationship
// stands, who knows them, what stands out, what they allow — in the order a
// reader works down the column. The same anatomy the company record's rail
// draws (companyrail.tsx), and for the same reason: a panel's own edge is
// what tells a reader they have moved on to a different question, where a
// hairline inside one long card read as one story that never ended.
//
// Every section here is still a GLANCE. The rail never becomes a second
// body — a reader who has to read the margin has lost the column it sits
// beside.

export type Contact360 = components["schemas"]["Contact360"];
type Contact = components["schemas"]["Contact"];
type ContactConsentGuard = components["schemas"]["ContactConsentGuard"];

// --- what this reader was allowed to read ----------------------------------

// A negative verdict must first prove it was allowed to look.
//
// The 360 answers a section the reader has no grant for by leaving it out and
// naming it in `sections_omitted`, so an absent field carries two opposite
// meanings — nothing was captured, or nothing was shown — and only the list
// tells them apart. This rail is the worst place in the product to get that
// wrong: its words are short verdicts ("One-sided", "Never", "Thin") that read
// as measured facts rather than as summaries, so a reader without an activity
// grant is told the contact has never written to them.
//
// So the list is read ONCE, into the sections this rail derives its words
// from, rather than as a check each verdict is trusted to remember — seven
// separate checks is the shape that left every one of them unwritten.
type Withheld = Readonly<{
  lastTouch: boolean;
  activities: boolean;
  commercial: boolean;
  nextMeeting: boolean;
  network: boolean;
  employments: boolean;
}>;

export function withheldSections(view: Contact360): Withheld {
  return {
    lastTouch: omitted(view, "last_touch"),
    activities: omitted(view, "activities"),
    commercial: omitted(view, "commercial"),
    nextMeeting: omitted(view, "next_meeting"),
    network: omitted(view, "network"),
    employments: omitted(view, "employments"),
  };
}

// A withheld reading says so rather than rendering the word a derivation would
// have produced from the absence, and it carries no tone, because there is no
// verdict to colour. The strip on this same page keeps the identical rule for
// its own six readings (contactstrip.tsx), down to the word, so the two halves
// of the record cannot disagree about what a missing section means.
function reading(
  value: string,
  withheld: boolean,
  t: ReturnType<typeof useT>,
): string {
  return withheld ? t("record.notShown") : value;
}

// The body state of a section whose sentence is a claim about the record.
// `empty` is the only state allowed to say there is none of something, so a
// withheld section keeps its place in the rail and says it is withheld instead
// of drawing as an account with nothing on it.
export function bodyState(withheld: boolean, count: number): SectionState {
  if (withheld) {
    return "withheld";
  }
  return count === 0 ? "empty" : "ready";
}

export function ContactRail({
  view,
  guard,
  firstName,
  onExplain,
  onOpenEmail,
}: Readonly<{
  view: Contact360;
  guard: ContactConsentGuard | undefined;
  firstName: string;
  onExplain: () => void;
  /** Opens one message in the record's drawer. The page owns the drawer, so
   * the rail is handed the opener rather than mounting a second one. */
  onOpenEmail?: (activityId: string) => void;
}>) {
  // A plain div: RecordView's own <aside> is the landmark around this, and a
  // second labelled region inside it would give a reader two names for one
  // column.
  return (
    <div className="pe-rail" data-testid="contact-rail">
      <ContactDetails contact={view.contact} />
      <Employers view={view} />
      {/* Beside their employers, not inside them: where somebody works and
          whose invoices they handle are different facts, and an external
          bookkeeper holds the second without the first. */}
      <ContactBillingRoles companies={view.billing_roles} />
      <RelationshipPulse view={view} onExplain={onExplain} />
      <WhoKnows view={view} firstName={firstName} />
      <SignalsAndRisks view={view} />
      <ConsentAndChannels view={view} guard={guard} />
      <ContactHoldSection view={view} />
      <ContactTagsSection view={view} />
      <RecentActivity view={view} onOpenEmail={onOpenEmail} />
    </div>
  );
}

// --- Keeping this correspondence private -------------------------------

// The hold control, in the rail's own section shape. Drawn for every contact
// with an address, held or not: a control that appeared only once a hold
// existed would leave a reader with no way to place the first one.
function ContactHoldSection({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const email = view.contact?.emails?.[0]?.email;
  if (!email) {
    return null;
  }
  return (
    <Panel title={t("hold.sectionTitle")}>
      <PanelBody>
        <CounterpartyHoldRow email={email} />
      </PanelBody>
    </Panel>
  );
}

// --- How this contact is filed -----------------------------------------

/**
 * The contact's tags, drawn by the SHARED panel.
 *
 * The three questions the server asks before it writes are asked here too: the
 * object grant, this record's own editability, and — inside the panel — whether
 * the vocabulary is visible at all.
 *
 * Exported for its own test: mounting the whole rail to ask whether the tag
 * verb appears would fail for eight unrelated reasons, and a test that instead
 * passed `canEdit` straight to the panel would prove only that the panel obeys
 * its prop — never that this mount computes it.
 */
export function ContactTagsSection({ view }: Readonly<{ view: Contact360 }>) {
  const contact = view.contact;
  const readOnlyReason = useContactReadOnlyReason(contact);
  // useCanWriteRecord, not useCan: applying a tag writes to the record, so the
  // verb owes the seat ceiling and this row's own `writable` as well as the
  // object grant. A rep holding `contact.update` still may not tag a colleague's
  // contact, and the read-only reason above does not answer that half.
  const canUpdate = useCanWriteRecord("contact", contact);
  if (!contact.id) {
    return null;
  }
  return (
    <TagsPanel
      entityType="contact"
      entityID={contact.id}
      canEdit={canUpdate && !readOnlyReason}
    />
  );
}

// --- Details -----------------------------------------------------------

export function useContactReadOnlyReason(contact: Contact): string | undefined {
  const t = useT();
  const overlay = useSorMode() === "overlay";
  if (contact.archived_at) {
    return t("contact.rail.archivedReadOnly");
  }
  if (overlay) {
    return t("overlay.partialWriteBack");
  }
  return undefined;
}

// --- Relationship pulse ----------------------------------------------------

// Words and directional facts. The composite score is NOT on the face
// (ADR-0096 D1); Explain reveals it with its factors and arithmetic.
function RelationshipPulse({
  view,
  onExplain,
}: Readonly<{ view: Contact360; onExplain: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const hidden = withheldSections(view);
  const inbound = view.last_inbound_at;
  const outbound = view.last_outbound_at;
  const twoWay = Boolean(inbound && outbound);
  const colleagues = view.network?.colleagues?.length ?? 0;
  return (
    <Panel
      title={t("contact.rail.pulseTitle")}
      titleAction={
        <Button small variant="ghost" onClick={onExplain}>
          {t("contact.rail.explain")}
        </Button>
      }
    >
      <PanelBody>
        {/* Four of these five readings are derived from the two directional
          timestamps, which arrive together or not at all — one grant governs
          both — so one question answers all four. */}
        <Row
          label={t("contact.rail.direction")}
          value={reading(
            twoWay ? t("contact.rail.twoWay") : t("contact.rail.oneSided"),
            hidden.lastTouch,
            t,
          )}
        />
        <Row
          label={t("contact.rail.lastReply")}
          value={reading(sinceWords(inbound, t, locale), hidden.lastTouch, t)}
        />
        <Row
          label={t("contact.rail.coverage")}
          value={reading(colleagueWords(colleagues, locale), hidden.network, t)}
        />
        <Row
          label={t("contact.rail.trend")}
          value={reading(trendWord(view, t), hidden.lastTouch, t)}
        />
        <div className="pe-pulse-overall">
          {/* The overall reading is the only one drawn in the verdict colour, and
            a withheld reading is not a verdict: colouring "Not shown" would
            state a healthy relationship in the one place a reader glances. */}
          <Row
            label={t("contact.rail.overall")}
            value={reading(overallWord(view, t), hidden.lastTouch, t)}
            strong={!hidden.lastTouch}
          />
        </div>
      </PanelBody>
    </Panel>
  );
}

function colleagueWords(count: number, locale: Locale): string {
  return translatePlural(locale, "contact.rail.colleagues", count, {
    count: formatNumber(count, locale),
  });
}

function trendWord(view: Contact360, t: ReturnType<typeof useT>): string {
  const inbound = view.last_inbound_at;
  const outbound = view.last_outbound_at;
  if (!inbound) {
    return t("contact.rail.noInbound");
  }
  if (outbound && new Date(outbound) > new Date(inbound)) {
    return t("contact.rail.cooling");
  }
  return t("contact.rail.warming");
}

function overallWord(view: Contact360, t: ReturnType<typeof useT>): string {
  const days = daysSinceInbound(view);
  if (days == null) {
    return t("contact.rail.thin");
  }
  if (isQuiet(days)) {
    return t("contact.rail.atRisk");
  }
  return t("contact.rail.strong");
}

// --- Who knows them --------------------------------------------------------

function WhoKnows({
  view,
  firstName,
}: Readonly<{ view: Contact360; firstName: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const colleagues = view.network?.colleagues ?? [];
  return (
    <Panel title={t("contact.rail.whoKnows", { name: firstName })}>
      <PanelBody>
        <SurfaceState
          loadingLabel={t("contact.rail.whoKnows", { name: firstName })}
          state={bodyState(withheldSections(view).network, colleagues.length)}
          emptyLabel={t("contact.rail.nobodyYet")}
        >
          {colleagues.slice(0, 3).map((colleague) => (
            <div className="pe-colleague" key={colleague.user_id}>
              <Avatar name={colleague.display_name} />
              <span>
                <span className="pe-colleague-name">
                  {colleague.display_name}
                </span>
                <span className="pe-colleague-proof t-caption">
                  {/* The PROOF, never a ranking nobody can check: six unanswered
                    sends must not read as stronger than two real exchanges. */}
                  {t("contact.rail.exchanges", {
                    count: formatNumber(colleague.interactions_90d, locale),
                  })}
                </span>
              </span>
            </div>
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

// --- Signals and risks -----------------------------------------------------

function SignalsAndRisks({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { signals, skipped } = derivedSignals(
    view,
    withheldSections(view),
    t,
    plural,
    locale,
  );
  return (
    <Panel title={t("contact.rail.signals")}>
      <PanelBody>
        <SurfaceState
          loadingLabel={t("contact.rail.signals")}
          state={signalsState(signals.length, skipped)}
          emptyLabel={t("contact.rail.noSignals")}
        >
          {signals.map((signal) => (
            <div className="pe-signal" key={signal.text}>
              <span className={`pe-dot pe-dot-${signal.tone}`} />
              <span>{signal.text}</span>
            </div>
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

// "Nothing stands out on this relationship" is the strongest sentence in the
// rail — it is the one a reader stops reading after — so it may only be said
// when every rule below actually ran. A derivation that had to skip a rule for
// want of a grant says the list is short instead, and one that could run none
// of them says it is withheld: a reader who is shown one signal out of three
// otherwise takes it for the whole finding.
function signalsState(shown: number, skipped: boolean): SectionState {
  if (!skipped) {
    return shown === 0 ? "empty" : "ready";
  }
  return shown === 0 ? "withheld" : "partial";
}

type Signal = Readonly<{ text: string; tone: "good" | "warn" | "bad" }>;

// Deterministic, from what the page already read. Each one is a fact the
// reader can check against the cards beside it rather than an assessment.
//
// A rule whose section this reader may not see is SKIPPED and reported as
// skipped, never resolved against the absence: "no next meeting booked" derived
// from a withheld calendar is an assertion about the deal manufactured out of a
// permission boundary, and it is indistinguishable from the real finding.
function derivedSignals(
  view: Contact360,
  hidden: Withheld,
  t: ReturnType<typeof useT>,
  plural: PluralTranslator,
  locale: Locale,
): Readonly<{ signals: ReadonlyArray<Signal>; skipped: boolean }> {
  const out: Signal[] = [];
  let skipped = hidden.lastTouch;
  const quiet = hidden.lastTouch ? null : daysSinceInbound(view);
  if (quiet != null && isQuiet(quiet)) {
    out.push({
      text: t("contact.rail.noReplyDays", {
        count: formatNumber(quiet, locale),
      }),
      tone: "bad",
    });
  } else if (quiet != null) {
    out.push({
      text: plural("contact.rail.repliedDaysAgo", quiet, {
        count: formatNumber(quiet, locale),
      }),
      tone: "good",
    });
  }
  if (hidden.commercial) {
    return { signals: out, skipped: true };
  }
  const deal = view.commercial?.deal;
  const committee = view.commercial?.committee?.length ?? 0;
  if (deal && committee === 0) {
    out.push({ text: t("contact.rail.singleThreaded"), tone: "warn" });
  }
  // The meeting rule needs BOTH sections — a visible deal for the finding to be
  // about, and a readable calendar to prove nothing is booked on it — so a
  // withheld calendar only costs a signal where there is a deal to book against.
  if (deal && hidden.nextMeeting) {
    skipped = true;
  } else if (deal && !view.next_meeting) {
    out.push({ text: t("contact.rail.noMeetingBooked"), tone: "warn" });
  }
  return { signals: out, skipped };
}

// --- Consent and channels --------------------------------------------------

// --- Recent activity -------------------------------------------------------

// Three condensed items. It never duplicates the raw timeline visible beside
// it — this is the glance, the Activity tab is the ledger.
function RecentActivity({
  view,
  onOpenEmail,
}: Readonly<{ view: Contact360; onOpenEmail?: (activityId: string) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  // The section's own emptiness is decided BEFORE the rows are defaulted: an
  // absent list and an empty one collapse into the same `[]` here, and that
  // collapse is what turns "you may not read the timeline" into "nothing has
  // ever happened with this contact".
  const withheld = withheldSections(view).activities;
  const rows = (view.activities?.data ?? []).slice(0, 3);
  return (
    <Panel
      title={t("contact.rail.recentActivity")}
      footer={
        // The rail's own glance leaves the tab's ledger one click away.
        <Button
          small
          variant="ghost"
          onClick={() => navigate(contactTabRoute(view.contact.id, "timeline"))}
        >
          {t("contact.rail.viewAllActivity")}{" "}
          <ChevronRight size={13} aria-hidden="true" />
        </Button>
      }
    >
      <PanelBody>
        <SurfaceState
          loadingLabel={t("contact.rail.recentActivity")}
          state={bodyState(withheld, rows.length)}
          emptyLabel={t("contact.rail.nothingCaptured")}
        >
          {rows.map((row) => (
            <div className="pe-rail-row" key={row.id}>
              {/* An email is CITED here rather than drawn: the rail is a
                  glance at what happened lately, and a full row with its
                  preview and access badge would make the aside compete with
                  the timeline beside it. The citation opens the same drawer
                  the timeline's row does, so both lead to one place. */}
              {row.kind === "email" ? (
                <EmailReference
                  subject={row.subject}
                  withheld={row.content_state === "withheld"}
                  onOpen={onOpenEmail ? () => onOpenEmail(row.id) : undefined}
                />
              ) : (
                <span className="pe-rail-label">{row.subject ?? row.kind}</span>
              )}
              <span className="pe-rail-value pe-rail-value-muted">
                {sinceWords(row.occurred_at, t, locale)}
              </span>
            </div>
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

// --- shared ----------------------------------------------------------------

function Row({
  label,
  value,
  strong,
}: Readonly<{ label: string; value: string; strong?: boolean }>) {
  return (
    <div className="pe-rail-row">
      <span className="pe-rail-label">{label}</span>
      <span
        className={
          strong ? "pe-rail-value pe-rail-value-good" : "pe-rail-value"
        }
      >
        {value}
      </span>
    </div>
  );
}

// sinceWords is the shared spelling, kept as a local name because two dozen
// call sites in this file read better with it.
function sinceWords(
  at: string | null | undefined,
  t: Translator,
  locale: Locale,
): string {
  return relativeDays(at, t, locale);
}
