import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Upload } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { Button, Field } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemCodeOf, refusalSentences, unwrap } from "../../lib/problem";
import { AddDocumentDialog } from "../voice-knowledge/adddocument";
import { roomBriefKey } from "./dealroombrief.key";
import "./dealroomdocuments.css";
import { dealFilesKey, roomDocumentsKey } from "../../lib/attachmentkeys";

// The seller's verbs on a Deal Room's documents: which of the deal's files the
// buyer gets to read, under which of the four fixed groups. The board that
// draws them lives in dealroomthreads.tsx; nothing added here reaches the
// buyer until the room is published.
//
// A room shares the deal's OWN files rather than holding files of its own, so
// there are two steps and the screen names both: a file is uploaded to the deal
// (the same dialog the deal's Files area opens, filing against the same record),
// and then chosen here. Without the first control on this page a rep with an
// empty Files area read "The deal's Files area is empty" and had nowhere on the
// screen to go — the upload existed one tab away and nothing said so.

type DealRoom = components["schemas"]["DealRoom"];

const ALREADY_PRESENT = "deal_room_document_already_present";

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
    queryKey: roomDocumentsKey(roomId),
    queryFn: async () => {
      return unwrap(
        await api.GET("/deal-rooms/{id}/documents", {
          params: { path: { id: roomId } },
        }),
      );
    },
  });
}

export function AddDocument({
  room,
  refusal,
}: Readonly<{ room: DealRoom; refusal: string | undefined }>) {
  const t = useT();
  const [attachmentId, setAttachmentId] = useState("");
  const [group, setGroup] = useState(DOCUMENT_GROUPS[0]?.key ?? "");
  const [uploading, setUploading] = useState(false);
  // The deal's Files area — uploads and the files its emails carried, hidden
  // ones excluded — is what a room may share; the server refuses anything else.
  const files = useQuery({
    queryKey: [...dealFilesKey(room.deal_id), false],
    enabled: refusal === undefined,
    queryFn: async () => {
      return unwrap(
        await api.GET("/deals/{id}/documents", {
          params: { path: { id: room.deal_id }, query: { limit: 100 } },
        }),
      );
    },
  });
  const add = useAddDocument(room.id);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  if (refusal !== undefined) {
    return <p>{refusal}</p>;
  }
  const labels = {
    attachment_id: t("room.docs.fileLabel"),
    group_key: t("room.docs.groupLabel"),
  };
  const refused = refusalSentences(add.error, t, labels);
  // The file already in the room is the file picked, so the refusal stands on
  // that field rather than at the foot.
  const alreadyPresent = problemCodeOf(add.error) === ALREADY_PRESENT;
  const placed = (error: unknown): ReadonlyMap<string, string> => {
    const answer = refusalSentences(error, t, labels);
    return problemCodeOf(error) === ALREADY_PRESENT && answer.rest
      ? new Map([["attachment_id", answer.rest]])
      : answer.byField;
  };
  const options = (files.data?.data ?? []).map((doc) => ({
    value: doc.attachment.id,
    label: doc.attachment.title || doc.attachment.filename,
  }));
  return (
    <>
      <div ref={faultsRoot} className="form-stack">
        <Field
          label={labels.attachment_id}
          hint={t("room.docs.fileHint")}
          error={faults.of("attachment_id")}
        >
          {(control) => (
            <Select
              {...control}
              options={options}
              value={attachmentId}
              onChange={(next) => {
                setAttachmentId(next);
                faults.edited("attachment_id");
              }}
              placeholder={
                options.length === 0
                  ? t("room.docs.noFiles")
                  : t("room.docs.pickFile")
              }
              disabled={options.length === 0}
            />
          )}
        </Field>
        <Field label={labels.group_key} error={faults.of("group_key")}>
          {(control) => (
            <Select
              {...control}
              options={DOCUMENT_GROUPS.map((g) => ({
                value: g.key,
                label: t(g.labelKey),
              }))}
              value={group}
              onChange={(next) => {
                setGroup(next);
                faults.edited("group_key");
              }}
            />
          )}
        </Field>
      </div>
      <div className="card-actions">
        <Button
          disabled={attachmentId === ""}
          pending={add.isPending}
          onClick={() =>
            add.mutate(
              { attachmentId, group },
              {
                onSuccess: () => setAttachmentId(""),
                onError: (error) => faults.show(placed(error)),
              },
            )
          }
        >
          {t("room.docs.add")}
        </Button>
        {/* The way out of an empty picker, beside the picker rather than on
            another tab. The dialog files against the DEAL — the room shares the
            deal's files and owns none — and the landed upload invalidates
            `deal-documents`, which is this list's own key, so the new file is
            in the picker when the dialog closes. */}
        <Button variant="ghost" onClick={() => setUploading(true)}>
          <Upload aria-hidden />
          {t("room.docs.upload")}
        </Button>
      </div>
      <AddDocumentDialog
        anchor={{ record: "deal", id: room.deal_id }}
        open={uploading}
        onClose={() => setUploading(false)}
      />
      <p className="t-caption">{t("room.editorial")}</p>
      {refused.rest && !alreadyPresent ? (
        <ErrorLine>{refused.rest}</ErrorLine>
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
      return unwrap(
        await api.POST("/deal-rooms/{id}/documents", {
          params: { path: { id: roomId } },
          body: {
            attachment_id: input.attachmentId,
            group_key: input.group,
            source: "manual",
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: roomDocumentsKey(roomId),
      });
      queryClient.invalidateQueries({ queryKey: roomBriefKey(roomId) });
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
      return unwrap(
        await api.DELETE("/deal-rooms/{id}/documents/{documentId}", {
          params: {
            path: { id: roomId, documentId: input.documentId },
            ...ifMatch(requireVersion(input.version)),
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: roomDocumentsKey(roomId),
      });
      queryClient.invalidateQueries({ queryKey: roomBriefKey(roomId) });
    },
  });
}
