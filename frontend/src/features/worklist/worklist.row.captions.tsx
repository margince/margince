// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { ENTITY } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import { briefSubjectLabel, subjectHref } from "../../lib/worklistcopy";
import { employerOf } from "./worklist.meetingparties";
import type { HeldTouch } from "./worklist.pane";
import type { WorklistItem } from "./worklist.queries";
import { useDrawerDials, withWorklistReturn } from "./worklist.return";
import type { RowReadings } from "./worklist.row.compact";

// The frame already carries the date; the row retains its evidence and figures.
export function shapedReadings(
  framed: boolean,
  readings: RowReadings,
): RowReadings {
  if (!framed) return readings;
  return {
    ...readings,
    when: null,
    facts: briefSubjectLabel(readings.item) ? readings.facts : null,
  };
}

// The human behind a row, as an address — the sender of a thread filed under a
// deal, the attendee a meeting is read with — through the same registry, so
// the contact's page is spelled once.
export function contactHref(contact: NonNullable<WorklistItem["contact"]>) {
  return routeHash(ENTITY.contact.route(contact.id));
}

/** How a row links an address inside a Worklist: carrying that list's way back. */
export function useListed(): (href: string) => string {
  const drawer = useDrawerDials();
  return (href) => withWorklistReturn(href, drawer);
}

/**
 * Whom a row is about, linked — where the row does not already say so.
 *
 * A message names its sender as text and draws no title line, so it links the
 * contact behind it here; a thread filed under a deal still came from one,
 * and the reply goes to them. Every other title names and links its own record
 * (`itemTitle`, `rowHref`), so the line repeats nothing — unless the server
 * put a contact on the row that the record is not: a task about a deal, with
 * the contact it is owed to beside it.
 */
export function aboutRecord(
  item: WorklistItem,
  named: boolean,
  listed: (href: string) => string,
): RowReadings["about"] {
  const contact = item.contact;
  if (contact?.label && (named || contact.id !== item.subject?.id)) {
    const employer = employerOf(contact);
    return {
      href: listed(contactHref(contact)),
      label: contact.label,
      company: employer && {
        ...employer,
        href: listed(employer.href),
      },
    };
  }
  if (!named) return undefined;
  const label = item.subject?.label;
  const href = subjectHref(item);
  return label && href ? { href: listed(href), label } : undefined;
}

/**
 * How the silence runs both ways with whoever the row is about. A contact on
 * the row outranks its account, since the pair belongs to whoever a reply would
 * go to; the account's shows only on a row naming no one. Keyed on which
 * record is present rather than on which pair is, so a contact whose moments
 * were withheld never borrows the account's.
 */
export function rowTouch(item: WorklistItem): HeldTouch {
  return item.contact
    ? { holder: "contact", moments: item.contact.touch }
    : { holder: "company", moments: item.company?.touch };
}

/**
 * The pair as the queue's row prints it. The triage shape names them under
 * the row itself (brief.feed.tsx) and withholds this, as it withholds the
 * about line.
 */
export function touchOf(
  item: WorklistItem,
  framed: boolean,
): HeldTouch | undefined {
  return framed ? undefined : rowTouch(item);
}

export function RowCaptions({
  about,
  host,
  touch,
  when,
  facts,
  said,
}: Readonly<{
  about?: RowReadings["about"];
  host?: string | null;
  touch: RowReadings["touch"];
  when: string | null;
  facts: string | null;
  said: readonly string[];
}>) {
  return (
    <>
      {said.length > 0 && (
        <p className="worklist-row-because">{said.join(" · ")}</p>
      )}
      {/* When it starts, or when it is due, FIRST — it is the fact the reasons
          beside it are about: "starting shortly" explains a rank, and this says
          what time. */}
      <div className="worklist-row-facts-line">
        {/* WHO it is with, first: a rep answering a row asks whose row it is
            before how long it has waited. */}
        {about && (
          <p className="t-caption worklist-row-about">
            <AppLink className="entity-link" href={about.href}>
              {about.label}
            </AppLink>
          </p>
        )}
        {about?.company && (
          <p className="t-caption worklist-row-about">
            <AppLink className="entity-link" href={about.company.href}>
              {about.company.label}
            </AppLink>
          </p>
        )}
        {host && <p className="t-caption worklist-row-host">{host}</p>}
        {when && <p className="t-caption worklist-row-when">{when}</p>}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
      </div>
      {/* Which side wrote last, on a line of its own under the fragments: two
          dated facts read as one statement about the relationship, and the
          question a rep answers a waiting row with. */}
      {touch.length > 0 && (
        <p className="t-caption worklist-row-touch">
          {touch.map((fact) => (
            <span key={fact.term} className="worklist-row-touch-fact">
              <span>{fact.term}</span>{" "}
              <span className="t-num">{fact.value}</span>
            </span>
          ))}
        </p>
      )}
    </>
  );
}
