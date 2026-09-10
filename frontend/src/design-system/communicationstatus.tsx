// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHETHER THIS MESSAGE CAN GO, said once, in the place the send is decided.
//
// The surfaces that write to a contact each answered this their own way, and
// mostly by refusing: the person page greyed out its Email verb when consent
// was missing, the composer showed nothing at all when the answer had not come
// back, and the company header asked a different question entirely. A rep
// learned what the product thought by finding out what they could not press.
//
// A DISABLED CONTROL STATES THAT SOMETHING IS WRONG AND NOTHING ABOUT WHAT.
// That is the defect this replaces. Writing is always available; the mark says
// what the message would meet, and the reader decides.
//
// SIX STATES, and the split that matters is between two SCOPES, not six
// colours. `contact_preferences` is what is true about a person however you
// reach them — they asked us to stop, their address bounced. `current_message`
// is what is true about the message being written, which is a different
// question with a different answer: a customer who stopped the newsletter can
// still be sent their invoice. A mark that conflated them would tell a rep
// their invoice was refused because a subscription was.
//
// THE MARK IS NEVER THE ONLY SIGNAL. Every state carries a translated label,
// and the accessible name says the subject and the state in words. A reader who
// does not read colour, or reads none of it, gets the same answer.

import {
  ArrowUpRight,
  Info,
  Mail,
  MailCheck,
  MailMinus,
  MailWarning,
} from "lucide-react";
import type { ReactNode } from "react";
import { BusyMark } from "./atoms";
import { Popover } from "./popover";
import "./communicationstatus.css";

/**
 * What this surface is able to say about the message or the person.
 *
 * `context_only` is the honest default and the commonest state: nothing is
 * wrong and nothing has been asked. A contact surface with no message in hand
 * cannot answer whether a send would be allowed, because that answer depends on
 * what the message is — so it says what it knows and no more.
 */
export type CommunicationState =
  | "context_only"
  | "ready"
  | "attention"
  | "restricted"
  | "checking"
  | "external";

/**
 * WHICH QUESTION this mark is answering, which decides what its states mean.
 *
 * The scopes are not decoration. `ready` under `current_message` says this
 * message may go; `ready` under `contact_preferences` would be a claim about
 * every future message, which nothing can make. So the contact scope never
 * reaches `ready` at all — its best answer is `context_only`.
 */
export type CommunicationScope =
  | "contact_preferences"
  | "current_message"
  | "batch"
  | "historical_message"
  | "external_handoff";

// The glyph each state wears.
//
// A SHAPE PER STATE, not a colour per state. Four marks differing only in hue
// are one signal wearing four coats, and the reader who cannot tell them apart
// is the one the mark exists for. Colour rides along; the envelope's own
// silhouette carries the meaning.
//
// MailMinus for restricted, not MailX. A bar says "stopped"; a cross says
// "failed", and a subject who asked us not to write to them has not failed at
// anything. The distinction matters most on the surface where a rep is deciding
// how to feel about a contact.
const GLYPH: Record<CommunicationState, typeof Mail> = {
  context_only: Mail,
  ready: MailCheck,
  attention: MailWarning,
  restricted: MailMinus,
  checking: Mail,
  external: Mail,
};

/**
 * CommunicationStatus is the mark, its label, and the aside behind it.
 *
 * It renders one control: a Popover trigger carrying the glyph and, when
 * `showLabel` is set, the words beside it. The panel holds whatever `detail`
 * the caller gives it — the reasons, the dates, the record links — because what
 * is worth reading differs by surface and the mark does not know.
 */
