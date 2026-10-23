// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId } from "react";
import { Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { ComposeModal } from "./compose";
import { useThreadProject } from "./composeanchor";
import { editSeedOf, isEditable, unavailableFilesOf } from "./schedulededit";
import { type ScheduledSend, useScheduledSend } from "./scheduledqueue";

/**
 * The composer over a scheduled message, opened from its own read. The old
 * message is withdrawn only once the new one is accepted.
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
  const queryClient = useQueryClient();
  const detail = useScheduledSend(send?.id);
  const current = detail.data;
  // The row was read earlier; the message may have gone or changed since.
  const stale = current !== undefined && !isEditable(current);
  const thread = useThreadProject(send?.anchor_activity_id ?? undefined);
  const unavailable = current ? unavailableFilesOf(current) : 0;
  const seed = current
    ? editSeedOf(current, thread.activity?.links)
    : undefined;
  const answers = Boolean(send?.anchor_activity_id);
  // A thread that read fine but carries no record to file under cannot open.
  const cannotOpen =
    answers &&
    !stale &&
    unavailable === 0 &&
    (thread.failed || (thread.settled && current !== undefined && !seed));
  useEffect(() => {
    if (cannotOpen) {
      toast.show(t("compose.anchorGone"), { tone: "danger" });
      onClose();
    }
  }, [cannotOpen, toast, t, onClose]);
  useEffect(() => {
    if (stale) {
      void queryClient.invalidateQueries({
        queryKey: ["scheduled-sends"],
        exact: true,
      });
    }
  }, [stale, queryClient]);
  if (!send) {
    return null;
  }
  if (detail.isError) {
    return (
      <EditBlocked subject={send.subject} onClose={onClose}>
        <Callout
          tone="danger"
          kind="outcome"
          title={t("sched.editLoadFailed")}
          actions={
            <Button variant="primary" onClick={() => void detail.refetch()}>
              {t("state.retry")}
            </Button>
          }
        >
          <p>{problemMessageOf(detail.error, t)}</p>
        </Callout>
      </EditBlocked>
    );
  }
  if (stale) {
    return (
      <EditBlocked subject={send.subject} onClose={onClose}>
        <Callout tone="info" kind="standing" title={t("sched.editStale")} />
      </EditBlocked>
    );
  }
  if (unavailable > 0) {
    return (
      <EditBlocked subject={send.subject} onClose={onClose}>
        <FilesRefusal count={unavailable} />
      </EditBlocked>
    );
  }
  if (!seed) {
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

// In place of the composer when the message cannot be opened in it.
function EditBlocked({
  subject,
  onClose,
  children,
}: Readonly<{ subject: string; onClose: () => void; children: ReactNode }>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h3 modal-title">
        {t("sched.editFor", { subject })}
      </Heading>
      {children}
    </Modal>
  );
}

function FilesRefusal({ count }: Readonly<{ count: number }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("sched.editRefused.title")}
    >
      <p>
        {plural("sched.editRefused.files", count, {
          count: formatNumber(count, locale),
        })}
      </p>
    </Callout>
  );
}
