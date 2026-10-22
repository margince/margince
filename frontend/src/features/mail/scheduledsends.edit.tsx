// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { ComposeModal } from "./compose";
import { useThreadProject } from "./composeanchor";
import { editSeedOf } from "./schedulededit";
import type { ScheduledSend } from "./scheduledqueue";

/**
 * The composer over a scheduled message, to write it again.
 *
 * The approval is bound to the content, so an edit is a new send that passes
 * every gate from the top. The old message is withdrawn only once the new one
 * is accepted, so a refusal leaves the original waiting. Keyed on the message,
 * because the composer reads its seed once, when it mounts.
 *
 * A reply opens once its thread's email has been read, since the record it is
 * filed under is that email's.
 */
export function EditComposer({
  send,
  onClose,
  onReplaced,
}: Readonly<{
  send: ScheduledSend | null;
  onClose: () => void;
  onReplaced: (id: string) => void;
}>) {
  const t = useT();
  const toast = useToast();
  const thread = useThreadProject(send?.anchor_activity_id ?? undefined);
  const seed = send ? editSeedOf(send, thread.activity?.links) : undefined;
  const unreadable = Boolean(send?.anchor_activity_id) && thread.failed;
  useEffect(() => {
    if (unreadable) {
      toast.show(t("compose.anchorGone"), { tone: "danger" });
      onClose();
    }
  }, [unreadable, toast, t, onClose]);
  if (!send || !seed) {
    return null;
  }
  return (
    <ComposeModal
      key={send.id}
      open
      edit={seed}
      activityId={seed.replyTo || undefined}
      entityType={seed.anchor.type}
      entityId={seed.anchor.id}
      onClose={onClose}
      onSent={() => onReplaced(send.id)}
    />
  );
}
