// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Checkbox, Textarea } from "../../design-system/field";
import { FileDropzone } from "../../design-system/filedropzone";
import { Panel, PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { loggedActivityKeys } from "../../lib/activitykeys";
import { uploadAttachment } from "../../lib/attachmentupload";
import { useChannelProviderOptions } from "../../lib/channelproviders";
import { problemMessageOf, unwrap } from "../../lib/problem";

// Capturing a chat from a screenshot pasted on a contact. The image is stored
// on the contact, read for its messages, and shown for the reader to check.
// Only "File on this contact" writes the message; until then the screenshot
// is a document on the contact, and Discard archives it.

type Reading = components["schemas"]["AttachmentExtraction"];
type Started = Readonly<{ attachmentId: string; readingId: string }>;

// How often a running reading is asked about. A screenshot reads in seconds,
// so a slower poll would leave the reader waiting on the poll, not the model.
const READING_POLL_MS = 1500;

function groundedValue(reading: Reading | undefined, field: string): string {
  return reading?.fields.find((f) => f.field === field)?.value ?? "";
}

function isLive(reading: Reading | undefined): boolean {
  return reading?.status === "queued" || reading?.status === "running";
}

// Polls the reading this panel started, by its id: the newest reading of the
// file could be one somebody else started for another question.
function useChatReading(started: Started | undefined) {
  return useQuery({
    queryKey: [
      "attachment-extraction",
      started?.attachmentId,
      started?.readingId,
    ],
    enabled: started !== undefined,
    queryFn: async () =>
      unwrap(
        await api.GET("/attachments/{id}/extractions/{extraction_id}", {
          params: {
            path: {
              id: started?.attachmentId ?? "",
              extraction_id: started?.readingId ?? "",
            },
          },
        }),
      ),
    refetchInterval: (query) =>
      isLive(query.state.data) ? READING_POLL_MS : false,
  });
}

// Store the screenshot on the contact and start reading it for a chat.
function useStartReading(contactId: string, onStored: (id: string) => void) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (file: File): Promise<Started> => {
      const stored = await uploadAttachment(
        { entityType: "contact", entityId: contactId },
        file,
      );
      if (!stored) {
        throw new Error(t("chatCapture.storedUnread"));
      }
      onStored(stored.id);
      const reading = unwrap(
        await api.POST("/attachments/{id}/extraction", {
          params: { path: { id: stored.id } },
          body: { question: "chat_capture" },
        }),
      );
      return { attachmentId: stored.id, readingId: reading.id };
    },
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: ["attachments", "contact", contactId],
      }),
  });
}

export function ContactChatCapture({
  contactId,
  contactName,
  onClose,
}: Readonly<{ contactId: string; contactName: string; onClose: () => void }>) {
  const t = useT();
  // Known as soon as the bytes are stored, so Discard reaches a screenshot
  // whose reading could not be started.
  const [attachmentId, setAttachmentId] = useState<string | null>(null);
  const start = useStartReading(contactId, setAttachmentId);
  const reading = useChatReading(start.data);
  const discard = useMutation({
    mutationFn: async () => {
      if (attachmentId) {
        unwrap(
          await api.DELETE("/attachments/{id}", {
            params: { path: { id: attachmentId } },
          }),
        );
      }
    },
    onSuccess: onClose,
  });
  const read = reading.data;
  const finished = read !== undefined && !isLive(read);

  return (
    <Panel
      tone="ai"
      title={t("chatCapture.title")}
      titleAction={
        <Button
          variant="ghost"
          disabled={discard.isPending}
          onClick={() => discard.mutate()}
        >
          {t("chatCapture.discard")}
        </Button>
      }
    >
      <PanelBody>
        {attachmentId === null && (
          <FileDropzone
            label={t("chatCapture.drop")}
            emptyLabel={t("chatCapture.dropEmpty")}
            accept="image/*"
            pasteable
            disabled={start.isPending}
            onPick={(file) => start.mutate(file)}
          />
        )}
        {start.isError && (
          <Callout tone="danger" title={t("chatCapture.startFailed")}>
            {problemMessageOf(start.error, t)}
          </Callout>
        )}
        {start.isSuccess && !finished && !reading.isError && (
          <SurfaceState
            state="loading"
            loadingLabel={t("chatCapture.reading")}
            emptyLabel=""
          >
            {null}
          </SurfaceState>
        )}
        {reading.isError && (
          <Callout tone="danger" title={t("chatCapture.startFailed")}>
            {problemMessageOf(reading.error, t)}
          </Callout>
        )}
        {start.data && finished && (
          <ChatReview
            contactId={contactId}
            contactName={contactName}
            attachmentId={start.data.attachmentId}
            reading={read}
            onFiled={onClose}
          />
        )}
        {discard.isError && (
          <Callout tone="danger" title={t("chatCapture.discardFailed")}>
            {problemMessageOf(discard.error, t)}
          </Callout>
        )}
      </PanelBody>
    </Panel>
  );
}

