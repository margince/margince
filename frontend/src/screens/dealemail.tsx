// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's email box: the one place on the deal page a rep writes to the
// buyer from.
//
// It is a MAIL box, not a to-do box. It is always here and it always offers to
// write — what changes is whether the mail continues a thread or starts one.
// The buyer wrote and nobody answered, so the box offers the reply anchored to
// their message; the buyer has never written (or their last message is already
// answered), so it offers a fresh mail to the deal's contacts. Those are the
// only two states, and neither is "nothing to do here".
//
// Which one applies is read from the deal status card's `reply_to`, so the box
// and Deal360 agree about whether an answer is owed. Deal360 says WHY writing
// is the move; this box is where the writing happens.

import { Mail } from "lucide-react";
import { useState } from "react";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { ComposeModal } from "./compose";
import { useDealStatusCard } from "./dealstatus";

export function DealEmailAside({
  dealId,
}: Readonly<{ dealId: string; dealName: string }>) {
  const t = useT();
  const [composing, setComposing] = useState(false);
  const status = useDealStatusCard(dealId);
  // A reply target only counts once the read has answered. While it is in
  // flight the box still draws and still opens the composer — it simply starts
  // a fresh mail, which is the safe half: a rep who meant to reply sees the
  // thread missing, where the other way round would silently file a new
  // message onto a thread they never chose.
  const replyTo = status.data?.reply_to ?? undefined;
  return (
    <Panel
      title={t("dealmail.title")}
      sub={replyTo ? t("dealmail.sub.reply") : t("dealmail.sub.fresh")}
    >
      <PanelBody>
        <Button variant="primary" onClick={() => setComposing(true)}>
          <Mail aria-hidden />
          {replyTo ? t("dealmail.reply") : t("dealmail.send")}
        </Button>
      </PanelBody>
      {composing ? (
        // Keyed by the deal AND the thread it answers, so moving to another
        // deal — or the reply target changing under an open box — remounts the
        // composer rather than re-pointing it. Without the key the text written
        // for one thread would be filed against another.
        <ComposeModal
          key={`${dealId}:${replyTo ?? "new"}`}
          activityId={replyTo}
          entityType="deal"
          entityId={dealId}
          kind="email"
          open={composing}
          onClose={() => setComposing(false)}
        />
      ) : null}
    </Panel>
  );
}
