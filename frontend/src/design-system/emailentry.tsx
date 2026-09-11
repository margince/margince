// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AlertTriangle, Mail, Paperclip } from "lucide-react";

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { VisibilityBadge } from "./visibility";
import "./emailentry.css";

// One retained email, as a row.
//
// The tree had four independent readings of a message — the timeline's, the
// company page's recent list, the person memory's fold, and the relationship
// spine's — each deciding for itself which parts of an email to show and
// whether it could be opened. This is the one row, and its layout is fixed:
// screens pass the server's own summary and where they sit, never a density, a
// variant, or nodes to render. That refusal IS the component. A `compact` prop
// would be four layouts wearing one name, which is what the four readings
// already were.
//
// A surface that only needs to CITE a message wants EmailReference, which
// shows the subject and the date and no preview.

type EmailSummary = components["schemas"]["EmailSummary"];

const MOVE_LABEL: Record<string, MessageKey> = {
  needs_reply: "email.move.needsReply",
  waiting_for_them: "email.move.waitingForThem",
};

// The row's direction line, as one whole sentence.
//
// Each direction has a form WITH a name and a form without, and the STRING
// owns the sentence rather than being glued to a name in the browser. Gluing
// is what produced "Received from" with nothing after it on every row whose
// counterparty is absent — which is every withheld row, and every row the
// summary carries no counterparty for. A preposition with no object reads as
// an unfinished render rather than as a deliberate absence.
//
// It also does not translate: German and Vietnamese put the name somewhere a
// `${direction} ${name}` template cannot.
//
// Direction is nullable, and an unknown one is not an outbound one — saying
// "Sent to" about a message nobody recorded a direction for is a claim the row
// does not have, so it answers null and the caller says "A message".
//
// The OUTBOUND verb comes from the delivery, not from the direction. "Sent"
// was drawn from direction alone, which made it a claim the row could not
// support: a message parked because the channel refused its files, or one the
// receiving system handed straight back, read exactly like one the provider
// confirmed, and the rep was told their mail went. Direction says which way a
// message points; only the delivery says whether it arrived.
function directionLine(
  direction: EmailSummary["direction"],
  counterparty: string | null | undefined,
  outbound: OutboundState,
  t: ReturnType<typeof useT>,
): string | null {
  // TRIMMED, and blank counts as absent: the server sends "" for a
  // counterparty it could not resolve, and "" is not null — so a bare presence
  // check picked the named form and rendered "Received from " with a trailing
  // space. That is the same dangling preposition this function exists to
  // remove, one character longer.
  const name = counterparty?.trim() ?? "";
  if (direction === "inbound") {
    return name ? t("email.receivedFrom", { who: name }) : t("email.received");
  }
  if (direction === "outbound") {
    const [named, bare] = OUTBOUND_PHRASE[outbound];
    return name ? t(named, { who: name }) : t(bare);
  }
  return null;
}

/**
 * What an outbound row may say about itself: the delivery's own state, or
 * `withheld` for a reader the message is not for.
 *
 * Withheld is a state here rather than a branch elsewhere because it answers
 * the same question — which verb this row has earned. A withheld row keeps the
 * direction and loses everything the delivery says, so it can claim neither
 * that the message went nor that it did not.
 */
type OutboundState =
  | NonNullable<EmailSummary["delivery"]>["state"]
  | "withheld";

// The outbound verb per state, named and bare.
//
// A message with NO delivery row falls on `sent`, and that is not a guess: it
// was logged rather than handed to a provider — somebody recording a mail they
// sent from their own client — and the record of it is the rep's own claim
// that it went.
const OUTBOUND_PHRASE: Record<
  OutboundState,
  readonly [MessageKey, MessageKey]
> = {
  pending: ["email.sendingTo", "email.sending"],
  sent: ["email.sentTo", "email.sent"],
  parked: ["email.notSentTo", "email.notSent"],
  bounced: ["email.bouncedFrom", "email.bounced"],
  withheld: ["email.outgoingTo", "email.outgoing"],
};

