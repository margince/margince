// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ComposeModal } from "./compose";
import { editSeedOf } from "./schedulededit";
import type { ScheduledSend } from "./scheduledqueue";

/**
 * The composer over a scheduled message, to write it again.
 *
 * The approval is bound to the content, so an edit is a new send that passes
 * every gate from the top. The old message is withdrawn only once the new one
 * is accepted, so a refusal leaves the original waiting. Keyed on the message,
 * because the composer reads its seed once, when it mounts.
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
  const seed = send ? editSeedOf(send) : undefined;
  if (!send || !seed) {
    return null;
  }
  return (
    <ComposeModal
      key={send.id}
      open
      edit={seed}
      entityType={seed.anchor.type}
      entityId={seed.anchor.id}
      onClose={onClose}
      onSent={() => onReplaced(send.id)}
    />
  );
}
