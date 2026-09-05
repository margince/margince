import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button, Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import "./dealroomdocuments.css";

// The seller's verbs on a Deal Room's documents: which of the deal's files the
// buyer gets to read, under which of the four fixed groups. The board that
// draws them lives in dealroomthreads.tsx; nothing added here reaches the
// buyer until the room is published.

type DealRoom = components["schemas"]["DealRoom"];

// The four groups are machine keys on the wire; these are their names.
export const DOCUMENT_GROUPS: readonly {
  key: string;
  labelKey: MessageKey;
}[] = [
  { key: "commercial", labelKey: "room.docs.group.commercial" },
  { key: "legal", labelKey: "room.docs.group.legal" },
  { key: "security_privacy", labelKey: "room.docs.group.security_privacy" },
  {
    key: "delivery_operations",
    labelKey: "room.docs.group.delivery_operations",
  },
];

export function useRoomDocuments(roomId: string) {
  return useQuery({
    queryKey: ["deal-room-documents", roomId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-rooms/{id}/documents", {
        params: { path: { id: roomId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function AddDocument({
  room,
  refusal,
}: Readonly<{ room: DealRoom; refusal: string | undefined }>) {
  const t = useT();
  const [attachmentId, setAttachmentId] = useState("");
  const [group, setGroup] = useState(DOCUMENT_GROUPS[0].key);
  // The deal's Files area — uploads and the files its emails carried, hidden
  // ones excluded — is what a room may share; the server refuses anything else.
  const files = useQuery({
    queryKey: ["deal-documents", room.deal_id, false],
    enabled: refusal === undefined,
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}/documents", {
        params: { path: { id: room.deal_id }, query: { limit: 100 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const add = useAddDocument(room.id);
  if (refusal !== undefined) {
    return <p className="t-caption">{refusal}</p>;
  }
  const options = (files.data?.data ?? []).map((doc) => ({
    value: doc.attachment.id,
    label: doc.attachment.title || doc.attachment.filename,
  }));
  return (
    <>
      <Field label={t("room.docs.fileLabel")} hint={t("room.docs.fileHint")}>
        {(control) => (
          <Select
            id={control.id}
            options={options}
            value={attachmentId}
            onChange={setAttachmentId}
            placeholder={
              options.length === 0
                ? t("room.docs.noFiles")
                : t("room.docs.pickFile")
            }
            disabled={options.length === 0}
          />
        )}
      </Field>
      <Field label={t("room.docs.groupLabel")}>
        {(control) => (
          <Select
            id={control.id}
            options={DOCUMENT_GROUPS.map((g) => ({
              value: g.key,
              label: t(g.labelKey),
            }))}
            value={group}
            onChange={setGroup}
          />
        )}
      </Field>
      <div className="card-actions">
        <Button
          small
          disabled={attachmentId === ""}
          pending={add.isPending}
          onClick={() =>
            add.mutate(
              { attachmentId, group },
              { onSuccess: () => setAttachmentId("") },
            )
          }
        >
          {t("room.docs.add")}
        </Button>
      </div>
      <p className="t-caption">{t("room.editorial")}</p>
      {add.isError ? (
        <p className="t-caption t-danger">{problemMessageOf(add.error, t)}</p>
      ) : null}
    </>
  );
}

function useAddDocument(roomId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["deal-room-document-add"],
    mutationFn: async (input: { attachmentId: string; group: string }) => {
      const { data, error } = await api.POST("/deal-rooms/{id}/documents", {
        params: { path: { id: roomId } },
        body: {
          attachment_id: input.attachmentId,
          group_key: input.group,
          source: "ui",
        },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["deal-room-documents", roomId],
      });
    },
  });
}

export function useRemoveDocument(roomId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["deal-room-document-remove"],
    mutationFn: async (input: {
      documentId: string;
      version: number | undefined;
    }) => {
      const { data, error } = await api.DELETE(
        "/deal-rooms/{id}/documents/{documentId}",
        {
          params: {
            path: { id: roomId, documentId: input.documentId },
            ...ifMatch(requireVersion(input.version)),
          },
        },
      );
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["deal-room-documents", roomId],
      });
    },
  });
}
