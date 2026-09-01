// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record's email box: the one place on a person, lead or deal page a rep
// writes from.
//
// It is a MAIL box, not a to-do box. It is always here and it always offers to
// write: what changes is whether the mail continues a thread or starts one.
// A caller that knows a thread is owed passes `replyTo`, so the box offers the
// reply anchored to that message; without one it offers a fresh mail to the
// record's own contacts. Those are the only two states, and neither is
// "nothing to do here".
//
// DealEmailAside (dealemail.tsx) is the caller that reads `reply_to` off the
// deal status card; a person or lead page has no such read today, so it turns
// on `detectWaitingReply` instead and lets this component ask the same
// question directly. Whoever decides which state applies is the caller's
// job: this component only draws it, from whichever of the two sources the
// caller chose.

import { useQuery } from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { ComposeModal, type RelinkKind } from "./compose";

/**
 * Whether this record has an inbound message still awaiting an answer, and
 * which activity it is — the same read DealEmailAside already gets for free
 * off the deal status card, offered here for a caller with no status card of
 * its own.
 *
 * `enabled` is the caller's opt-in, not a default: DealEmailAside supplies
 * its own `replyTo` and must never also run this query, or the box would
 * have two sources answering one question. A person or lead page turns it on
 * because it has no other read of the same fact.
 *
 * Undefined while the read is unsettled or finds nothing: fresh mail, the
 * safe half. A rep who meant to reply sees the thread missing, where the
 * other way round would silently file a new message onto a thread they
 * never chose — the same reasoning DealEmailAside's mid-flight state uses.
 */
function useWaitingReply(
  entityType: RelinkKind,
  entityId: string,
  enabled: boolean,
): string | undefined {
  const query = useQuery({
    queryKey: ["record-waiting-reply", entityType, entityId],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: {
          query: {
            entity_type: entityType,
            entity_id: entityId,
            waiting_reply: true,
            limit: 1,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled,
  });
  return query.data?.data?.[0]?.id;
}

export function RecordEmailAside({
  entityType,
  entityId,
  replyTo,
  detectWaitingReply = false,
  personId,
  strings,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  replyTo?: string;
  // Turns on this component's own waiting-reply read (see useWaitingReply
  // above) for a caller with no status card of its own to read it from. A
  // caller that already knows its reply target (dealemail.tsx) leaves this
  // off and passes `replyTo` directly instead.
  detectWaitingReply?: boolean;
  personId?: string;
  // Overrides for a caller that already has its own wording for these five
  // spots (dealemail.tsx keeps its `dealmail.*` copy this way). Absent falls
  // back to the generic `recordmail.*` catalog entries.
  strings?: Readonly<{
    title: MessageKey;
    subReply: MessageKey;
    subFresh: MessageKey;
    reply: MessageKey;
    send: MessageKey;
  }>;
}>) {
  const t = useT();
  const [composing, setComposing] = useState(false);
  const waitingReply = useWaitingReply(
    entityType,
    entityId,
    detectWaitingReply,
  );
  const effectiveReplyTo = replyTo ?? waitingReply;
  const title = strings?.title ?? "recordmail.title";
  const subReply = strings?.subReply ?? "recordmail.sub.reply";
  const subFresh = strings?.subFresh ?? "recordmail.sub.fresh";
  const replyLabel = strings?.reply ?? "recordmail.reply";
  const sendLabel = strings?.send ?? "recordmail.send";
  return (
    <Panel title={t(title)} sub={effectiveReplyTo ? t(subReply) : t(subFresh)}>
      <PanelBody>
        <Button variant="primary" onClick={() => setComposing(true)}>
          <Mail aria-hidden />
          {effectiveReplyTo ? t(replyLabel) : t(sendLabel)}
        </Button>
      </PanelBody>
      {composing ? (
        // Keyed by the record AND the thread it answers, so moving to another
        // record, or the reply target changing under an open box, remounts
        // the composer rather than re-pointing it. Without the key the text
        // written for one thread would be filed against another.
        <ComposeModal
          key={`${entityId}:${effectiveReplyTo ?? "new"}`}
          activityId={effectiveReplyTo}
          entityType={entityType}
          entityId={entityId}
          personId={personId}
          kind="email"
          open={composing}
          onClose={() => setComposing(false)}
        />
      ) : null}
    </Panel>
  );
}
