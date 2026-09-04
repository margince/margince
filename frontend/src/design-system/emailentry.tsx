// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail, Paperclip } from "lucide-react";

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { Badge } from "./atoms";
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
type EmailAccessStatus = components["schemas"]["EmailAccessStatus"];

// What each access state says on the row. One word, because a badge that ran
// to a sentence would out-weigh the subject beside it — the detail carries the
// sentence.
const STATUS_LABEL: Record<EmailAccessStatus, MessageKey> = {
  team: "email.access.team",
  participants: "email.access.participants",
  selected: "email.access.selected",
  withheld: "email.access.withheld",
};

const MOVE_LABEL: Record<string, MessageKey> = {
  needs_reply: "email.move.needsReply",
  waiting_for_them: "email.move.waitingForThem",
};

// `withheld` is the one state about the READER rather than the message, so it
// is the one drawn as a caution. The other three describe a message working as
// intended and take the quiet form: a column of filled pills is decoration a
// reader learns to skip.
function statusTone(status: EmailAccessStatus): "warn" | undefined {
  return status === "withheld" ? "warn" : undefined;
}

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
function directionLine(
  direction: EmailSummary["direction"],
  counterparty: string | null | undefined,
  t: ReturnType<typeof useT>,
): string | null {
  const named = counterparty !== null && counterparty !== undefined;
  if (direction === "inbound") {
    return named
      ? t("email.receivedFrom", { who: counterparty })
      : t("email.received");
  }
  if (direction === "outbound") {
    return named ? t("email.sentTo", { who: counterparty }) : t("email.sent");
  }
  return null;
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
  const direction = directionLine(summary.direction, counterparty, t);
  return {
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
    attachments: withheld ? 0 : summary.attachment_count,
  };
}

export function EmailEntry({
  summary,
  timestamp,
  onOpen,
}: Readonly<{
  /** The server's own row model. Nothing here is derived in the browser. */
  summary: EmailSummary;
  /** Formatted by the caller, which owns the reader's timezone. */
  timestamp: string;
  /**
   * Opens the canonical detail. Omitted only where there is no detail to open;
   * a row that looks openable and is not teaches a reader the product does not
   * work.
   */
  onOpen?: () => void;
}>) {
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
        <Badge tone={statusTone(summary.display_status)} quiet={!row.withheld}>
          {t(STATUS_LABEL[summary.display_status])}
        </Badge>
        {row.move && <span className="emailentry__move">{t(row.move)}</span>}
        {row.attachments > 0 && (
          <span className="emailentry__files">
            <Paperclip aria-hidden="true" />
            {formatNumber(row.attachments, locale)}
          </span>
        )}
      </span>
    </>
  );

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
