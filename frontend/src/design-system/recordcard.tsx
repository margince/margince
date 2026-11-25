// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * One record, listed somewhere else: its mark, its name as the way in, what it
 * is to the record around it, and the handles a reader can act on.
 *
 * A reference to a contact or a company was a name and a link. That is enough
 * to navigate and nothing else — a reader deciding which of three contacts to
 * call had to open all three, and a rail listing an account's contacts said only
 * that they exist. The card carries the facts that decision needs, so the
 * decision is made before the click rather than after it.
 *
 * It is for a record LISTED as a reference — the account's contacts, on the
 * glance and in the rail, the companies and contacts a search finds. A name
 * appearing inline in a sentence or a fact's value is not this: expanding
 * every mention is how a page stops being readable, and `EntityRef` stays the
 * shape there.
 *
 * The card is NOT the link. The name is, and so is each handle beside it —
 * a card-wide anchor with a button inside it is a control inside a control,
 * which `nested-interactive` fails and which leaves a reader unable to say
 * which of the two a press will take. So the mark and the name share one
 * generous target, and every other affordance is its own.
 */

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { AppLink } from "./applink";
import { Avatar } from "./atoms";
import { ContactLink } from "./contactlink";
import "./recordcard.css";

/**
 * The way in: the record's own page, or, for a record with no page (an
 * activity), the panel the caller opens beside the list.
 */
type RecordCardWay =
  | Readonly<{ href: string; onOpen?: never }>
  | Readonly<{ href?: never; onOpen: () => void }>;

/** What the card is and what it is called. */
type RecordCardMark = Readonly<{
  // What the card stands for, and so what a message to its address is filed
  // under. A deal and an activity have no address, so only these two take one.
  kind: "contact" | "company" | "deal" | "activity";
  name: string;
  // A record with no face (a deal, an activity) wears its kind's icon where
  // the avatar goes.
  icon?: LucideIcon;
}>;

type RecordCardFacts = Readonly<{
  /**
   * The record's own id. It keys the mark's mesh through `Avatar`, which
   * explains why a name is the poor key: renaming a record moves it to another
   * colour on every screen at once. It is also what a message to the card's
   * address is filed under, which is why it is not optional.
   */
  identity: string;
  // A company's resolved logo, drawn on the mark. `Avatar` keeps the monogram
  // under it while it loads, if it fails, and when there is none.
  logo?: string | null;
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
}>;

export function RecordCard({
  kind,
  name,
  href,
  onOpen,
  icon: Icon,
  identity,
  logo,
  position,
  email,
  aside,
}: RecordCardMark & RecordCardWay & RecordCardFacts) {
  // One target for the mark and the name together: a monogram beside a link
  // that goes to the same place is a second, smaller version of the same
  // target, and a reader who aims at the face gets nothing.
  const mark = (
    <>
      {Icon ? (
        <span className="record-card-icon">
          <Icon aria-hidden="true" />
        </span>
      ) : (
        <Avatar name={name} identity={identity} src={logo} />
      )}
      <span className="record-card-name">{name}</span>
    </>
  );
  const addressed = kind === "contact" || kind === "company" ? kind : null;
  return (
    <div className="record-card">
      {href !== undefined ? (
        <AppLink className="record-card-open" href={href}>
          {mark}
        </AppLink>
      ) : (
        <button
          type="button"
          className="record-card-open"
          aria-haspopup="dialog"
          onClick={onOpen}
        >
          {mark}
        </button>
      )}
      {(position || email) && (
        <div className="record-card-facts">
          {position && <p className="record-card-position">{position}</p>}
          {email && addressed && (
            <p className="record-card-handles">
              <ContactLink
                kind="email"
                value={email}
                record={{ entityType: addressed, entityId: identity }}
              />
            </p>
          )}
        </div>
      )}
      {aside && <div className="record-card-aside">{aside}</div>}
    </div>
  );
}

/**
 * A stack of `RecordCard`s, each in its own `<li>`. The list owns the gap, so
 * a surface that stacks cards never draws them edge to edge.
 */
export function RecordCardList({
  children,
}: Readonly<{ children: ReactNode }>) {
  return <ul className="record-card-list">{children}</ul>;
}
