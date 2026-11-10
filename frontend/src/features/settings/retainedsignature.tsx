// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { SignatureHtml } from "../../components/signaturehtml";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { sameEditValue } from "../../lib/independentedit";
import {
  type ReadySignature,
  readRetainedSignature,
  type SignatureValues,
  saveRetainedReview,
  signatureValues,
} from "./retainedsignaturecalls";
import { SignatureFields } from "./settingssignatureeditor";

type ReviewDraft = Readonly<{
  opened: ReadySignature;
  values: SignatureValues;
}>;
type SaveReview = ReviewDraft & Readonly<{ missingVersion: string }>;
const queryKey = (id: string) => ["retained-email-signature", id];

export function RetainedSignaturePanel({ id }: Readonly<{ id: string }>) {
  return <RetainedSignatureReview key={id} id={id} />;
}

function RetainedSignatureReview({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const cache = useQueryClient();
  const [draft, setDraft] = useState<ReviewDraft>();
  const missingVersion = t("settings.signatureReviewReload");
  const query = useQuery({
    queryKey: queryKey(id),
    queryFn: ({ signal }) => readRetainedSignature(id, missingVersion, signal),
  });
  const mutation = useMutation({
    mutationFn: ({ opened, values, missingVersion }: SaveReview) =>
      saveRetainedReview(opened, values, missingVersion, (latest) =>
        cache.setQueryData(queryKey(latest.id), latest),
      ),
    onSuccess: (saved) => {
      cache.setQueryData(queryKey(saved.id), saved);
      setDraft(undefined);
      toast.show(t("settings.signatureReviewSaved"));
    },
  });
  const dirty =
    draft !== undefined &&
    !sameEditValue(draft.values, signatureValues(draft.opened));
  const changed =
    !mutation.isPending &&
    draft !== undefined &&
    query.data !== undefined &&
    query.data.version !== draft.opened.version;
  useSaveDraft({
    dirty,
    invalid: changed,
    save: () => {
      if (draft === undefined) return Promise.reject(new Error(missingVersion));
      return mutation.mutateAsync({ ...draft, missingVersion });
    },
    discard: () => {
      setDraft(undefined);
      mutation.reset();
    },
  });
  return (
    <QueryGate
      query={query}
      pendingLabel={t("settings.signatureReviewLoading")}
    >
      {(signature) => (
        <RetainedSignatureEditor
          signature={signature}
          values={draft?.values ?? signatureValues(signature)}
          changed={changed}
          pending={mutation.isPending}
          change={(field, value) =>
            setDraft((before) => {
              const editing = before ?? {
                opened: signature,
                values: signatureValues(signature),
              };
              return {
                ...editing,
                values: { ...editing.values, [field]: value },
              };
            })
          }
          useLatest={() => {
            setDraft(undefined);
            mutation.reset();
          }}
          keepDraft={() => {
            setDraft((before) =>
              before === undefined
                ? undefined
                : { ...before, opened: signature },
            );
            mutation.reset();
          }}
        />
      )}
    </QueryGate>
  );
}

function RetainedSignatureEditor({
  signature,
  values,
  changed,
  pending,
  change,
  useLatest,
  keepDraft,
}: Readonly<{
  signature: ReadySignature;
  values: SignatureValues;
  changed: boolean;
  pending: boolean;
  change: (field: keyof SignatureValues, value: string) => void;
  useLatest: () => void;
  keepDraft: () => void;
}>) {
  const t = useT();
  return (
    <Panel
      title={t("settings.signatureReviewTitle", {
        name: signature.owner_name || t("settings.signatureReviewMember"),
      })}
    >
      <PanelBody>
        <PanelIntro>{t("settings.signatureReviewIntro")}</PanelIntro>
        {changed && (
          <>
            <Callout
              kind="outcome"
              tone="warning"
              title={t("settings.signatureReviewChangedTitle")}
            >
              {t("settings.signatureReviewChanged")}
            </Callout>
            <Panel title={t("settings.signatureReviewLatest")}>
              <PanelBody>
                <SignatureFields
                  shown={{ ...signatureValues(signature), format: "html" }}
                  mode="retained"
                  disabled
                  change={change}
                />
                <div className="actions">
                  <Button
                    variant="ghost"
                    disabled={pending}
                    onClick={useLatest}
                  >
                    {t("settings.signatureReviewUseLatest")}
                  </Button>
                  <Button
                    variant="ghost"
                    disabled={pending}
                    onClick={keepDraft}
                  >
                    {t("settings.signatureReviewKeepDraft")}
                  </Button>
                </div>
              </PanelBody>
            </Panel>
          </>
        )}
        <div className="form-stack">
          <SignatureFields
            shown={{ ...values, format: values.html ? "html" : "text" }}
            mode="retained"
            disabled={pending}
            change={change}
          />
          {signature.preview_html && (
            <>
              <p className="t-caption">
                {t("settings.signatureReviewPreview")}
              </p>
              <SignatureHtml
                html={signature.preview_html}
                title={t("settings.signatureReviewPreview")}
                resolveEmbeddedLogo={false}
              />
            </>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}
