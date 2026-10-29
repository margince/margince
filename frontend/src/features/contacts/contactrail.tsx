import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { CounterpartyHoldRow } from "../../components/counterparty-hold";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import { omitted, type SectionState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { navigate } from "../../lib/router";
import { useWriteRefusal } from "./contactactions";
import { type ConsentRead, useConsentRows } from "./contactconsentpanel";
import { useContactAboutSections } from "./contactdetails";

export type Contact360 = components["schemas"]["Contact360"];
type Contact = components["schemas"]["Contact"];
type RelationshipDirection = components["schemas"]["RelationshipDirection"];

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

/**
 * The left column under the head: About, Ownership and Consent & channels as
 * flat groups. Then the lists the contact is on, the one link to the empty
 * fields, and the page's footnote. Employers, billing roles, tags, the record's
 * source and the consent proof live on the Network and Research tabs.
 */
export function ContactRail({
  view,
  consent,
  refusals,
  writeRefusalId,
}: Readonly<{
  view: Contact360;
  consent: ConsentRead;
  // What the page refuses this reader, each sentence under the id its verbs
  // point at. The Email verb's own refusal joins them here.
  refusals?: ReactNode;
  writeRefusalId: string;
}>) {
  const t = useT();
  const writeRefusal = useWriteRefusal(view);
  const footnote = (refusals || writeRefusal) && (
    <>
      {writeRefusal && <span id={writeRefusalId}>{writeRefusal}</span>}
      {writeRefusal && refusals && " "}
      {refusals}
    </>
  );
  const about = useContactAboutSections(view);
  const consentRows = useConsentRows(view, consent);
  const listsOn = useListsAvailable();
  const sections: DetailSection[] = [
    ...about,
    { key: "consent", label: t("contact.aside.consent"), rows: consentRows },
  ];
  // The lists read answers 404 while lists are switched off, so it is asked
  // only where they are on.
  if (listsOn && !view.contact.archived_at) {
    return (
      <ContactRailWithLists
        contactId={view.contact.id}
        sections={sections}
        footnote={footnote}
      />
    );
  }
  return <RecordDetails sections={sections} footnote={footnote} />;
}

function ContactRailWithLists({
  contactId,
  sections,
  footnote,
}: Readonly<{
  contactId: string;
  sections: readonly DetailSection[];
  footnote?: ReactNode;
}>) {
  const t = useT();
  const lists = useRecordLists("contact", contactId).data?.data ?? [];
  // No memberships, no group: the design draws no empty state for it.
  const withLists =
    lists.length === 0
      ? sections
      : [
          ...sections,
          {
            key: "lists",
            label: t("lists.record.title"),
            children: (
              <div className="pe-list-chips">
                {lists.map((list) => (
                  <button
                    key={list.id}
                    type="button"
                    className="link-button"
                    onClick={() => navigate({ screen: "lists", id: list.id })}
                  >
                    <Badge>{list.name}</Badge>
                  </button>
                ))}
              </div>
            ),
          },
        ];
  return <RecordDetails sections={withLists} footnote={footnote} />;
}

// --- Keeping this correspondence private -------------------------------

// The hold control, in the rail's own section shape, drawn on Data & tools.
// Drawn for every contact with an address, held or not: a control that
// appeared only once a hold existed would leave a reader with no way to place
// the first one.
export function ContactHoldSection({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const email = view.contact?.primary_email;
  if (!email) {
    return null;
  }
  return (
    <Panel title={t("hold.sectionTitle")}>
      <PanelBody>
        <CounterpartyHoldRow scope={{ address: email }} />
      </PanelBody>
    </Panel>
  );
}

// --- Details -----------------------------------------------------------

export function useContactReadOnlyReason(contact: Contact): string | undefined {
  const t = useT();
  if (contact.archived_at) {
    return t("contact.rail.archivedReadOnly");
  }
  return undefined;
}

// --- Relationship standing --------------------------------------------

// Which way the relationship runs, as a sentence. The server decides it by the
// rule the company roll-up and the colleague graph also use; the page only
// words it. A server that sent no reading words it as "none".
const DIRECTION_SENTENCE: Record<RelationshipDirection, MessageKey> = {
  two_way: "contact.standing.direction.twoWay",
  inbound_only: "contact.standing.direction.inboundOnly",
  outbound_only: "contact.standing.direction.outboundOnly",
  none: "contact.standing.direction.none",
};

export function directionSentence(
  view: Contact360,
  t: ReturnType<typeof useT>,
): string {
  return t(DIRECTION_SENTENCE[view.relationship_direction ?? "none"]);
}
