// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { api } from "../api/client";
import { Field, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { FileDropzone } from "../design-system/filedropzone";
import { useToast } from "../design-system/toast";
import { TokenList } from "../design-system/tokeninput";
import { formatBytes } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  ACCEPTED_ATTACHMENT_ATTR,
  uploadAttachment,
} from "../lib/attachmentupload";
import { useCanWrite } from "../lib/capability";
import { ProblemError, problemMessageOf, unwrap } from "../lib/problem";
import { formatUploadLimit, useMaxUploadBytes } from "../lib/uploadlimit";

// The file fields of the Log activity form. A transcript is read into the body.
// Attached files wait in the form until the server has created the activity,
// because an attachment needs a parent id.

/**
 * One file that is not on the activity. `uncertain` marks an upload that got
 * no answer and that a re-read of the activity could not settle either way.
 */
export type UnattachedFile = Readonly<{
  filename: string;
  error: unknown;
  uncertain: boolean;
}>;

/** A file picked in the form. The id tells apart two files with the same name. */
export type PickedFile = Readonly<{ id: string; file: File }>;

let lastPick = 0;
function picked(file: File): PickedFile {
  lastPick += 1;
  return { id: `pick-${lastPick}`, file };
}

// Only a 4xx answer proves nothing was stored. Any other failure may have
// come after the server committed the file.
function refusedByServer(error: unknown): boolean {
  if (!(error instanceof ProblemError)) return false;
  const { problem } = error;
  const status =
    typeof problem === "object" && problem !== null && "status" in problem
      ? problem.status
      : undefined;
  return typeof status === "number" && status >= 400 && status < 500;
}

async function sha256Hex(file: File): Promise<string> {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await file.arrayBuffer(),
  );
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

// The upload endpoint takes no Idempotency-Key, so a retry could store a second
// copy. An unanswered upload is checked against the activity's stored files
// instead, by checksum: the server may rename a file it stores.
async function settleUnanswered(
  activityId: string,
  unanswered: readonly Readonly<{ file: File; error: unknown }>[],
): Promise<UnattachedFile[]> {
  const unsettled = unanswered.map(({ file, error }) => ({
    filename: file.name,
    error,
    uncertain: true,
  }));
  try {
    const stored = unwrap(
      await api.GET("/attachments", {
        params: {
          query: { entity_type: "activity", entity_id: activityId, limit: 200 },
        },
      }),
    );
    // A cut list cannot prove a file is missing.
    if (stored.page.has_more) return unsettled;
    const checksums = new Set(stored.data.map((file) => file.checksum));
    const missing: UnattachedFile[] = [];
    for (const { file, error } of unanswered) {
      if (!checksums.has(await sha256Hex(file))) {
        missing.push({ filename: file.name, error, uncertain: false });
      }
    }
    return missing;
  } catch {
    return unsettled;
  }
}

/**
 * attachLoggedFiles uploads each file against the new activity, one after the
 * other. A refusal of one file never stops the rest: the activity is already
 * logged, so every file that can land should.
 */
export async function attachLoggedFiles(
  activityId: string,
  files: readonly File[],
): Promise<UnattachedFile[]> {
  const refused: UnattachedFile[] = [];
  const unanswered: { file: File; error: unknown }[] = [];
  for (const file of files) {
    try {
      await uploadAttachment(
        { entityType: "activity", entityId: activityId },
        file,
      );
    } catch (error) {
      if (refusedByServer(error)) {
        refused.push({ filename: file.name, error, uncertain: false });
      } else {
        unanswered.push({ file, error });
      }
    }
  }
  if (unanswered.length === 0) return refused;
  return [...refused, ...(await settleUnanswered(activityId, unanswered))];
}

/**
 * useReportUnattached says which files did not land, one sticky toast each,
 * with the server's own reason. The form has closed by then.
 */
export function useReportUnattached() {
  const t = useT();
  const toast = useToast();
  return (unattached: readonly UnattachedFile[]) => {
    for (const file of unattached) {
      toast.show(
        file.uncertain
          ? t("log.fileUncertain", { filename: file.filename })
          : t("log.fileUnattached", {
              filename: file.filename,
              reason: problemMessageOf(file.error, t),
            }),
        { tone: "danger", sticky: true },
      );
    }
  };
}

// Plain text keeps transcript citations aligned with spoken lines, without cue headers.
const ACCEPTED_TRANSCRIPT_EXTENSION = ".txt";

/** TranscriptUploadField reads a transcript file into the body as text. */
export function TranscriptUploadField({
  onText,
}: Readonly<{ onText: (text: string) => void }>) {
  const t = useT();
  const [fileError, setFileError] = useState<string | null>(null);
  return (
    <Field label={t("log.transcriptUpload")} hint={fileError ?? undefined}>
      {(control) => (
        <TextInput
          {...control}
          type="file"
          accept={ACCEPTED_TRANSCRIPT_EXTENSION}
          onChange={async (event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (!file) {
              return;
            }
            if (
              !file.name.toLowerCase().endsWith(ACCEPTED_TRANSCRIPT_EXTENSION)
            ) {
              setFileError(t("log.transcriptUploadRejected"));
              return;
            }
            try {
              const text = await file.text();
              setFileError(null);
              onText(text);
            } catch {
              setFileError(t("log.transcriptUploadFailed"));
            }
          }}
        />
      )}
    </Field>
  );
}

type FilesUpdate = (current: readonly PickedFile[]) => readonly PickedFile[];

type FieldProps = Readonly<{
  files: readonly PickedFile[];
  // An updater, because several files can arrive from one drop.
  onChange: (update: FilesUpdate) => void;
  // While the log is out: a file picked then would miss the request.
  disabled?: boolean;
}>;

/**
 * LogActivityFilesField picks the files to attach. Drawn only for a seat that
 * may change activities, since the upload asks for that grant.
 */
export function LogActivityFilesField(props: FieldProps) {
  const permitted = useCanWrite("activity", "update");
  return permitted ? <FilesPicker {...props} /> : null;
}

function FilesPicker({ files, onChange, disabled }: FieldProps) {
  const t = useT();
  const { locale } = useLocale();
  const maxBytes = useMaxUploadBytes();
  const limitLabel = maxBytes ? formatUploadLimit(maxBytes, locale) : "";
  const [refused, setRefused] = useState<string | null>(null);
  const pick = (file: File) => {
    // Refused here, before the activity exists, with the limit the
    // add-document dialog states.
    if (maxBytes !== undefined && file.size > maxBytes) {
      setRefused(t("docs.add.errTooLarge", { size: limitLabel }));
      return;
    }
    setRefused(null);
    onChange((current) => [...current, picked(file)]);
  };
  return (
    <div className="form-stack">
      <FileDropzone
        label={t("log.files")}
        hint={
          limitLabel ? t("docs.add.fileHint", { size: limitLabel }) : undefined
        }
        emptyLabel={t("log.filesEmpty")}
        multiple
        files={files.map((held) => held.file)}
        onPick={pick}
        accept={ACCEPTED_ATTACHMENT_ATTR}
        disabled={disabled}
      />
      {files.length > 0 && (
        <TokenList
          items={files.map(({ id, file }) => ({
            id,
            label: `${file.name} · ${formatBytes(file.size, locale)}`,
          }))}
          removeLabel={(item) =>
            t("compose.fileRemove", { filename: item.label })
          }
          disabled={disabled}
          onRemove={(id) =>
            onChange((current) => current.filter((held) => held.id !== id))
          }
        />
      )}
      {refused && <ErrorLine>{refused}</ErrorLine>}
    </div>
  );
}
