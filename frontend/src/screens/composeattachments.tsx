// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a message CARRIES, chosen where it is written.
//
// The send has taken `attachment_ids` since ADR-0086/A131 and no surface in the
// product ever named one: a rep who wanted to send the offer they were writing
// about had to leave the drawer, file the paper on the record, come back and
// start the mail again. Both halves are here now — the record's own library to
// pick from, and an upload for the file that is still on the rep's desktop.
//
// AN UPLOAD FILES IT ON THE RECORD FIRST, and that is the contract rather than a
// convenience: `attachment_ids` names files that already exist, each snapshotted
// at staging so archiving one later cannot rewrite what the timeline says a sent
// message carried. Bytes that reached a recipient and no record would be exactly
// the history that snapshot exists to keep.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Paperclip } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FileDropzone } from "../design-system/filedropzone";
import { Popover } from "../design-system/popover";
import { TokenList } from "../design-system/tokeninput";
import { formatBytes, formatNumber } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { type AttachmentParent, uploadAttachment } from "./attachmentupload";
import { type CarriageViolation, carriageViolations } from "./carriage";
import { useProviderCarriage } from "./channelproviders";
import { problemMessageOf, throwProblem } from "./common";
import type { RelinkKind } from "./compose";
import "./composeattachments.css";

type Attachment = components["schemas"]["Attachment"];

// The contract's own cap (`attachment_ids`, maxItems 10), mirrored so the reader
// is stopped at the shelf rather than by a 422 after they have written the mail.
// It is the same number `maxAttachmentsPerSend` holds on the server; the server
// is still the one that decides, which is why this only stops the ADDING.
const MOST_FILES = 10;

// How much of the record's library to offer. A shelf is a way to reach the paper
// this conversation is about, not a file browser: past this the reader is better
// served by the record's Documents tab, which pages.
const OFFERED = 25;

/** One file this message will carry, as the shelf holds it. */
export type ChosenFile = Readonly<{
  id: string;
  filename: string;
  byteSize?: number | null;
}>;

/**
 * The record's library, for the shelf to offer.
 *
 * `activity` is not a parent this asks about even though the endpoint accepts
 * one: the composer is opened on a RECORD, and a reply's own anchor carries the
 * files that arrived with the message being answered — offering those back would
 * invite a rep to send the customer their own attachment.
 */
