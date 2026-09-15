import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";

import { Panel, PanelBody } from "../design-system/panel";
import { omitted, type SectionState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { type Locale, translatePlural, useT } from "../i18n";
import { useSorMode } from "./common";
import { ContactBillingRoles } from "./contactbillingroles";
import { ContactConfirmCallout } from "./contactconfirm";
import { ConsentAndChannels } from "./contactconsentpanel";
import { ContactDetails } from "./contactdetails";
import { Employers } from "./contactemployers";
import { daysSinceInbound, isQuiet, QUIET_AFTER_DAYS } from "./contactquiet";
import { CounterpartyHoldRow } from "./counterparty-hold";
import { TagsPanel } from "./tagspanel";

// The right rail (concept §5.11): SEPARATE panels, each answering one question
// about the contact (their fields, their companies, what they allow) in the
// order a reader works down the column. The same anatomy the company record's
// rail draws (companyrail.tsx), and for the same reason: a panel's own edge is
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
  guardLoading = false,
  guardFailed = false,
  onRetryGuard,
}: Readonly<{
  view: Contact360;
  guard: ContactConsentGuard | undefined;
  guardLoading?: boolean;
  guardFailed?: boolean;
  onRetryGuard?: () => void;
}>) {
  // A plain div: RecordView's own <aside> is the landmark around this, and a
  // second labelled region inside it would give a reader two names for one
  // column.
  return (
    <div className="pe-rail" data-testid="contact-rail">
      <ContactDetails contact={view.contact} />
      {/* Under the fields it is about: what the enrichment pass read into
          them and a reader has not yet confirmed, with the way to Data &
          tools where each value can be judged. */}
      <ContactConfirmCallout view={view} />
      <Employers view={view} />
      {/* Beside their employers, not inside them: where somebody works and
          whose invoices they handle are different facts, and an external
          bookkeeper holds the second without the first. */}
      <ContactBillingRoles companies={view.billing_roles} />
      {/* The relationship's standing is one badge under the name (the page's
          head reads `contactStanding`), not a card of readings: the brief in
          the work column carries direction, coverage and what is owed, and
          the same four facts in a rail card beside it were the brief twice. */}
      {/* Last, and still here: the head's Write verb is refused on what this
          panel states, and a refusal whose reason sits a tab away is a button
          that says no for no reason a reader can see. The hold and the tags
          are filing, and live on Data & tools. */}
      <ConsentAndChannels
        view={view}
        guard={guard}
        loading={guardLoading}
        failed={guardFailed}
        onRetry={onRetryGuard}
      />
    </div>
  );
}

// --- Keeping this correspondence private -------------------------------

