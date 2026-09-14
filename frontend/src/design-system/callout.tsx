// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  CircleCheck,
  CircleX,
  Info,
  type LucideIcon,
  TriangleAlert,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { IconAction } from "./iconaction";
import "./callout.css";

// Callout: something the surface says ABOUT itself, rather than content.
//
// The product had fourteen spellings of this — three app banners duplicating
// the same inline style object, nine screen-local classes, and a handful of
// bare paragraphs tinted by hand. One of them, `.co-callout`, declared only a
// top margin: the name promised a callout and the sheet delivered whitespace.
//
// The tones are a closed set because they are claims, not decoration. `warn`
// says something will go wrong if you do nothing; `danger` says something is
// wrong or is about to be irreversible; `success` confirms an action landed;
// `info` is the default and carries no urgency at all; `accent` is `info` said
// emphatically. There is no sixth: a surface reaching for one is reaching for
// emphasis, which is what the words are for.

export type CalloutTone = "info" | "accent" | "warn" | "danger" | "success";

/**
 * What the notice IS, which is a different question from how loud it is.
 *
 * `outcome` answers something the reader just did — a save failed, an invite
 * landed. `standing` is true as the page renders — a licence in grace, a
 * missing prerequisite, help text. `event` reports something that happened
 * elsewhere or asynchronously — a job finished, a connector broke, work
 * arrived. That is a question a caller can answer about their own screen, and
 * `live` is then derived from it rather than chosen per site by whoever was
 * thinking about ARIA that afternoon.
 */
export type CalloutKind = "outcome" | "standing" | "event";

/**
 * The tone's glyph, so the claim reaches a reader who does not read colour.
 *
 * `CircleX` rather than `OctagonX` for danger: an octagon's eight short edges
 * alias into a circle at 16px, and the circular pair reads as one family beside
 * `CircleCheck` — the same outline, the opposite news.
 *
 * `accent` takes `Info` as well, because it is the same CLAIM as `info` said
 * emphatically: a second glyph there would tell a reader the two notices were
 * about different kinds of thing, when the only difference is how much of their
 * attention the surface is asking for.
 */
const TONE_ICONS: Readonly<Record<CalloutTone, LucideIcon>> = {
  info: Info,
  accent: Info,
  warn: TriangleAlert,
  danger: CircleX,
  success: CircleCheck,
};

/**
 * How a screen reader learns about a notice whose caller did not choose a
 * loudness. The table in `Callout`'s own doc is this function, in prose.
 */
function announcementFor(
  kind: CalloutKind | undefined,
  tone: CalloutTone,
): "status" | "alert" | undefined {
  if (kind === undefined || kind === "standing") return undefined;
  return kind === "outcome" && tone === "danger" ? "alert" : "status";
}

/**
 * A bordered notice on the pane's ground: the tone's icon, the heading in the
 * tone's ink, the words in ordinary ink, then whatever the reader can do about
 * it. Never a filled coloured box — colour on a page means one thing at a time,
 * and a tinted plate shouts over the record it sits in.
 *
 * ONE anatomy, and the heading is part of it rather than a choice: icon,
 * heading over an OPTIONAL body, `actions` in a column at the end which drops
 * below the words on a phone, then the dismiss. A heading alone is a complete
 * callout, and it is the commonest shape — most notices are one sentence, and
 * that sentence is the heading. What the two faces this replaced bought was
 * density; what they cost was a notice whose first words were sometimes a
 * heading and sometimes not, so no two of them scanned the same way down a page
 * and the tone reached the ink of one and not the other.
 *
 * `accent` is the emphasised information notice, in the brand accent: the same
 * role `Panel tone="accent"` plays — a lead, an ask, the one notice on a screen
 * that wants a MOVE rather than reporting state. Two of them on one screen is
 * no emphasis at all. It is deliberately NOT indigo: `--ai` is a claim about
 * provenance, that an agent authored what you are reading, so an emphatic
 * notice a contact wrote wearing indigo would tell every reader of that screen
 * something false about who decided.
 *
 * `live` decides how a screen reader learns about it; `kind` is the same
 * question asked in the caller's own terms. Passing neither is silent, exactly
 * as before, and an explicit `live` always wins. Otherwise:
 *
 * | `kind`     | tone     | role     |
 * |------------|----------|----------|
 * | `outcome`  | `danger` | `alert`  |
 * | `outcome`  | other    | `status` |
 * | `event`    | any      | `status` |
 * | `standing` | any      | none     |
 *
 * That is the honest reading of each: an outcome the reader caused and cannot
 * use is a failure they have to be told about, anything else that arrived while
 * they were reading is worth mentioning quietly, and a notice rendered WITH the
 * page has nothing to interrupt for. Passing `alert` to something merely
 * informative is how a reader learns to ignore all of them.
 *
 * There is no `className`, and that is the point of the prop's absence: a
 * notice needing air gets it from the stack it sits in, and one that wanted its
 * own colour, edge or plate was not this primitive at all — it was a second
 * callout wearing this one's name, which is the thing the fourteen spellings
 * above were.
 *
 * Copy never lives here: every word arrives as a prop, translated by the
 * caller.
 */
export function Callout({
  tone = "info",
  kind,
  icon: Icon = TONE_ICONS[tone],
  title,
  actions,
  live,
  dismiss,
  children,
}: Readonly<{
  tone?: CalloutTone;
  kind?: CalloutKind;
  /**
   * Replaces the tone's glyph, where the notice is about a nameable thing
   * rather than about how bad the news is.
   */
  icon?: LucideIcon;
  /** The notice's first words, and the one part of it that is not optional. */
  title: ReactNode;
  /** Buttons or links, laid out after the body. */
  actions?: ReactNode;
  live?: "status" | "alert";
  /**
   * The control that puts the notice away, at the end of the card. ONE prop
   * rather than two, so the handler and the word for it cannot arrive apart: an
   * unnamed `X` is the failure `IconAction` exists to prevent, and a label with
   * no handler is a control that does nothing. The label is the caller's
   * translated verb — this file holds no words.
   */
  dismiss?: Readonly<{ label: string; onDismiss: () => void }>;
  /** Whatever the heading does not already say, where there is more to say. */
  children?: ReactNode;
}>) {
  return (
    <div
      className={`callout callout-${tone}`}
      role={live ?? announcementFor(kind, tone)}
    >
      <span className="callout-icon" aria-hidden="true">
        <Icon size={16} strokeWidth={2} />
      </span>
      <div className="callout-body">
        <div className="callout-copy">
          <p className="callout-title">{title}</p>
          {children !== undefined && (
            <div className="callout-text">{children}</div>
          )}
        </div>
        {actions !== undefined && (
          <div className="callout-actions">{actions}</div>
        )}
      </div>
      {dismiss !== undefined && (
        <span className="callout-dismiss">
          <IconAction
            small
            label={dismiss.label}
            icon={<X aria-hidden />}
            onClick={dismiss.onDismiss}
          />
        </span>
      )}
    </div>
  );
}
