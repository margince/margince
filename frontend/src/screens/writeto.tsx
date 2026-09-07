// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext, useState } from "react";
import { ComposeModal, type RelinkKind } from "./compose";

/**
 * Writing to an address, from wherever the address is shown.
 *
 * An address on a record header, in a rail's details row, on a card listing
 * the account's people: each is a way to ACT, and the act is the product's
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
export type WriteToTarget = WriteToRecord & Readonly<{ address?: string }>;

export type WriteTo = (target: WriteToTarget) => void;

const WriteToContext = createContext<WriteTo | null>(null);

/**
 * The way to write to an address from here, or null where nothing hosts a
 * composer. The authenticated shell always does; null is a story or a test
 * that mounted a surface on its own, and a surface answers it by keeping the
 * address as text rather than by promising a press that opens nothing.
 */
export function useWriteTo(): WriteTo | null {
  return useContext(WriteToContext);
}

/** A page's own answer to the call, for one that keeps its own composer. */
export function WriteToProvider({
  writeTo,
  children,
}: Readonly<{ writeTo: WriteTo; children: ReactNode }>) {
  return (
    <WriteToContext.Provider value={writeTo}>
      {children}
    </WriteToContext.Provider>
  );
}

/**
 * The shell's composer for an address pressed anywhere under it: an
 * account-started mail to the record the address belongs to, in the drawer
 * every other mail leaves from.
 */
export function WriteToHost({ children }: Readonly<{ children: ReactNode }>) {
  const [target, setTarget] = useState<WriteToTarget | null>(null);
  return (
    <WriteToContext.Provider value={setTarget}>
      {children}
      {target && (
        // Keyed by the record, so a press on another record's address while
        // the composer is open remounts it rather than re-pointing it — the
        // text written for one record must not be filed against another.
        <ComposeModal
          key={target.entityId}
          entityType={target.entityType}
          entityId={target.entityId}
          // A person IS the contact the composer files under; every other
          // record resolves its people from its own links.
          personId={
            target.entityType === "person" ? target.entityId : undefined
          }
          recordAddress={target.address}
          kind="email"
          open
          onClose={() => setTarget(null)}
        />
      )}
    </WriteToContext.Provider>
  );
}
