// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useState } from "react";
import { Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { QueryStates } from "../../design-system/querystates";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemStatus } from "../../lib/problem";
import { ComposeModal } from "./compose";
import { useThreadProject } from "./composeanchor";
import {
  type EditSeed,
  editSeedOf,
  isEditable,
  unavailableFilesOf,
} from "./schedulededit";
import { type ScheduledSend, useScheduledSend } from "./scheduledqueue";

type EditRead = ReturnType<typeof useEditRead>;

// The row was read earlier; the message may have gone or changed since.
function changedSinceListed(
  current: ScheduledSend | undefined,
  failed: boolean,
  error: unknown,
): boolean {
  return (
    (current !== undefined && !isEditable(current)) ||
    (failed && problemStatus(error) === 404)
  );
}

// What the message's own read and its thread say about opening the composer.
// Decided only before it opens: a later read must not take the reader's words.
function useEditRead(send: ScheduledSend | null, opened: boolean) {
  const detail = useScheduledSend(send?.id);
  const current = opened ? undefined : detail.data;
  const thread = useThreadProject(send?.anchor_activity_id ?? undefined);
  const answers = Boolean(send?.anchor_activity_id);
  const failed = detail.isLoadingError;
  const stale = changedSinceListed(current, failed, detail.error);
  const unavailable = current ? unavailableFilesOf(current) : 0;
  const seed = current
    ? editSeedOf(current, thread.activity?.links)
    : undefined;
  const waiting =
    detail.isPending || (answers && !thread.settled && !thread.failed);
  const refused = stale || unavailable > 0;
  // A thread that read fine but carries no record to file under cannot open.
  const cannotOpen =
    answers &&
    current !== undefined &&
    !refused &&
    (thread.failed || (thread.settled && !seed));
  const ready = refused || waiting ? undefined : seed;
  return { detail, failed, stale, unavailable, waiting, ready, cannotOpen };
}

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
  const [opened, setOpened] = useState<Readonly<{
    id: string;
    seed: EditSeed;
  }> | null>(null);
  if (!send && opened) {
    setOpened(null);
  }
  const latched = send && opened?.id === send.id ? opened.seed : undefined;
  const read = useEditRead(send, latched !== undefined);
  const { cannotOpen, stale } = read;
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
  const seed = latched ?? read.ready;
  if (!seed) {
    return <EditWaiting subject={send.subject} read={read} onClose={onClose} />;
  }
  if (!latched) {
    setOpened({ id: send.id, seed });
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

// What stands in the composer's place until it can open, or instead of it.
function EditWaiting({
  subject,
  read,
  onClose,
}: Readonly<{ subject: string; read: EditRead; onClose: () => void }>) {
  const t = useT();
  const { detail } = read;
  if (read.stale) {
    return (
      <EditBlocked subject={subject} onClose={onClose}>
        <Callout tone="info" kind="standing" title={t("sched.editStale.title")}>
          <p>{t("sched.editStale.body")}</p>
        </Callout>
      </EditBlocked>
    );
  }
  if (read.waiting || read.failed) {
    return (
      <EditBlocked subject={subject} onClose={onClose}>
        <QueryStates
          query={{
            isPending: !read.failed,
            isError: read.failed,
            error: detail.error,
            refetch: detail.refetch,
          }}
          pendingLabel={t("sched.editLoading")}
          pendingLines={2}
        >
          {null}
        </QueryStates>
      </EditBlocked>
    );
  }
  if (read.unavailable > 0) {
    return (
      <EditBlocked subject={subject} onClose={onClose}>
        <FilesRefusal count={read.unavailable} />
      </EditBlocked>
    );
  }
  return null;
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
