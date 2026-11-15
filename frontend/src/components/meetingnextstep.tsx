// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The step after marking a meeting held: what happens next. Otherwise next
// steps come only from reading a transcript. A meeting held without one would
// leave its Next steps empty, and nothing would ask for one.

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DateInput, type ISODate } from "../design-system/dateinput";
import { useT } from "../i18n";
import { taskDueFields } from "../lib/activitybody";
import { activityFollowUpsKey } from "../lib/activitykeys";
import { problemMessageOf, unwrap } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";

/**
 * MeetingNextStep asks for the next step of a meeting just marked held. The
 * task it files names the meeting as its source, which is what the meeting's
 * Next steps list. Skip files nothing.
 */
export function MeetingNextStep({
  meetingId,
  invalidateKeys,
  onClose,
}: Readonly<{
  meetingId: string;
  invalidateKeys: readonly QueryKey[];
  onClose: () => void;
}>) {
  const t = useT();
  const readerZone = useViewerZone();
  const queryClient = useQueryClient();
  const [subject, setSubject] = useState("");
  const [due, setDue] = useState<ISODate | "">("");
  const file = useMutation({
    mutationFn: async (input: { subject: string; due: ISODate | "" }) =>
      unwrap(
        await api.POST("/activities", {
          body: {
            kind: "task",
            source: "manual",
            subject: input.subject,
            source_activity_id: meetingId,
            ...(input.due === ""
              ? {}
              : taskDueFields(input.due, "", readerZone)),
          },
        }),
        t,
      ),
    onSuccess: async () => {
      await Promise.all(
        [activityFollowUpsKey(meetingId), ...invalidateKeys].map((queryKey) =>
          queryClient.invalidateQueries({ queryKey }),
        ),
      );
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("meeting.nextStep.title")}
      intent="form"
      confirmLabel={t("meeting.nextStep.add")}
      cancelLabel={t("meeting.nextStep.skip")}
      confirmDisabled={subject.trim() === ""}
      pending={file.isPending}
      error={file.isError ? problemMessageOf(file.error, t) : undefined}
      onConfirm={() => file.mutate({ subject: subject.trim(), due })}
    >
      <Field label={t("meeting.nextStep.subject")} required>
        {(control) => (
          <TextInput
            {...control}
            value={subject}
            maxLength={500}
            onChange={(event) => setSubject(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("meeting.nextStep.due")}>
        {(control) => (
          <DateInput
            {...control}
            value={due}
            onChange={(event) => setDue(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
