// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { FileDropzone } from "../../design-system/filedropzone";
import { useToast } from "../../design-system/toast";
import { TokenList } from "../../design-system/tokeninput";
import { formatBytes } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  ACCEPTED_ATTACHMENT_ATTR,
  uploadAttachment,
} from "../../lib/attachmentupload";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import { formatUploadLimit, useMaxUploadBytes } from "../../lib/uploadlimit";

// The file fields of the Log activity form. A transcript is read into the body.
// Attached files wait in the form until the server has created the activity,
// because an attachment needs a parent id.

/** One file the server did not take, and the error it answered with. */
export type UnattachedFile = Readonly<{ filename: string; error: unknown }>;

/**
 * attachLoggedFiles uploads each file against the new activity, one after the
 * other. A refusal of one file never stops the rest: the activity is already
 * logged, so every file that can land should.
 */
export async function attachLoggedFiles(
  activityId: string,
  files: readonly File[],
): Promise<UnattachedFile[]> {
  const unattached: UnattachedFile[] = [];
  for (const file of files) {
    try {
      await uploadAttachment(
        { entityType: "activity", entityId: activityId },
        file,
      );
    } catch (error) {
      unattached.push({ filename: file.name, error });
    }
  }
  return unattached;
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
        t("log.fileUnattached", {
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

// Two picks of the same file are one file.
function fileKey(file: File): string {
  return `${file.name}:${file.size}:${file.lastModified}`;
}

type FilesUpdate = (current: readonly File[]) => readonly File[];

/**
 * LogActivityFilesField picks the files to attach. Drawn only for a seat that
 * may change activities, since the upload asks for that grant.
 */
export function LogActivityFilesField(
  props: Readonly<{
    files: readonly File[];
    // An updater, because several files can arrive from one drop.
    onChange: (update: FilesUpdate) => void;
    disabled?: boolean;
  }>,
) {
  const permitted = useCanWrite("activity", "update");
  return permitted ? <FilesPicker {...props} /> : null;
}

function FilesPicker({
  files,
  onChange,
  disabled,
}: Readonly<{
  files: readonly File[];
  onChange: (update: FilesUpdate) => void;
  disabled?: boolean;
}>) {
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
    onChange((current) =>
      current.some((held) => fileKey(held) === fileKey(file))
        ? current
        : [...current, file],
    );
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
        files={files}
        onPick={pick}
        accept={ACCEPTED_ATTACHMENT_ATTR}
      />
      {files.length > 0 && (
        <TokenList
          items={files.map((file) => ({
            id: fileKey(file),
            label: `${file.name} · ${formatBytes(file.size, locale)}`,
          }))}
          removeLabel={(item) =>
            t("compose.fileRemove", { filename: item.label })
          }
          disabled={disabled}
          onRemove={(id) =>
            onChange((current) =>
              current.filter((file) => fileKey(file) !== id),
            )
          }
        />
      )}
      {refused && <ErrorLine>{refused}</ErrorLine>}
    </div>
  );
}