// The finished reading, for the reader to check and file. The text starts as
// what the model copied and is marked so; a change makes it the reader's own.
function ChatReview({
  contactId,
  contactName,
  attachmentId,
  reading,
  onFiled,
}: Readonly<{
  contactId: string;
  contactName: string;
  attachmentId: string;
  reading: Reading;
  onFiled: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const readText = groundedValue(reading, "chat_text");
  const [edited, setEdited] = useState<string | null>(null);
  const text = edited ?? readText;
  const options = useChannelProviderOptions();
  const [picked, setPicked] = useState("");
  // A chat whose header names someone else files here only once the reader
  // says it belongs here; the server refuses it otherwise.
  const other = reading.other_counterparty ?? null;
  const [confirmedOther, setConfirmedOther] = useState(false);
  const channel = picked || (options[0]?.value ?? "");
  const who = [
    groundedValue(reading, "counterparty_name"),
    groundedValue(reading, "counterparty_phone"),
  ]
    .filter(Boolean)
    .join(", ");

  const file = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/attachments/{id}/extraction:file-chat", {
          params: { path: { id: attachmentId } },
          body: {
            extraction_id: reading.id,
            channel_provider: channel,
            body: text,
            confirm_other_counterparty: other !== null && confirmedOther,
          },
        }),
      ),
    onSuccess: () => {
      for (const queryKey of [
        ...loggedActivityKeys("message", "contact", contactId),
        ["attachments", "contact", contactId],
      ]) {
        queryClient.invalidateQueries({ queryKey });
      }
      onFiled();
    },
  });

  return (
    <>
      {reading.status_detail && (
        <Callout
          tone={reading.status === "failed" ? "warning" : "info"}
          title={t(
            reading.status === "failed"
              ? "chatCapture.unread"
              : "chatCapture.nothingRead",
          )}
        >
          {reading.status_detail}
        </Callout>
      )}
      {other !== null ? (
        <Callout
          tone="warning"
          title={t("chatCapture.otherCounterparty", {
            shows: other,
            contact: contactName,
          })}
        >
          <Checkbox
            label={t("chatCapture.confirmOther", { contact: contactName })}
            checked={confirmedOther}
            onChange={(event) => setConfirmedOther(event.target.checked)}
          />
        </Callout>
      ) : (
        who && <p>{t("chatCapture.header", { who })}</p>
      )}
      <Field
        label={t("chatCapture.text")}
        hint={
          edited === null && readText ? undefined : t("chatCapture.textHint")
        }
      >
        {(control) => (
          <>
            {edited === null && readText && (
              <Badge tone="ai">{t("chatCapture.readBadge")}</Badge>
            )}
            <Textarea
              {...control}
              rows={8}
              value={text}
              onChange={(event) => setEdited(event.target.value)}
            />
          </>
        )}
      </Field>
      <Field label={t("chatCapture.channel")}>
        {(control) => (
          <Select
            {...control}
            options={options}
            value={channel}
            onChange={setPicked}
          />
        )}
      </Field>
      {file.isError && (
        <Callout tone="danger" title={t("chatCapture.fileFailed")}>
          {problemMessageOf(file.error, t)}
        </Callout>
      )}
      <div className="card-actions">
        <Button
          variant="primary"
          disabled={
            !channel || file.isPending || (other !== null && !confirmedOther)
          }
          onClick={() => file.mutate()}
        >
          {t("chatCapture.file")}
        </Button>
      </div>
    </>
  );
}
