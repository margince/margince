import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Button, Field, Textarea } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { QueryGate, throwProblem, unwrap } from "./common";
import { SIGN_OFF_QUERY, useSignOff } from "./composesignoff";
import { SignatureHtml } from "./signaturehtml";

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
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: TEMPLATE_KEY });
      void queryClient.invalidateQueries({ queryKey: ["me-email-signature"] });
      void queryClient.invalidateQueries({ queryKey: [SIGN_OFF_QUERY] });
    },
  });
}

// The workspace's signature layout. An admin writes it once with the
// placeholders, and every member's mail signs with it in their own values.
// The preview is the reader's own signature as a recipient sees it.
export function SignatureTemplateCard() {
  const t = useT();
  const canEdit = useCanWrite("installation_settings", "update");
  const query = useSignatureTemplate();
  const save = useSaveSignatureTemplate();
  const [draft, setDraft] = useState<string | null>(null);
  // The preview shows the template as typed, before it is saved.
  const preview = useSignOff(
    "",
    "",
    draft === null ? undefined : { template: draft },
  ).signOff;
  return (
    <Panel title={t("signatureTemplate.title")}>
      <PanelBody>
        <PanelIntro>{t("signatureTemplate.sub")}</PanelIntro>
        <QueryGate query={query} pendingLabel={t("signatureTemplate.title")}>
          {(stored) => {
            const shown = draft ?? stored.template;
            return (
              <form
                className="form-stack"
                onSubmit={(event) => {
                  event.preventDefault();
                  save.mutate(shown, { onSuccess: () => setDraft(null) });
                }}
              >
                <Field label={t("signatureTemplate.label")}>
                  {(control) => (
                    <Textarea
                      {...control}
                      rows={6}
                      value={shown}
                      disabled={!canEdit}
                      placeholder={t("signatureTemplate.placeholder")}
                      onChange={(event) => setDraft(event.target.value)}
                    />
                  )}
                </Field>
                <p className="t-caption">{t("signatureTemplate.hint")}</p>
                <ErrorLine error={save.isError ? save.error : undefined} />
                <div className="actions">
                  <Button
                    type="submit"
                    variant="primary"
                    disabled={!canEdit || shown === stored.template}
                    pending={save.isPending}
                    busyLabel={t("settings.signatureSaving")}
                  >
                    {t("record.save")}
                  </Button>
                </div>
                {preview?.html ? (
                  <SignatureHtml
                    html={preview.html}
                    title={t("settings.signaturePreview")}
                  />
                ) : null}
              </form>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
