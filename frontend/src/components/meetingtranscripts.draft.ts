// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import { api } from "../api/client";
import { useIdempotencyKey } from "../api/idempotencykey";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { useCreateOncePerKey } from "./logactivity.retry";
import { attachLoggedFiles, type UnattachedFile } from "./logactivityfiles";

type SaveInput = Readonly<{
  meeting: components["schemas"]["Activity"];
  body: string;
  file: File | undefined;
  key: string;
}>;

/** The draft belongs to the meeting row, so closing its dialog keeps a file retry. */
export function useMeetingTranscriptDraft() {
  const t = useT();
  const [body, setBody] = useState("");
  const [file, setFile] = useState<File>();
  const [stored, setStored] = useState(false);
  const [readingFile, setReadingFile] = useState(false);
  const [saving, setSaving] = useState(false);
  const { key, renew } = useIdempotencyKey();
  const createOnce = useCreateOncePerKey();
  const completed = useRef(new Set<File>());
  const inflight = useRef<Promise<UnattachedFile[]> | null>(null);
  const save = (input: SaveInput): Promise<UnattachedFile[]> => {
    if (inflight.current) return inflight.current;
    setSaving(true);
    const work = async () => {
      await createOnce(
        input.key,
        {
          kind: "meeting",
          source: "manual",
          source_system: "transcript",
          meeting_status: "held",
          records_meeting_id: input.meeting.id,
          subject: input.meeting.subject ?? undefined,
          body: input.body,
        },
        async (request) =>
          unwrap(
            await api.POST("/activities", {
              params: { header: { "Idempotency-Key": input.key } },
              body: request,
            }),
            t,
          ),
      );
      const failures = input.file
        ? await attachLoggedFiles(
            input.meeting.id,
            [input.file],
            completed.current,
          )
        : [];
      setStored(failures.length > 0);
      if (failures.length === 0) {
        setBody("");
        setFile(undefined);
        completed.current.clear();
        renew();
      }
      return failures;
    };
    const pending = work().finally(() => {
      inflight.current = null;
      setSaving(false);
    });
    inflight.current = pending;
    return pending;
  };
  return {
    body,
    setBody,
    file,
    setFile,
    stored,
    readingFile,
    setReadingFile,
    saving,
    key,
    save,
  };
}

export type MeetingTranscriptDraft = ReturnType<
  typeof useMeetingTranscriptDraft
>;
