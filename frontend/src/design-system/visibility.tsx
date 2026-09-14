// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  EyeOff,
  Lock,
  type LucideIcon,
  UserRoundCheck,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";

import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./visibility.css";

// Who may read a thing, as one mark the whole product draws the same way.
//
// A message, a contact and a note each answer the question "who else sees
// this?", and each answered it in its own spelling: a dot-and-word badge on a
// mail row, a sentence in a panel on a contact, nothing at all on a limited
// note. A reader learns one of them and misses the others. This is the one
// mark — an icon that says the SHAPE of the audience at a glance (open, sealed,
// named, not yours) and a word that names it — so the state reads the same on
// a row, in a drawer header and beside a contact.
//
// It is a fact, not a status, which is why it is not a `Badge`: `Badge`'s
// tones say whether something is going well, and a message limited to its
// participants is a message working exactly as intended. Only `withheld` takes
// a tone, because it is the one state about the READER rather than the thing.

/**
 * The closed vocabulary. `team` is what a mail row and a shared contact both
 * mean — everyone the linked record already admits, which is why the word is
 * not "everyone". `private` is a captured contact's owner-only state; a message
 * has no such state, because the contacts on it can always read it.
 */
export type Visibility =
  | "team"
  | "participants"
  | "selected"
  | "private"
  | "withheld";

const WORD: Record<Visibility, MessageKey> = {
  team: "visibility.team",
  participants: "visibility.participants",
  selected: "visibility.selected",
  private: "visibility.private",
  withheld: "visibility.withheld",
};

// `Lock` twice on purpose: "participants" and "private" are both a sealed
// thing, and the word beside the icon says who holds the key. A different
// glyph per state would be five shapes to learn for a question with three
// answers — open, sealed, or not yours.
const ICON: Record<Visibility, LucideIcon> = {
  team: Users,
  participants: Lock,
  selected: UserRoundCheck,
  private: Lock,
  withheld: EyeOff,
};

// Three looks for five states. Open is outlined and quiet; every limit is
// filled, so a sealed thing is heavier on the page than an open one; withheld
// is the caution, because it is why the reader sees no content.
function look(state: Visibility): "open" | "limited" | "withheld" {
  if (state === "team") {
    return "open";
  }
  return state === "withheld" ? "withheld" : "limited";
}

export function VisibilityBadge({ state }: Readonly<{ state: Visibility }>) {
  const t = useT();
  const Icon = ICON[state];
  return (
    <span className={`visibility visibility-${look(state)}`}>
      <Icon aria-hidden="true" />
      {t(WORD[state])}
    </span>
  );
}

/**
 * The mark with the control that changes it, on one line.
 *
 * The verb sits beside the fact it acts on rather than under a paragraph
 * further down: "Make private" three lines away from the word it flips is a
 * button a reader has to connect for themselves. `marks` is for a second fact
 * about the same audience — the reason a captured message is held — and stays
 * on the badge's side of the line, between the mark and the verb, because it
 * explains what the mark says rather than what the press does.
 *
 * `action` follows them immediately instead of taking the far end of the row:
 * the whole claim of this component is that the verb reads AS a change to that
 * mark, and a gap as wide as the surface is what breaks it. Absent draws no
 * slot at all. A `.link-button` rather than a filled box, since the fact is
 * what the line is for and a bordered control beside a badge reads as the
 * louder of the two.
 */
export function VisibilityLine({
  state,
  marks,
  action,
}: Readonly<{
  state: Visibility;
  marks?: ReactNode;
  action?: ReactNode;
}>) {
  return (
    <div className="visibility-line">
      <VisibilityBadge state={state} />
      {marks}
      {action && <span className="visibility-line__action">{action}</span>}
    </div>
  );
}