export function CommunicationStatus({
  state,
  scope,
  label,
  name,
  showLabel,
  exception,
  freshness,
  size = 18,
  detail,
}: Readonly<{
  state: CommunicationState;
  scope: CommunicationScope;
  // The short line, already translated. The mark never builds copy: what a
  // stopped subscription should say differs by surface, and a component that
  // wrote its own sentence would say the same thing in every one of them.
  label: string;
  // The accessible name, already translated and already carrying the subject:
  // "Communication status for Anna: marketing stopped. View details." An
  // icon-only control with no name is a button a screen reader announces as
  // "button", which is the whole disclosure a blind rep would get.
  name: string;
  // Draws the label beside the glyph. The composer's footer does; a row in a
  // list does not, because forty labels down a table is noise and the same
  // words are one hover away.
  showLabel?: boolean;
  // "Recorded exception" and its like, shown as an adjunct.
  //
  // It NEVER RECOLOURS THE MARK, and that is the rule this prop exists to
  // keep. A message sent under a recorded exception was refused and sent
  // anyway; drawing it green would say the refusal was resolved, which is
  // exactly the record the exception path exists not to falsify. The original
  // risk stays visible and the exception sits beside it.
  exception?: string;
  // "Checked now · Will check again before sending". For a parked message,
  // where the answer on screen is older than the message and the reader has no
  // other way to know.
  freshness?: string;
  // 18 in prose and beside a verb; 16 in a dense row. The touch target stays 44
  // either way — see the stylesheet, which grows it with negative margins
  // rather than by making the glyph bigger.
  size?: 18 | 16;
  // What the panel holds. Absent, the trigger still opens and shows the label,
  // which is the honest minimum for a surface that knows only the state.
  detail?: ReactNode;
}>) {
  const drawn = drawnState(state, scope);
  const Glyph = GLYPH[drawn];
  return (
    <Popover
      className={[
        "commstatus",
        `commstatus-${drawn}`,
        size === 16 ? "commstatus-dense" : "commstatus-prose",
      ].join(" ")}
      onHover
      label={
        <span className="commstatus-face">
          {state === "checking" ? (
            <BusyMark className="commstatus-busy" />
          ) : (
            <Glyph size={size} aria-hidden="true" />
          )}
          {/* The two composed cues. Drawn as a second glyph over the corner of
              the first rather than as a distinct envelope, so `context_only`
              and `external` stay recognisably the same neutral mark with a
              note about where the message goes. */}
          {state === "context_only" && (
            <Info
              size={size === 18 ? 11 : 10}
              aria-hidden="true"
              className="commstatus-cue"
            />
          )}
          {state === "external" && (
            <ArrowUpRight
              size={size === 18 ? 11 : 10}
              aria-hidden="true"
              className="commstatus-cue"
            />
          )}
          {showLabel ? <span className="commstatus-label">{label}</span> : null}
          <span className="sr-only">{name}</span>
        </span>
      }
    >
      <div className="commstatus-panel">
        <p className="t-label commstatus-panel-head">{label}</p>
        {exception ? (
          <p className="t-caption commstatus-exception">{exception}</p>
        ) : null}
        {freshness ? (
          <p className="t-caption commstatus-freshness">{freshness}</p>
        ) : null}
        {detail}
      </div>
    </Popover>
  );
}

/**
 * CommunicationStatusLine seats the mark BESIDE the action, never inside it.
 *
 * One flex row: the mark first, then whatever verbs the surface offers. Inside
 * would nest a button in a button, which is invalid and which no reader can
 * operate — the outer press swallows the inner one, so the panel a rep reaches
 * for opens the composer instead.
 *
 * It is also what the send-surface census looks for. A surface that writes to a
 * contact and renders no mark is a surface where the old silence survives, and
 * the wrapper is the shape that makes the presence of one checkable.
 */
export function CommunicationStatusLine({
  status,
  children,
}: Readonly<{ status: ReactNode; children: ReactNode }>) {
  return (
    <span className="commstatus-line">
      {status}
      {children}
    </span>
  );
}

/**
 * What the mark actually draws, once the scope has had its say.
 *
 * ONE RULE, and it exists because the scopes answer different questions.
 * `ready` under `current_message` says THIS message may go, which a surface
 * holding a message can know. `ready` under `contact_preferences` would say
 * every future message may go, which nothing can know — the answer depends on
 * what the message turns out to be, and a contact surface has none in hand.
 *
 * So a contact surface asking for `ready` gets `context_only`: nothing is
 * wrong, and nothing has been asked. Softened here rather than refused at the
 * type level because the caller is often an adapter mapping a server answer,
 * and a surface that crashed on an over-confident reading would be worse than
 * one that drew a quieter true thing.
 *
 * Everything else passes through. A contact surface CAN say restricted — a
 * standing objection is true however you reach somebody — and it can say
 * attention, because a dead address is too.
 */
function drawnState(
  state: CommunicationState,
  scope: CommunicationScope,
): CommunicationState {
  if (state === "ready" && scope === "contact_preferences") {
    return "context_only";
  }
  return state;
}
