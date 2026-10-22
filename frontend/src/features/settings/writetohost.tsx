// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useCallback, useState } from "react";
import { isMailbox } from "../../format/connectorprovider";
import { useConnectors } from "../../lib/connectors";
import { WriteToProvider, type WriteToTarget } from "../../lib/writeto";
import { ComposeModal } from "../mail/compose";

/**
 * Whether this reader has a mailbox the product can send from: a connected
 * mail connection, not a calendar. Unanswered reads as none, because a drawer
 * offered on a guess would open onto a refusal.
 */
export function useMailboxConnected(): boolean {
  const connectors = useConnectors();
  return (connectors.data?.data ?? []).some(
    (connection) =>
      connection.status === "connected" && isMailbox(connection.provider),
  );
}

/**
 * The shell's composer for an address pressed anywhere under it. It opens the
 * mail drawer while the reader has a mailbox to send from, and offers nothing
 * otherwise.
 */
export function WriteToHost({ children }: Readonly<{ children: ReactNode }>) {
  // The target outlives the close, so the composer can animate out. `seq`
  // counts the opens and rides in the key, so every press mounts a fresh one.
  const [target, setTarget] = useState<
    (WriteToTarget & Readonly<{ seq: number }>) | null
  >(null);
  const [writing, setWriting] = useState(false);
  const connected = useMailboxConnected();
  const writeTo = useCallback((next: WriteToTarget) => {
    setTarget((prior) => ({ ...next, seq: (prior?.seq ?? 0) + 1 }));
    setWriting(true);
  }, []);
  return (
    <WriteToProvider writeTo={connected ? writeTo : null}>
      {children}
      {target !== null && (
        // Keyed by the record and the open: text written for one record must
        // never be filed against another.
        <ComposeModal
          key={`${target.entityId}:${target.seq}`}
          entityType={target.entityType}
          entityId={target.entityId}
          // A contact files under itself; other records resolve their contacts.
          contactId={
            target.entityType === "contact" ? target.entityId : undefined
          }
          recordAddress={target.address}
          kind="email"
          open={writing}
          onClose={() => setWriting(false)}
        />
      )}
    </WriteToProvider>
  );
}