// The hold control, in the rail's own section shape, drawn on Data & tools.
// Drawn for every contact with an address, held or not: a control that
// appeared only once a hold existed would leave a reader with no way to place
// the first one.
export function ContactHoldSection({ view }: Readonly<{ view: Contact360 }>) {
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

// --- Relationship standing --------------------------------------------

export function trendWord(
  view: Contact360,
  t: ReturnType<typeof useT>,
): string {
  switch (trendKind(view)) {
    case "none":
      return t("contact.rail.noInbound");
    case "cooling":
      return t("contact.rail.cooling");
    case "warming":
      return t("contact.rail.warming");
  }
}

// Which way the relationship is heading, as a key the word and the glyph are
// both read from: nothing without an inbound message, cooling when ours is
// the newer one, warming otherwise.
export function trendKind(view: Contact360): "warming" | "cooling" | "none" {
  const inbound = view.last_inbound_at;
  const outbound = view.last_outbound_at;
  if (!inbound) {
    return "none";
  }
  if (outbound && new Date(outbound) > new Date(inbound)) {
    return "cooling";
  }
  return "warming";
}

// The overall reading: its word, and the tone the word is drawn in. The tone
// follows the verdict key, never the rendered word, so a translation cannot
// recolour it; a thin relationship is no verdict either way and stays neutral.
export function overallVerdict(view: Contact360): {
  word: (t: ReturnType<typeof useT>) => string;
  tone: "success" | "danger" | undefined;
} {
  const days = daysSinceInbound(view);
  if (days == null) {
    return { word: (t) => t("contact.rail.thin"), tone: undefined };
  }
  if (isQuiet(days)) {
    return { word: (t) => t("contact.rail.atRisk"), tone: "danger" };
  }
  return { word: (t) => t("contact.rail.strong"), tone: "success" };
}

// Which way the relationship runs, from the two directional timestamps: they
// arrive together or not at all, one grant governs both.
export function directionWord(
  view: Contact360,
  t: ReturnType<typeof useT>,
): string {
  const inbound = view.last_inbound_at;
  const outbound = view.last_outbound_at;
  if (inbound && outbound) {
    return t("contact.rail.twoWay");
  }
  if (inbound) {
    return t("contact.rail.inboundOnly");
  }
  if (outbound) {
    return t("contact.rail.outboundOnly");
  }
  return t("contact.rail.noDirection");
}

// How many colleagues also know them, as a counted phrase.
export function colleagueWords(count: number, locale: Locale): string {
  return translatePlural(locale, "contact.rail.colleagues", count, {
    count: formatNumber(count, locale),
  });
}

// The one line of standing the head carries: the verdict word and, when the
// relationship has a direction, the trend it is on ("Strong · warming"). One
// Badge, because a label in a pill is one badge; the tone is the verdict's
// alone, the trend is a word after it. Nothing when the reader may not see
// the touch dates: a verdict drawn over withheld facts is a guess in a pill.
export function contactStanding(
  view: Contact360,
  t: ReturnType<typeof useT>,
): { words: string; tone: "success" | "danger" | undefined } | null {
  if (withheldSections(view).lastTouch) {
    return null;
  }
  const overall = overallVerdict(view);
  const trend = view.last_inbound_at ? trendWord(view, t) : null;
  return {
    words: trend
      ? `${overall.word(t)} · ${trend.toLowerCase()}`
      : overall.word(t),
    tone: overall.tone,
  };
}

// Why the head says what it says, as one short text for a tooltip: the
// verdict's sentence and the trend's. Words, not a panel: the chip's press
// leads to the brief, where the readings behind the verdict are laid out.
export function standingSentences(
  view: Contact360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const days = daysSinceInbound(view);
  const quietDays = formatNumber(QUIET_AFTER_DAYS, locale);
  const why =
    days == null
      ? t("contact.standing.why.thin")
      : isQuiet(days)
        ? t("contact.standing.why.atRisk", { days: quietDays })
        : t("contact.standing.why.strong", { days: quietDays });
  const trend = trendSentence(view, t);
  return trend ? `${why} ${trend}` : why;
}

// Which way the relationship runs, as a sentence: what the direction word
// stands for, for a reader who hovers it.
export function directionSentence(
  view: Contact360,
  t: ReturnType<typeof useT>,
): string {
  const inbound = view.last_inbound_at;
  const outbound = view.last_outbound_at;
  if (inbound && outbound) {
    return t("contact.standing.direction.twoWay");
  }
  if (inbound) {
    return t("contact.standing.direction.inboundOnly");
  }
  if (outbound) {
    return t("contact.standing.direction.outboundOnly");
  }
  return t("contact.standing.direction.none");
}

// The trend as a sentence, on the same rule `trendWord` draws the word from:
// nothing without an inbound message, cooling when ours is the newer one.
export function trendSentence(
  view: Contact360,
  t: ReturnType<typeof useT>,
): string | null {
  switch (trendKind(view)) {
    case "none":
      return null;
    case "cooling":
      return t("contact.standing.trend.cooling");
    case "warming":
      return t("contact.standing.trend.warming");
  }
}
