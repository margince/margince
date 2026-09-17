// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the selected row is ABOUT, beside the queue.
//
// A row says why it is on the page. It cannot say what else is true of the
// contact or deal behind it — whether they have other open work, when anybody
// last spoke to them, what the account is worth — and a rep deciding how to
// answer needs that. Today the only way to see it is to leave the queue, which
// costs the reader their place in it.
//
// So the record's own 360 read is drawn beside the list. It is the SAME read
// the record page makes, not a second assembly of the same facts: a pane that
// composed its own view of a contact would be a second answer to "what do we
// know about them", and the two would drift.

import { Avatar } from "../design-system/atoms";
import { type Fact, FactList } from "../design-system/factlist";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { useContact360 } from "./contact360";
import { EntityRef } from "./entityref";
import type { WorklistItem } from "./worklist.queries";

// The pane, for whichever record the selected row is about.
//
// Only a CONTACT is drawn today. A deal-bearing row already carries its own
// figures — amount, close date, owner, risk evidence — on the row itself, so a
// pane repeating them would be the second spelling this file exists to avoid;
// what a deal row lacks is its timeline, and that is its own change. A row
// about neither draws nothing rather than an empty frame.
export function WorklistPane({ item }: Readonly<{ item: WorklistItem }>) {
  const subject = item.subject;
  if (subject?.type !== "contact") {
    return null;
  }
  return <ContactContext id={subject.id} label={subject.label} />;
}

// Whether this row HAS a pane, asked before one is rendered.
//
// A component returning null is still an element, and an element handed to
// PageZones still gets its aside column and its landmark. The caller has to be
// able to ask the question without rendering the answer, so the rule lives
// here — beside the component that obeys it — rather than in the screen.
export function hasPane(item: WorklistItem | undefined): boolean {
  return item?.subject?.type === "contact";
}

// One contact's context: who they are, and what else is open with them.
//
// The TITLE is the contact, and it is a link to their record — a pane naming
// somebody a rep is about to write to had their name as dead text, so the one
// obvious way to the whole relationship was to go back to the row and find its
// own link. `EntityRef` is handed the name the row already carried, so the
// link costs no lookup; the avatar rides beside it because a face is how a rep
// recognises whose day they are in before they read a word.
function ContactContext({
  id,
  label,
}: Readonly<{ id: string; label?: string }>) {
  const t = useT();
  const view = useContact360(id);
  const state = view.isPending
    ? "loading"
    : view.isError
      ? "failed"
      : ("ready" as const);
  // The row's own label first, the record's full name once it lands: the pane
  // draws its head before the read answers, and a title that changed from a
  // generic word to a name would move the reader's eye for nothing. A contact
  // with neither is unnameable rather than unlinkable, so the generic title
  // stands and no link is drawn round it.
  //
  // `contact` is required on the wire, so an answer without it is version skew
  // rather than a state the server means — and the honest fallback for a NAME
  // is the generic title, not a page that stops drawing. The board beside this
  // reads a missing count the same way and for the same reason.
  const name = label ?? view.data?.contact?.full_name;
  return (
    <Panel
      title={
        name ? (
          <span className="worklist-pane-head">
            <Avatar name={name} identity={id} />
            <EntityRef kind="contact" id={id} name={name} />
          </span>
        ) : (
          t("worklist.pane.title")
        )
      }
    >
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("worklist.pane.nothing")}
          loadingLabel={t("worklist.pane.loading")}
          detail={{ onRetry: () => void view.refetch() }}
        >
          {view.data && <ContactFacts view={view.data} />}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

// The facts a rep answering this row would otherwise open a second page for.
//
// Deliberately few, and chosen for the question the queue leaves open: a row
// says a customer is waiting, and what it cannot say is how long the silence
// has run in BOTH directions. A rep who last wrote yesterday answers
// differently from one who has not written since March.
//
// Who they work for and what they do are the other two, and they are here
// because the READ already carries them — `contact.employer` and
// `contact.title` come with the 360 the pane is drawing anyway, so naming them
// costs no request. Both are DROPPED when absent rather than drawn blank: an
// absent employer is not "works nowhere", it is also the answer for a reader
// with no grant on relationship edges, and a row saying nothing claims we know
// it and it is empty.
//
// A pane that reproduced the record page would be the record page in a
// narrower column, and the reader who wanted that has the title's own link.
function ContactFacts({
  view,
}: Readonly<{ view: NonNullable<ReturnType<typeof useContact360>["data"]> }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // Both come off `contact`, which is required on the wire — so an answer
  // without it is version skew, and the two facts it carries are simply
  // absent. Absent is already this list's ordinary case: a row is dropped
  // rather than drawn blank.
  const employer = view.contact?.employer;
  const role = view.contact?.title;
  // Required on the wire; an answer without it is version skew, read as a
  // record with nothing withheld rather than as one with nothing to say.
  const withheld = (view.sections_omitted ?? []).includes("last_touch");
  const facts: Fact[] = [
    ...lastTouch(withheld ? undefined : view, t, locale, zone).map((fact) => ({
      key: fact.term,
      term: fact.term,
      value: fact.value,
    })),
    // The company is a LINK, for the reason the contact's name is: a rep
    // deciding how to answer often needs the account rather than the contact,
    // and the name is already resolved on this read.
    ...(employer
      ? [
          {
            key: "company",
            term: t("worklist.pane.company"),
            value: (
              <EntityRef
                kind="company"
                id={employer.company_id}
                name={employer.company_name}
              />
            ),
          },
        ]
      : []),
    ...(role
      ? [
          {
            key: "role",
            term: t("worklist.pane.role"),
            value: role,
          },
        ]
      : []),
  ];
  return <FactList facts={facts} />;
}

// When somebody last wrote, or that nobody has.
//
// "Never" is a reading rather than a gap: a customer nobody has ever answered
// is the strongest case on the page for answering now, and an em dash would
// leave the reader to guess whether the fact was missing or the silence real.
function spoken(
  at: string | null | undefined,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  return at ? formatDateTime(at, locale, zone) : t("worklist.pane.never");
}

/** The two moments as any read spells them: the contact's own page and a
 *  queue row's `contact.touch` carry the same pair under the same names. */
export type TouchMoments = Readonly<{
  last_inbound_at?: string | null;
  last_outbound_at?: string | null;
}>;

/**
 * When they last wrote and when we did, as the two facts every surface that
 * answers a row prints — the pane beside the queue, the Brief's row in hand
 * and every queue row — so one relationship is never described two ways.
 * Spelled once here, beside the read that first carried them.
 *
 * NOTHING when the moments were withheld: a reader without the activity grant
 * is not told "Never", which is a claim about the relationship the server
 * declined to make. The caller passes nothing then, and a missing pair printed
 * as "Never" would be the wrong fact rather than no fact.
 */
export function lastTouch(
  touch: TouchMoments | undefined,
  t: Translator,
  locale: Locale,
  zone: string,
): readonly { term: string; value: string }[] {
  if (!touch) return [];
  return [
    {
      term: t("worklist.pane.lastInbound"),
      value: spoken(touch.last_inbound_at, t, locale, zone),
    },
    {
      term: t("worklist.pane.lastOutbound"),
      value: spoken(touch.last_outbound_at, t, locale, zone),
    },
  ];
}