export function useRecordFiles(
  entityType: RelinkKind,
  entityId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["attachments", entityType, entityId, "compose"],
    queryFn: async () => {
      const { data, error } = await api.GET("/attachments", {
        params: {
          query: {
            entity_type: entityType,
            entity_id: entityId,
            limit: OFFERED,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled,
  });
}

/**
 * The shelf: what is attached, and the two ways to add to it.
 *
 * It sits UNDER the body, where a mail client puts it and where a rep writing
 * one arrives at it: the sentence about the offer, then the offer. Drawn on
 * every mail, empty or not — a paperclip a rep has to discover is a feature that
 * does not exist for the rep who never found it, and the row costs one line when
 * nothing is attached.
 */
type ShelfProps = Readonly<{
  chosen: readonly ChosenFile[];
  /**
   * An UPDATER, never a finished list.
   *
   * An upload is a round trip, and the reader can tick another file off the
   * library while it is out. Handing over a list computed from the `chosen` this
   * render captured would drop whichever of the two landed first — the bug the
   * mutation-variable rule exists for, arriving through a callback rather than
   * through a `mutationFn`.
   */
  onChange: (
    update: (current: readonly ChosenFile[]) => readonly ChosenFile[],
  ) => void;
  disabled?: boolean;
}>;

/**
 * The paperclip, as one of the message's own quiet verbs.
 *
 * It sits in the editor's footer beside bold and italic rather than in a
 * labelled row of its own, because that is what it IS: something you do to the
 * message you are writing. A row cost a line of the drawer on every mail to say
 * "Files" over an empty space, and drew a full bordered button next to a message
 * it is not more important than.
 */
export function AttachAction({
  entityType,
  entityId,
  chosen,
  onChange,
  disabled,
}: ShelfProps & Readonly<{ entityType: RelinkKind; entityId: string }>) {
  const t = useT();
  return (
    <Popover
      // NO `variant`: a variant draws the trigger as a full control, and this
      // one stands in a row of flat marks. Bare, it inherits the row and takes
      // the same box the formatting buttons do (richtext.css), so five glyphs
      // read as five glyphs rather than four and a button.
      className="richtext-btn"
      label={
        <>
          <Paperclip size={14} aria-hidden="true" />
          <span className="sr-only">{t("compose.attach")}</span>
        </>
      }
    >
      <AttachPicker
        entityType={entityType}
        entityId={entityId}
        chosen={chosen}
        onChange={onChange}
        full={chosen.length >= MOST_FILES}
        disabled={disabled}
      />
    </Popover>
  );
}

/**
 * What the message carries, under the words it is about.
 *
 * Nothing at all when nothing is attached — the row exists to name files, and a
 * label over an empty space names none. The paperclip that fills it lives in the
 * footer above (see {@link AttachAction}), so the way IN is always on screen
 * whether or not this is.
 */
export function AttachedFiles({ chosen, onChange, disabled }: ShelfProps) {
  const t = useT();
  const { locale } = useLocale();
  if (chosen.length === 0) {
    return null;
  }
  return (
    <div className="compose-files">
      <TokenList
        items={chosen.map((file) => ({
          id: file.id,
          label:
            file.byteSize == null
              ? file.filename
              : `${file.filename} · ${formatBytes(file.byteSize, locale)}`,
        }))}
        removeLabel={(item) =>
          t("compose.fileRemove", { filename: item.label })
        }
        disabled={disabled}
        onRemove={(id) =>
          onChange((current) => current.filter((file) => file.id !== id))
        }
      />
      {/* The cap said where it bites, rather than at the send. Ten is the
          contract's, and a message about more records than that is a message
          about none of them — the same reasoning the link list is bounded by. */}
      {chosen.length >= MOST_FILES && (
        <span className="t-caption">
          {t("compose.filesFull", { most: formatNumber(MOST_FILES, locale) })}
        </span>
      )}
    </div>
  );
}

/**
 * The picker behind the paperclip: the record's paper, then an upload.
 *
 * That order on purpose. The file a rep means is usually already on the record —
 * the offer they sent last week, the drawing the customer asked about — and a
 * dropzone at the top would have them upload a second copy of it.
 */
function AttachPicker({
  entityType,
  entityId,
  chosen,
  onChange,
  full,
  disabled,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  chosen: readonly ChosenFile[];
  onChange: (
    update: (current: readonly ChosenFile[]) => readonly ChosenFile[],
  ) => void;
  full: boolean;
  disabled?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const library = useRecordFiles(entityType, entityId, true);
  const [failed, setFailed] = useState<string | null>(null);
  const taken = new Set(chosen.map((file) => file.id));

  // Both the cap and the duplicate rule are re-asked against the list AS IT IS
  // when the change lands, not as this render saw it: the two ways to add — a
  // tick and an upload — can be in flight together.
  const add = (file: Attachment) => {
    onChange((current) => {
      if (current.length >= MOST_FILES) {
        return current;
      }
      if (current.some((held) => held.id === file.id)) {
        return current;
      }
      return [
        ...current,
        { id: file.id, filename: file.filename, byteSize: file.byte_size },
      ];
    });
  };

  // An upload files the bytes on the record and then attaches what came back.
  // The two are one act to the reader and two writes underneath, which is why
  // the record's own file list is re-asked: the Documents tab behind this drawer
  // is now out of date about a file this rep just added.
  const upload = useMutation({
    mutationFn: async (picked: File) => {
      const parent: AttachmentParent = { entityType, entityId };
      return uploadAttachment(parent, picked);
    },
    onMutate: () => setFailed(null),
    onError: (error) => setFailed(problemMessageOf(error, t)),
    onSuccess: (stored) => {
      void queryClient.invalidateQueries({
        queryKey: ["attachments", entityType, entityId],
      });
      // The bytes are stored either way; what can be missing is the metadata,
      // and an id is exactly what attaching needs. Reported rather than
      // swallowed: the file IS on the record, and telling the rep it failed
      // would have them upload it twice.
      if (!stored) {
        setFailed(t("compose.fileStoredUnnamed"));
        return;
      }
      add(stored);
    },
  });

  const files = library.data?.data ?? [];
  return (
    <div className="compose-files-picker">
      <h3 className="t-eyebrow">{t("compose.filesOnRecord")}</h3>
      {library.isPending ? (
        <PendingBody label={t("compose.filesLoading")} lines={2} />
      ) : library.isError ? (
        <p className="t-caption" role="alert">
          {problemMessageOf(library.error, t)}
        </p>
      ) : files.length === 0 ? (
        <p className="t-caption">{t("compose.filesNone")}</p>
      ) : (
        <ul className="compose-files-list">
          {files.map((file) => (
            <li key={file.id}>
              <Button
                small
                className="compose-files-row"
                disabled={disabled || taken.has(file.id) || full}
                onClick={() => add(file)}
              >
                <span className="compose-files-name">{file.filename}</span>
                {file.byte_size != null && (
                  <span className="t-caption">
                    {formatBytes(file.byte_size, locale)}
                  </span>
                )}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <FileDropzone
        label={t("compose.fileUpload")}
        hint={t("compose.fileUploadHint")}
        emptyLabel={t("compose.fileUploadEmpty")}
        onPick={(picked) => upload.mutate(picked)}
      />
      {upload.isPending && (
        <p className="t-caption" aria-live="polite">
          {t("compose.fileUploading")}
        </p>
      )}
      {failed && (
        <p className="t-caption" role="alert">
          {failed}
        </p>
      )}
    </div>
  );
}

/**
 * Where this message breaks the resolved channel's carriage bounds, read from
 * the directory the transport publishes.
 *
 * Empty when the transport is unknown — mail, or a provider the directory does
 * not name — so the mail path keeps its own limits and only a channel reply is
 * held here. The dispatcher's `carriageRefusal` is the authority (backend
 * `comms/gates.go`); this pre-check moves the same answer in front of the send.
 */
export function useCarriageBlocks(
  provider: string | undefined,
  files: readonly ChosenFile[],
  body: string,
): CarriageViolation[] {
  const carriageFor = useProviderCarriage();
  const carriage = provider ? carriageFor(provider) : undefined;
  return carriage ? carriageViolations(carriage, files, body) : [];
}

// carriageReason is one broken bound in the reader's own language. The wording
// lives here, where `t` does, and not in the pure check that found the
// violation — the check is language-free so a test can assert the bound rather
// than the sentence. Bytes read through `formatBytes` and counts through
// `formatNumber`, as everywhere a size or a tally reaches a reader.
function carriageReason(
  t: Translator,
  locale: Locale,
  channel: string,
  violation: CarriageViolation,
): string {
  switch (violation.kind) {
    case "carries":
      return t("compose.carriageCarries", {
        channel,
        count: formatNumber(violation.count, locale),
      });
    case "count":
      return t("compose.carriageCount", {
        channel,
        limit: formatNumber(violation.limit, locale),
        named: formatNumber(violation.named, locale),
      });
    case "perFile":
      return t("compose.carriagePerFile", {
        channel,
        filename: violation.filename,
        limit: formatBytes(violation.limit, locale),
      });
    case "aggregate":
      return t("compose.carriageAggregate", {
        channel,
        count: formatNumber(violation.count, locale),
        total: formatBytes(violation.total, locale),
        limit: formatBytes(violation.limit, locale),
      });
    case "caption":
      return t("compose.carriageCaption", {
        channel,
        limit: formatNumber(violation.limit, locale),
        length: formatNumber(violation.length, locale),
      });
  }
}

/**
 * Why the attached files cannot go as they are, beside the shelf they are on.
 *
 * `standing`: the bounds are re-read from the draft's own files and text on
 * every render, so this is true AS the shelf renders rather than an answer to
 * one press, and there is nothing to interrupt a reader for. Null when nothing
 * is wrong, or when there is no resolved channel to name — mail warns nowhere
 * here, holding to its own path's limits.
 */
export function CarriageNotice({
  channel,
  blocks,
}: Readonly<{ channel?: string; blocks: readonly CarriageViolation[] }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!channel || blocks.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="warn"
      kind="standing"
      title={t("compose.carriageTitle", { channel })}
    >
      <ul className="compose-carriage">
        {blocks.map((violation) => (
          <li
            key={
              violation.kind === "perFile"
                ? `perFile-${violation.filename}`
                : violation.kind
            }
          >
            {carriageReason(t, locale, channel, violation)}
          </li>
        ))}
      </ul>
    </Callout>
  );
}
