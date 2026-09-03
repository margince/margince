// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail } from "lucide-react";

import { useT } from "../i18n";
import "./emailreference.css";

// A CITATION of an email, not a rendering of one.
//
// The distinction is why this exists beside EmailEntry. A relationship
// chronology, a piece of brief evidence, a graph receipt — each needs to NAME
// a message and let a reader open it, and none of them is a place to read
// mail. Giving them EmailEntry would put a preview and an access badge inside
// an analytics layout; giving each its own subject line, which is what the
// tree did before, is four spellings of one citation.
//
// So: the subject, the date, and nothing else. No preview, because a reference
// is not a reading. No access badge, because a badge without the message it
// qualifies is a fact floating free of its subject. What it does carry is the
// opener — the same drawer EmailEntry opens, so a citation and a row lead to
// one place.
//
// The answer to "can this be a bit denser" is always this component rather
// than a prop on EmailEntry: a reference that grew a preview would be the
// density variant the whole arrangement exists to prevent.

export function EmailReference({
  subject,
  occurredAt,
  withheld = false,
  onOpen,
}: Readonly<{
  /** Null when the message has none, or when its content is not the reader's. */
  subject: string | null | undefined;
  /** Already formatted by the caller, which owns the reader's timezone. */
  occurredAt?: string;
  /**
   * Whether this reader is outside the message's audience. The citation takes
   * the state rather than trusting its caller to have blanked the subject: a
   * privacy rule that lives only in a prop contract is a rule the next caller
   * has to remember, and the surfaces that cite mail are exactly the ones that
   * assemble their own summaries.
   */
  withheld?: boolean;
  /**
   * Opens the canonical detail. Omitted when this reader may not read the
   * message: a control that opens nothing teaches a reader that citations do
   * not work, which costs more than the click it saves.
   */
  onOpen?: () => void;
}>) {
  const t = useT();
  const label = withheld
    ? t("email.withheldSubject")
    : subject?.trim() || t("email.noSubject");
  // A message this reader may not read has nothing to open, whatever the
  // caller passed.
  const open = withheld ? undefined : onOpen;
  const body = (
    <>
      <Mail aria-hidden="true" />
      <span className="emailref__subject">{label}</span>
      {occurredAt && <span className="emailref__when">{occurredAt}</span>}
    </>
  );
  if (!open) {
    return <span className="emailref">{body}</span>;
  }
  return (
    <button
      type="button"
      className="emailref emailref--open"
      onClick={open}
      aria-haspopup="dialog"
    >
      {body}
    </button>
  );
}