// Why a message did not arrive, in the words it was recorded with — the half
// of the answer a rep can act on.
//
// Only for a state that HAS one. A sent message's reason would be the last
// park it recovered from, and a pending one has not been attempted, so there
// is nothing yet to say about why it stopped.
function troubleReason(delivery: EmailSummary["delivery"]): string | null {
  if (delivery?.state !== "parked" && delivery?.state !== "bounced") {
    return null;
  }
  return delivery.reason?.trim() || null;
}

// What the row draws, decided in one place.
//
// Every field the withheld status governs is settled here TOGETHER, from the
// row's own reading of the status rather than from trusting the payload to
// have been stripped. The server does strip it; this is the second lock, and
// it exists because a response assembled by a path that forgot would otherwise
// print a counterparty's name beside a message the reader may not open.
function rowFields(summary: EmailSummary, t: ReturnType<typeof useT>) {
  const withheld = summary.display_status === "withheld";
  const counterparty = withheld ? null : summary.counterparty;
  // A withheld row says nothing about delivery, for the reason it says nothing
  // about attachments: "this message you may not read was parked because the
  // recipient blocked us" is the content it just refused, in smaller print.
  // The server already strips it; this is the second lock, as everywhere here.
  const delivery = withheld ? undefined : summary.delivery;
  const direction = directionLine(
    summary.direction,
    counterparty,
    withheld ? "withheld" : (delivery?.state ?? "sent"),
    t,
  );
  const staged = delivery?.files;
  return {
    trouble: troubleReason(delivery),
    // The names of the files, for the chip's tooltip.
    stagedNames: staged?.map((file) => file.filename) ?? null,
    withheld,
    // A withheld row keeps its shape and loses its words. Drawing it as absent
    // would leave a reader unable to tell a limited conversation from one that
    // never happened; drawing it as empty would say there was nothing to read,
    // which is a claim about the message rather than about them.
    subject: withheld
      ? t("email.withheldSubject")
      : summary.subject?.trim() || t("email.noSubject"),
    who: direction ?? t("email.aMessage"),
    preview: withheld ? null : summary.preview,
    move: withheld || summary.move === "none" ? null : MOVE_LABEL[summary.move],
    // What the message CARRIED, preferring the snapshot the send froze over
    // the live attachment list. Archiving or superseding a document later
    // changes what the library holds and must change nothing about what the
    // timeline says went out with a message that already left. The live count
    // still answers for an inbound message and for one that was only logged,
    // neither of which was ever staged.
    attachments: withheld ? 0 : (staged?.length ?? summary.attachment_count),
  };
}

/**
 * Why a row does not open, for the two cases where that is the honest answer.
 *
 * `noDetail` — the row has no message to open: a thread projection standing
 * for several messages, or an entry the server gave no activity id.
 * `noReader` — the surface itself mounts no drawer, so there is nowhere to
 * open INTO. The Brief is the one such surface.
 * `withheld` — the message exists and its content is not this reader's. The
 * row keeps its shape and loses its words, which is what `rowFields` already
 * does to every field the status governs; this makes the OPENER honest about
 * the same fact. Borrowing `noDetail` here would say there is no message,
 * and `noReader` would blame the surface for a limit that belongs to the
 * message — both are the wrong sentence about a row a reader will ask about.
 *
 * Naming the reason is the point. An optional opener could not tell these
 * apart from a surface that simply forgot to pass one, and a forgotten opener
 * renders a full-fidelity preview that does nothing — the defect this union
 * exists to make unwritable.
 */
type NoOpenReason = "noDetail" | "noReader" | "withheld";

