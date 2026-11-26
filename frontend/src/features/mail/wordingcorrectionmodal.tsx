// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { RichText } from "../../design-system/richtext";
import { useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { useRichTextLabels } from "../../lib/richtextlabels";

type CorrectionContext = components["schemas"]["WordingCorrectionContext"];
type CorrectionRequest =
  components["schemas"]["CorrectCommunicationWordingRequest"];

async function saveCorrection(
  command: Readonly<{ reviewId: string; body: CorrectionRequest }>,
) {
  const result = await api.POST("/communication-reviews/{id}/correct-wording", {
    params: { path: { id: command.reviewId } },
    body: command.body,
  });
  if (!result.response.ok || !result.data) {
    throwProblem(
      result.error || { title: "The wording correction was not saved." },
    );
  }
  return result.data;
}

export function WordingCorrectionModal({
  reviewId,
  context,
  onClose,
  onSaved,
}: Readonly<{
  reviewId: string;
  context: CorrectionContext;
  onClose: () => void;
  onSaved: () => void;
}>) {
  const t = useT();
  const labels = useRichTextLabels();
  const client = useQueryClient();
  const [subject, setSubject] = useState(context.subject);
  const [body, setBody] = useState(context.body);
  const [html, setHtml] = useState(context.html_body);
  const [acknowledged, setAcknowledged] = useState(false);
  const save = useMutation({
    mutationFn: saveCorrection,
    onSuccess: async () => {
      onSaved();
      await client.invalidateQueries({
        queryKey: ["communication-review", reviewId],
      });
      await client.invalidateQueries({ queryKey: ["scheduled-sends"] });
    },
  });
  const changed =
    subject !== context.subject ||
    body !== context.body ||
    html !== context.html_body;
  const missing = !changed
    ? t("directSend.correctionNeedsEdit")
    : !acknowledged
      ? t("directSend.correctionNeedsConfirmation")
      : !subject.trim() || !body.trim()
        ? t("directSend.correctionNeedsText")
        : undefined;

  return (
    <ConfirmModal
      open
      intent="form"
      title={t("directSend.correctionTitle")}
      confirmLabel={t("directSend.correctionSave")}
      confirmDisabled={Boolean(missing)}
      confirmReason={missing}
      pending={save.isPending}
      error={save.isError ? t("directSend.correctionFailed") : undefined}
      onClose={onClose}
      onConfirm={() =>
        save.mutate({
          reviewId,
          body: {
            version: context.version,
            expected_wording: context.wording_digest,
            subject,
            body,
            html_body: html,
            meaning_unchanged: acknowledged,
          },
        })
      }
    >
      <p className="t-body">{t("directSend.correctionScope")}</p>
      <Field label={t("compose.subject")}>
        {(control) => (
          <TextInput
            {...control}
            value={subject}
            maxLength={998}
            disabled={save.isPending}
            onChange={(event) => {
              setSubject(event.target.value);
              setAcknowledged(false);
            }}
          />
        )}
      </Field>
      {context.html_body ? (
        <RichText
          value={html}
          onChange={(next) => {
            setBody(next.text);
            setHtml(next.html);
            setAcknowledged(false);
          }}
          label={t("compose.body")}
          labels={labels}
          rows={8}
          disabled={save.isPending}
        />
      ) : (
        <Field label={t("compose.body")}>
          {(control) => (
            <Textarea
              {...control}
              value={body}
              rows={8}
              disabled={save.isPending}
              onChange={(event) => {
                setBody(event.target.value);
                setAcknowledged(false);
              }}
            />
          )}
        </Field>
      )}
      <Checkbox
        checked={acknowledged}
        disabled={save.isPending}
        label={t("directSend.correctionAcknowledge")}
        onChange={(event) => setAcknowledged(event.target.checked)}
      />
    </ConfirmModal>
  );
}
