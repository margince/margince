// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * One record, listed somewhere else: its mark, its name as the way in, what it
 * is to the record around it, and the handles a reader can act on.
 *
 * A reference to a person or a company was a name and a link. That is enough
 * to navigate and nothing else — a reader deciding which of three contacts to
 * call had to open all three, and a rail listing an account's people said only
 * that they exist. The card carries the facts that decision needs, so the
 * decision is made before the click rather than after it.
 *
 * It is for a record LISTED as a reference — the account's people, on the
 * glance and in the rail. A name appearing inline in a sentence or a fact's
 * value is not this: expanding every mention is how a page stops being
 * readable, and `EntityRef` stays the shape there.
 *
 * The card is NOT the link. The name is, and so is each handle beside it —
 * a card-wide anchor with a `mailto:` inside it is a control inside a control,
 * which `nested-interactive` fails and which leaves a reader unable to say
 * which of the two a press will take. So the mark and the name share one
 * generous target, and every other affordance is its own.
 */

import type { ReactNode } from "react";
import { Avatar } from "./atoms";
import { ContactLink } from "./contactlink";
import "./recordcard.css";

export function RecordCard({
  kind,
  name,
  href,
  identity,
  position,
  email,
  aside,
}: Readonly<{
  /**
   * What the card stands for, which the mark's shape says before a word of it
   * is read: a person is round the way a face is, a company a rounded square
   * the way a logo is. `Avatar` owns the distinction; the card passes it on.
   */
  kind: "person" | "organization";
  name: string;
  // The record's own page.
  href: string;
  /**
   * What the mark's tint is derived from, when a stable id is at hand. Passed
   * through to `Avatar`, which explains why a name is the poor key: renaming a
   * record moves it to another colour on every screen at once.
   */
  identity?: string;
  /**
   * What this record is to the one listing it — a job title, what somebody
   * was at a former employer. A node rather than a string because the value
   * can carry its own provenance: a bought title says so where it stands.
   *
   * Not `role`: on a component that name reads as the ARIA attribute, to a
   * linter and to the next author alike.
   */
  position?: ReactNode;
  email?: string;
  /**
   * What the surrounding surface knows that the record itself does not — which
   * colleagues are already in touch, whether an employment is the current one.
   * It sits at the trailing edge, away from the record's own facts.
   */
  aside?: ReactNode;
}>) {
  // One target for the mark and the name together: a monogram beside a link
  // that goes to the same place is a second, smaller version of the same
  // target, and a reader who aims at the face gets nothing.
  const mark = (
    <>
      <Avatar name={name} identity={identity} shape={kind} />
      <span className="record-card-name">{name}</span>
    </>
  );
  return (
    <div className="record-card">
      {/* Labelled with the name alone: the monogram inside is TEXT, so it
          would otherwise lead the computed name and a screen reader would
          announce two initials before the person they stand for. */}
      <a className="record-card-open" href={href} aria-label={name}>
        {mark}
      </a>
      {(position || email) && (
        <div className="record-card-facts">
          {position && <p className="record-card-position">{position}</p>}
          {email && (
            <p className="record-card-handles">
              <ContactLink kind="email" value={email} />
            </p>
          )}
        </div>
      )}
      {aside && <div className="record-card-aside">{aside}</div>}
    </div>
  );
}
