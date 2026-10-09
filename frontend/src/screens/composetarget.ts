// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import type { components } from "../api/schema";
import type { ChosenFile } from "./composeattachments";
import {
  type ComposeFields,
  emptyFields,
  type FieldsSnapshot,
} from "./composefields";
import type { OfferSnapshot, RecipientOffer } from "./composerecipientoffer";
import type { Transport } from "./contacttransports";

type Activity = components["schemas"]["Activity"];

// Which way the message goes. Each opening starts on the caller's transport,
// never on the last opening's dial; an empty one resolves to the record's lead.
export function useTransportDial({
  open,
  transports,
  initialTransportId,
  kind,
}: Readonly<{
  open: boolean;
  transports: readonly Transport[];
  initialTransportId?: string;
  kind?: Activity["kind"];
}>) {
  const [dial, setDial] = useState({ open, id: initialTransportId ?? "" });
  const transportId = open && !dial.open ? (initialTransportId ?? "") : dial.id;
  if (dial.open !== open) setDial({ open, id: transportId });
  const transport =
    transports.find((option) => option.id === transportId) ??
    transports.find((option) => option.id === initialTransportId) ??
    transports[0];
  // A channel the reader picked. Mail alone can open a conversation, so any
  // other transport is anchored by nature.
  const channel = transport && transport.id !== "email" ? transport : undefined;
  // The endpoint follows the kind: every channel message goes to send-message,
  // and the server resolves the recipient from the conversation.
  const isChannelReply = kind === "message" || channel !== undefined;
  // The drawer's shape follows the transport this opening began on.
  const openedOn = useRef(transport);
  if (!open || !dial.open || !openedOn.current) openedOn.current = transport;
  const asDrawer =
    kind !== "message" && (openedOn.current?.id ?? "email") === "email";
  return {
    transport,
    channel,
    isChannelReply,
    asDrawer,
    turn: (next: string) => setDial({ open, id: next }),
  };
}

// The message being answered. Undefined follows the opener, and null starts
// a new message. A channel always answers its own conversation.
export function useAnswerTarget({
  open,
  channelAnchorId,
  activityId,
}: Readonly<{
  open: boolean;
  channelAnchorId: string | undefined;
  activityId: string | undefined;
}>) {
  const [chosen, setChosen] = useState<string | null | undefined>(undefined);
  const answering =
    channelAnchorId ??
    (chosen === undefined ? activityId : (chosen ?? undefined));
  useEffect(() => {
    if (!open) setChosen(undefined);
  }, [open]);
  return { answering, choose: setChosen };
}

// The files attached per target. An upload may finish after a switch, and its
// callback keeps the bank of the target it started on.
export function useTargetFiles(fileTarget: string) {
  const [filesByTarget, setFilesByTarget] = useState<
    Record<string, readonly ChosenFile[]>
  >({});
  const files = filesByTarget[fileTarget] ?? [];
  const setFiles = (
    update: (current: readonly ChosenFile[]) => readonly ChosenFile[],
  ) => {
    setFilesByTarget((current) => ({
      ...current,
      [fileTarget]: update(current[fileTarget] ?? []),
    }));
  };
  return { files, setFiles };
}

type Parked = Readonly<{ fields: FieldsSnapshot; offer: OfferSnapshot }>;

// Moving to another conversation parks the whole editable offer and brings
// back the one parked there, so no field becomes part of a different reply.
export function useConversationSwitch({
  answering,
  choose,
  fields,
  offer,
  busy,
  resetRequests,
}: Readonly<{
  answering: string | undefined;
  choose: (id: string | null) => void;
  fields: ComposeFields;
  offer: RecipientOffer;
  /** A send or a rejection in flight holds the reader on this conversation. */
  busy: boolean;
  /** Clears the draft and send outcomes, which belong to the last target. */
  resetRequests: () => void;
}>) {
  const parked = useRef(new Map<string, Parked>());
  const standing: Parked = { fields: fields.snapshot(), offer: offer.standing };
  return (id: string | null) => {
    if (id === (answering ?? null) || busy) return;
    parked.current.set(answering ?? "", standing);
    const saved = parked.current.get(id ?? "");
    const next = saved ?? {
      fields: emptyFields(),
      offer: { offeredAddress: undefined, offered: false },
    };
    fields.draftEpoch.current += 1;
    resetRequests();
    choose(id);
    fields.load(next.fields);
    offer.load(next.offer, Boolean(saved));
  };
}
