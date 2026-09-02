// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's email box: the one place on the deal page a rep writes to the
// buyer from. It is RecordEmailAside (recordemail.tsx) with its wording and
// its reply target, both particular to a deal.
//
// Which state applies is read from the deal status card's `reply_to`, so the
// box and Deal360 agree about whether an answer is owed. Deal360 says WHY
// writing is the move; this box is where the writing happens.

import { useDealStatusCard } from "./dealstatus";
import { RecordEmailAside } from "./recordemail";

export function DealEmailAside({ dealId }: Readonly<{ dealId: string }>) {
  const status = useDealStatusCard(dealId);
  // A reply target only counts once the read has answered. While it is in
  // flight the box still draws and still opens the composer — it simply starts
  // a fresh mail, which is the safe half: a rep who meant to reply sees the
  // thread missing, where the other way round would silently file a new
  // message onto a thread they never chose.
  const replyTo = status.data?.reply_to ?? undefined;
  return (
    <RecordEmailAside
      entityType="deal"
      entityId={dealId}
      replyTo={replyTo}
      strings={{
        title: "dealmail.title",
        subReply: "dealmail.sub.reply",
        subFresh: "dealmail.sub.fresh",
        reply: "dealmail.reply",
        send: "dealmail.send",
      }}
    />
  );
}