/**
 * EmailWords is a message's WORDS and nothing else — the server's own preview
 * line, with the signature and the quoted history already removed.
 *
 * It exists for the one host that has already drawn everything else: a thread
 * card places the conversation on the axis, says what kind it is and prints the
 * sender and the time on each member, so a member row needs the words alone.
 * EmailEntry there would be a second lead line and a second timestamp over the
 * card's own, and EmailReference carries no preview by design, so the words had
 * no canonical spelling and the card wrote its own.
 *
 * The reason it lives HERE rather than in the host is `rowFields`: a withheld
 * message loses its words, and that rule has to be spelled once. The host read
 * `preview` off the summary directly and took its withheld answer from a
 * different field on a different object, so the two could disagree about the
 * same message — which is the drift the canonical row exists to stop, arriving
 * by the one route the row could not cover.
 */
export function EmailWords({ summary }: Readonly<{ summary: EmailSummary }>) {
  const t = useT();
  const { preview } = rowFields(summary, t);
  // Nothing drawn rather than an empty line: the server composes this, so no
  // preview means the sender wrote none — not that the row is still loading.
  if (!preview) {
    return null;
  }
  return <span className="emailentry__words">{preview}</span>;
}

export function EmailEntry({
  summary,
  timestamp,
  ...opener
}: Readonly<{
  /** The server's own row model. Nothing here is derived in the browser. */
  summary: EmailSummary;
  /** Formatted by the caller, which owns the reader's timezone. */
  timestamp: string;
}> &
  Readonly<
    /**
     * Openable, or explicitly not — never silently neither.
     *
     * A caller threading a callback it cannot guarantee (an optional prop, a
     * row that may carry no activity id) takes the first branch and states the
     * fallback reason too: `onOpen` undefined then means what the caller says
     * it means, rather than meaning nobody thought about it.
     */
    | { onOpen: () => void }
    | { onOpen: undefined; whyNotOpenable: NoOpenReason }
    | { whyNotOpenable: NoOpenReason }
  >) {
  const t = useT();
  const { locale } = useLocale();
  const row = rowFields(summary, t);

  const content = (
    <>
      <span className="emailentry__lead">
        <Mail aria-hidden="true" />
        {/* The row's KIND, for a reader who cannot see the envelope. Every
            other timeline kind announces itself through a Badge; this one
            said "email" in an icon and nothing else, so a screen reader was
            told what happened without being told what kind of thing it was. */}
        <span className="sr-only">{t("timeline.kind.email")}</span>
        <span className="emailentry__who">{row.who}</span>
        <span className="emailentry__when">{timestamp}</span>
      </span>
      <span className="emailentry__subject">{row.subject}</span>
      {/* No preview on a withheld row, and none invented when the message has
          no text of its own: the server composes this line, so an empty one
          means the sender wrote nothing rather than that the row is loading. */}
      {row.preview && (
        <span className="emailentry__preview">{row.preview}</span>
      )}
      <span className="emailentry__marks">
        {/* Who may read it, as the one mark the drawer and the contact panel
            draw too. `display_status` is the server's word and the mark's
            vocabulary contains every value of it, so the row prints the state
            it was sent and never re-derives one. */}
        <VisibilityBadge state={summary.display_status} />
        {row.move && <span className="emailentry__move">{t(row.move)}</span>}
        {row.attachments > 0 && (
          <span
            className="emailentry__files"
            // The names, for a reader who wants to know WHICH files without
            // opening the message. Only when the row has the snapshot: a live
            // count knows how many and not what.
            title={row.stagedNames?.join(", ") ?? undefined}
          >
            <Paperclip aria-hidden="true" />
            {formatNumber(row.attachments, locale)}
          </span>
        )}
      </span>
      {/* Why it did not arrive. The lead line above already says that it did
          not; this is the part a rep can act on, and it is the message the
          park or the bounce was recorded with rather than a phrase invented
          here. */}
      {row.trouble && (
        <span className="emailentry__trouble">
          <AlertTriangle aria-hidden="true" />
          {row.trouble}
        </span>
      )}
    </>
  );

  const onOpen = "onOpen" in opener ? opener.onOpen : undefined;
  if (!onOpen) {
    return <div className="emailentry">{content}</div>;
  }
  return (
    <button
      type="button"
      className="emailentry emailentry--open"
      onClick={onOpen}
      aria-haspopup="dialog"
    >
      {content}
    </button>
  );
}
