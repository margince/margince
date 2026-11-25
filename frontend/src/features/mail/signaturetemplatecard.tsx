import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { SignatureHtml } from "../../components/signaturehtml";
import { Field, Textarea } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { lockFor } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { SIGN_OFF_QUERY, useSignOff } from "./composesignoff";

const TEMPLATE_KEY = ["email-signature-template"] as const;

function useSignatureTemplate() {
  return useQuery({
    queryKey: TEMPLATE_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/email-signature-template",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useSaveSignatureTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (template: string) => {
      return unwrap(
        await api.PUT("/email-signature-template", {
          body: { template },
        }),
      );
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(TEMPLATE_KEY, saved);
      void queryClient.invalidateQueries({ queryKey: TEMPLATE_KEY });
      void queryClient.invalidateQueries({ queryKey: ["me-email-signature"] });
      void queryClient.invalidateQueries({ queryKey: [SIGN_OFF_QUERY] });
    },
  });
}

// The workspace's signature layout. An admin writes it once with the
// placeholders, and every member's mail signs with it in their own values.
// The preview is the reader's own signature as a recipient sees it. The page's
// save bar writes a changed layout.
export function SignatureTemplateCard() {
  const t = useT();
  const canEdit = useCanWrite("installation_settings", "update");
  // The read-only line waits for /me, so a reader who may edit never sees it.
  const readOnly = useMe().isSuccess && !canEdit;
  const query = useSignatureTemplate();
  const save = useSaveSignatureTemplate();
  const [draft, setDraft] = useState<string | null>(null);
  const stored = query.data?.template;
  useSaveDraft({
    dirty: draft !== null && stored !== undefined && draft !== stored,
    save: async () => {
      if (draft === null) return;
      await save.mutateAsync(draft);
      setDraft(null);
    },
    discard: () => {
      setDraft(null);
      save.reset();
    },
  });
  // The preview shows the template as typed, before it is saved.
  const preview = useSignOff(
    "",
    "",
    draft === null ? { html: "" } : { template: draft, html: "" },
  ).signOff;
  return (
    <Panel title={t("signatureTemplate.title")}>
      <PanelBody>
        <PanelIntro>{t("signatureTemplate.sub")}</PanelIntro>
        {readOnly && <p>{t("installationSettings.readOnly")}</p>}
        <QueryGate query={query} pendingLabel={t("signatureTemplate.title")}>
          {(template) => {
            const shown = draft ?? template.template;
            const lock = lockFor(
              template.locks,
              "contacts.signature_template",
              t,
            );
            return (
              <div className="form-stack">
                <Field
                  label={t("signatureTemplate.label")}
                  hint={lock.disabled ? lock.reason : undefined}
                >
                  {(control) => (
                    <Textarea
                      maxLength={TEXT_LIMITS.document}
                      {...control}
                      rows={6}
                      value={shown}
                      disabled={!canEdit || lock.disabled || save.isPending}
                      placeholder={t("signatureTemplate.placeholder")}
                      onChange={(event) => setDraft(event.target.value)}
                    />
                  )}
                </Field>
                <p className="t-caption">{t("signatureTemplate.hint")}</p>
                {preview?.html ? (
                  <SignatureHtml
                    html={preview.html}
                    title={t("settings.signaturePreview")}
                  />
                ) : null}
              </div>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
