// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import type { RelinkKind } from "./recordtargets";

/**
 * Writing to an address, from wherever the address is shown.
 *
 * An address on a record header, in a rail's details row, on a card listing
 * the account's contacts: each is a way to ACT, and the act is the product's
 * composer — the one place a message is drafted, gated on consent, sent and
 * filed against the record it was written from. Handing the address to the
 * reader's own mail client instead (a `mailto:`) sent the message AROUND the
 * product: nothing was filed, no consent was asked, and the thread the record
 * shows had a hole where the reply arrived.
 *
 * ONE host, mounted once in the authenticated shell, so a surface that shows an
 * address needs to know nothing about composers: it names the record the
 * address belongs to and calls. A page that already keeps a composer of its
 * own — the contact page, whose drawer also offers chat transports — answers
 * the same call with that one through `WriteToProvider`, so its address and
 * its header verb open the same drawer rather than two.
 * The host is `WriteToHost` in `features/settings/writetohost.tsx`.
 *
 * Only while the reader has a mailbox the product can write from. The composer
 * sends from the reader's own connected mailbox and can only refuse without
 * one, so an address is then handed to the reader's own mail client instead:
 * the message still leaves outside the product, but that is the truth of the
 * situation rather than a drawer that opens onto a refusal.
 */

/** The record an address belongs to, which a message to it is filed under. */
export type WriteToRecord = Readonly<{
  entityType: RelinkKind;
  entityId: string;
}>;

/**
 * What a caller asks to write: the record, and the address the message goes
 * to when the caller is holding one — a first message to a record nobody has
 * written to yet has no thread to resolve an addressee from, so the address
 * the reader pressed is the one the composer offers.
 */
export type WriteToTarget = WriteToRecord &
  Readonly<{
    address?: string;
    // Set by an AI verb: the composer drafts on arrival, steered by these
    // words. An empty string still drafts; only an absent one opens empty.
    draftIntent?: string;
  }>;

export type WriteTo = (target: WriteToTarget) => void;

const WriteToContext = createContext<WriteTo | null>(null);

/**
 * The way to write to an address from the product, or null when there is none
 * from here: the reader has no connected mailbox, or nothing hosts a composer
 * (a story or a test that mounted a surface on its own). A surface answers
 * null with the reader's own mail client, never with a press that opens
 * nothing.
 */
export function useWriteTo(): WriteTo | null {
  return useContext(WriteToContext);
}

/**
 * A page's own answer to the call, for one that keeps its own composer — or
 * null, the same "nothing writes from here" the host says. Always a provider
 * and never the bare children: swapping between the two would remount the
 * page under it the moment the mailbox roster answers.
 */
export function WriteToProvider({
  writeTo,
  children,
}: Readonly<{ writeTo: WriteTo | null; children: ReactNode }>) {
  return (
    <WriteToContext.Provider value={writeTo}>
      {children}
    </WriteToContext.Provider>
  );
}
