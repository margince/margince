// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { CommunicationContext } from "../../lib/compose-context";
import type { Transport } from "../../lib/contacttransports";
import type { RelinkKind } from "./composerelink";
import type { useVoiceDraft } from "./composevoicedraft";
import type { EditSeed } from "./schedulededit";

type Activity = components["schemas"]["Activity"];

/** What a surface hands the composer: where it writes from, and what to. */
export type ComposeProps = Readonly<{
  // Absent means an ACCOUNT-STARTED send: a new conversation (ADR-0087).
  // Drafting, consent, refusal and provenance are shared with a reply. A
  // reply keeps threading and inherits its links. A new message files itself
  // under the record it started from.
  initialMessage?: Readonly<{ subject: string; body: string }>;
  initialReason?: CommunicationContext;
  initialDealId?: string;
  activityId?: string;
  entityType: RelinkKind;
  entityId: string;
  contactId?: string;
  /** A meeting follow-up: further recipients and the meeting it follows up. */
  followUp?: Parameters<typeof useVoiceDraft>[0]["followUp"];
  /**
   * The record's own email address, for a FIRST message to it — a lead or a
   * contact nobody has written to yet, where there is no thread to resolve a
   * counterparty from.
   *
   * The caller's, because the record is already on screen behind this drawer
   * and its address came with it: a lookup here would ask the server for a
   * field the page is holding. Offered only when nothing is being answered,
   * and offered the same way a thread's address is — into an empty field,
   * once, and never over what the reader typed.
   */
  recordAddress?: string;
  // Undefined (or any non-channel kind) keeps the mail behaviour this modal
  // already had before a channel existed — every mail test renders this
  // component without ever naming a kind.
  kind?: Activity["kind"];
  /**
   * The ways this record can be written to, when there is more than one.
   *
   * The CALLER's, because reachability is a fact about a contact and the record
   * page is already holding the payload it is read from — asking again here
   * would let the drawer and the verb that opened it disagree about what
   * pressing it does. A company, a deal and a lead pass none and get the mail
   * composer they have always had; only a contact reachable on a chat channel
   * has a choice to offer.
   */
  transports?: readonly Transport[];
  /** Which of them to open on — the transport the conversation a caller named
   *  is ON, rather than the record's own lead. The reader may still change it. */
  initialTransportId?: string;
  /** The named conversation cannot be answered any more. The composer opens
   *  on the default AND says so, rather than fall back silently. */
  staleThread?: boolean;
  /** What the caller wants the draft to be ABOUT. A moment action knows why it
   *  fired — "their reply is overdue", "the meeting needs an agenda" — and that
   *  reason shaped nothing until it reached the steer field. */
  intent?: string;
  /** An AI verb ("Draft reply") opened the composer, so it drafts on open
   *  rather than offering that verb again. Mail only. */
  draftOnOpen?: boolean;
  /** A scheduled message written again. Read when the composer mounts, so the
   *  caller keys the modal on the message. It keeps no saved draft: the
   *  words are the scheduled message's, not a draft's. */
  edit?: EditSeed;
  open: boolean;
  onClose: () => void;
  // A message left this composer — sent now or scheduled. Called before the
  // close, and only then: a caller whose own reading of the record depends on
  // what was sent re-asks on this, and not on a cancel that changed nothing.
  onSent?: () => void;
  /** The control the reader pressed, which gets focus back on close. */
  openedFrom?: HTMLElement;
}>;
