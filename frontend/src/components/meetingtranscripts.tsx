// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Modal } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { NoteMarkdown } from "../design-system/markdown";
import { useToast } from "../design-system/toast";
import { TranscriptReadCard } from "../design-system/transcriptread";
import { useT } from "../i18n";
import { useActivity } from "../lib/activityread";
import { useCanWrite, useCanWriteRecord } from "../lib/capability";
import { meetingNotesText } from "../lib/meetingrecord";
import { unwrap } from "../lib/problem";
import { ActivityBodyField } from "./activitybodyfield";
import { TranscriptUploadField, useReportUnattached } from "./logactivityfiles";

import {
  type MeetingTranscriptDraft,
  useMeetingTranscriptDraft,
} from "./meetingtranscripts.draft";

type Activity = components["schemas"]["Activity"];
type Props = Readonly<{
  activity: Activity;
  invalidateKeys: readonly QueryKey[];
}>;

/** Opens the existing meeting's notes and transcripts from either record list. */
export function MeetingTranscripts({ activity, invalidateKeys }: Props) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const draft = useMeetingTranscriptDraft();
  if (
    activity.kind !== "meeting" ||
    activity.source_system === "transcript" ||
    activity.content_state === "withheld"
  )
    return null;
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("meeting.transcripts")}</Button>
      {open && (
        <MeetingTranscriptDialog
          id={activity.id}
          draft={draft}
          invalidateKeys={invalidateKeys}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

function MeetingTranscriptDialog({
  id,
  draft,
  invalidateKeys,
  onClose,
}: Readonly<{
  id: string;
  draft: MeetingTranscriptDraft;
  invalidateKeys: readonly QueryKey[];
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const meeting = useActivity(id);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t("meeting.transcripts")}
      </Heading>
      <ErrorLine error={meeting.error} />
      {meeting.data && meeting.data.content_state !== "withheld" && (
        <MeetingTranscriptContents
          activity={meeting.data}
          draft={draft}
          invalidateKeys={invalidateKeys}
        />
      )}
      <div className="actions">
        <Button onClick={onClose}>{t("common.close")}</Button>
      </div>
    </Modal>
  );
}

function MeetingTranscriptContents({
  activity,
  invalidateKeys,
  draft,
}: Props & Readonly<{ draft: MeetingTranscriptDraft }>) {
  const t = useT();
  const client = useQueryClient();
  const toast = useToast();
  const writable = useCanWriteRecord("activity", activity);
  const canCreate = useCanWrite("activity", "create");
  const {
    body,
    setBody,
    file,
    setFile,
    stored,
    readingFile,
    setReadingFile,
    key,
    saving,
  } = draft;
  const report = useReportUnattached();
  const queryKey = ["meeting-transcripts", activity.id];
  const transcripts = useInfiniteQuery({
    queryKey,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              source_activity_id: activity.id,
              kind: "meeting",
              limit: 50,
              cursor: pageParam,
            },
          },
        }),
        t,
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
    gcTime: 0,
  });
  const save = useMutation({
    mutationFn: draft.save,
    onSuccess: (failures) => {
      client.invalidateQueries({ queryKey });
      client.invalidateQueries({ queryKey: ["activity", activity.id] });
      client.invalidateQueries({ queryKey: ["attachments"] });
      for (const queryKey of invalidateKeys)
        client.invalidateQueries({ queryKey });
      if (failures.length) report(failures);
      else toast.show(t("tasks.logged"));
    },
  });
  const notes = meetingNotesText(activity);
  const canAdd =
    writable &&
    canCreate &&
    !activity.archived_at &&
    (activity.meeting_status === "held" || activity.awaiting_outcome);
  return (
    <div className="form-stack">
      <Heading size="small">
        {activity.subject ?? t("meeting.transcripts")}
      </Heading>
      <p className="t-caption">{t("meeting.internalNotesHelp")}</p>
      <p>
        {t("history.field.meeting_status")}:{" "}
        {activity.meeting_status
          ? t(`history.value.meeting_status.${activity.meeting_status}`)
          : t("worklist.verb.meetingHowDidItGo")}
      </p>
      {notes && <NoteMarkdown source={notes} />}
      <ErrorLine error={transcripts.error} />
      {transcripts.data?.pages
        .flatMap((page) => page.data)
        .filter((row) => row.source_system === "transcript")
        .map((row) => (
          <section key={row.id}>
            <Heading size="small">
              {row.subject ?? t("meeting.transcripts")}
            </Heading>
            <TranscriptReadCard activityId={row.id} />
          </section>
        ))}
      {transcripts.hasNextPage && (
        <Button
          pending={transcripts.isFetchingNextPage}
          onClick={() => void transcripts.fetchNextPage()}
        >
          {t("list.loadMore")}
        </Button>
      )}
      {!writable && <p className="t-caption">{t("meeting.readOnly")}</p>}
      {canAdd && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (saving) return;
            save.mutate({ meeting: activity, body, file, key });
          }}
        >
          <div className="form-stack">
            <ActivityBodyField
              label={t("meeting.transcriptText")}
              value={body}
              onChange={(text) => {
                setBody(text);
                setFile(undefined);
              }}
              transcript
              disabled={saving || stored || readingFile}
              rows={8}
            />
            <TranscriptUploadField
              onText={setBody}
              onFile={setFile}
              disabled={saving || stored || readingFile}
              onReadingChange={setReadingFile}
            />
          </div>
          {file && <p className="t-caption">{file.name}</p>}
          {stored && <p role="status">{t("meeting.transcriptFileRetry")}</p>}
          <ErrorLine error={save.error} />
          <Button
            type="submit"
            pending={saving}
            disabled={!body.trim() || readingFile}
          >
            {t("log.save")}
          </Button>
        </form>
      )}
    </div>
  );
}
