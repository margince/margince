// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useRef } from "react";
import { useReplyRecipient } from "./composereplyrecipient";

// Where the recipient offer stands, parked with a conversation on a switch.
export type OfferSnapshot = Readonly<{
  offeredAddress: string | undefined;
  offered: boolean;
}>;

// The field with the offered address in it. A drafted or committed recipient
// outranks the offer, unless the field holds values beside a vacated slot.
function withOffer(
  current: string[],
  address: string,
  fillsVacancy: boolean,
): string[] {
  if (current.length > 0 && !fillsVacancy) {
    return current;
  }
  return current.includes(address) ? current : [...current, address];
}

// The address the composer offers into To: the thread's counterparty, or the
// record's own address on a first message. It is offered once per
// conversation, and a reader's typing or removal outranks it.
export function useRecipientOffer({
  open,
  isChannelReply,
  answering,
  recordAddress,
  setTo,
}: Readonly<{
  open: boolean;
  isChannelReply: boolean;
  answering: string | undefined;
  recordAddress: string | undefined;
  setTo: (update: (current: string[]) => string[]) => void;
}>) {
  // A channel reply resolves its recipient on the server and asks nothing.
  const asksAddress = open && !isChannelReply;
  const { address: replyRecipient, mailboxes } = useReplyRecipient(
    asksAddress ? answering : undefined,
  );
  // The thread wins over the record: the address on the answered message is
  // who that conversation is with.
  const threadRecipient =
    replyRecipient ?? (asksAddress && !answering ? recordAddress : undefined);
  // Keyed on the recipient rather than an empty field. An empty `to` can hide
  // uncommitted typing, or a prefill the reader just deleted.
  //
  // A ref, because re-rendering on the decision would be the repeat it avoids.
  const offered = useRef(false);
  // Restoring a saved target must not re-offer an address the reader removed.
  const restoringRecipient = useRef(false);
  // Which address was offered, so a change of conversation replaces it alone.
  const offeredAddress = useRef<string | undefined>(undefined);
  // A change of conversation emptied the offer's slot, so the next anchor's
  // address fills it even beside recipients the reader typed.
  const vacated = useRef(false);
  // The reader has taken the field over. The first keystroke is the signal,
  // because committed values stay empty until Enter.
  const settle = useCallback(() => {
    offered.current = true;
  }, []);
  // The previous offer leaves on the change of conversation itself, so a
  // slow or empty lookup cannot leave the old counterparty as the recipient.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the anchor alone. A change of conversation re-arms the offer, and an address resolving does not.
  useEffect(() => {
    if (restoringRecipient.current) {
      restoringRecipient.current = false;
      return;
    }
    offered.current = false;
    const previous = offeredAddress.current;
    if (previous === undefined) {
      return;
    }
    offeredAddress.current = undefined;
    vacated.current = true;
    setTo((current) => current.filter((address) => address !== previous));
  }, [answering]);
  // biome-ignore lint/correctness/useExhaustiveDependencies: the anchor is a key here. Two conversations with one counterparty share an address, and the vacated slot still needs filling.
  useEffect(() => {
    if (threadRecipient === undefined || offered.current) {
      return;
    }
    offered.current = true;
    offeredAddress.current = threadRecipient;
    // The refs settle before the updater, which StrictMode may run twice.
    const fillsVacancy = vacated.current;
    vacated.current = false;
    setTo((current) => withOffer(current, threadRecipient, fillsVacancy));
  }, [threadRecipient, answering]);

  return {
    mailboxes,
    // Read at render, as the conversation switch parks it.
    standing: {
      offeredAddress: offeredAddress.current,
      offered: offered.current,
    } satisfies OfferSnapshot,
    settle,
    // The offer is per conversation, and a new transport is a new one.
    rearm: () => {
      offered.current = false;
    },
    load: (next: OfferSnapshot, restoring: boolean) => {
      offeredAddress.current = next.offeredAddress;
      offered.current = next.offered;
      restoringRecipient.current = restoring;
      vacated.current = false;
    },
  };
}

export type RecipientOffer = ReturnType<typeof useRecipientOffer>;
