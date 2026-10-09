import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Button,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { SettingRow } from "../design-system/settingrow";
import type { Toast } from "../design-system/toast";
import { useT } from "../i18n";
import { throwProblem, WriteRefused } from "./common";
import { useSignOff } from "./composesignoff";
import { useSaveSignature } from "./settingssignature";
import { SignatureHtml } from "./signaturehtml";

type Draft = { body: string; title: string; phone: string };

// The sign-off appended below every message this member sends, as one row.
//
// It lives beside identity rather than under the composer because it is who
// the sender IS, not something about one mail. When the workspace has a
// signature template, the member fills in their title and phone and the
// template signs their mail; otherwise they write their own plain text.
export function SignatureSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Draft | null>(null);
  const signature = useQuery({
    queryKey: ["me-email-signature"],
    queryFn: async () => {
      const { data, error } = await api.GET("/me/email-signature");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const templated = signature.data?.template_active === true;
  const save = useSaveSignature((saved) => {
    // Hand the edit back to the server's answer: it trims what it stores.
    setDraft(
      saved
        ? { body: saved.body, title: saved.title, phone: saved.phone }
        : null,
    );
    setOpen(false);
    toast.show(t("settings.saved"));
  });

  const stored: Draft = {
    body: signature.data?.body ?? "",
    title: signature.data?.title ?? "",
    phone: signature.data?.phone ?? "",
  };
  // The saved value until the member types; theirs from then on.
  const shown = draft ?? stored;
  const dirty =
    shown.body !== stored.body ||
    shown.title !== stored.title ||
    shown.phone !== stored.phone;
  useUnsavedGuard(open && dirty);
  // The preview shows the title and phone as typed, before they are saved.
  const rendered = useSignOff(
    "",
    "",
    draft === null ? undefined : { title: draft.title, phone: draft.phone },
  ).signOff;
  const change = (field: keyof Draft, value: string) =>
    setDraft({ ...shown, [field]: value });

  const answer = signature.isPending
    ? undefined
    : rowAnswer(t, templated, stored.body);

  const close = () => setOpen(false);
  const edit = () => {
    setDraft(null);
    save.reset();
    setOpen(true);
  };

  return (
    <>
      <SettingRow
        label={t("settings.signature")}
        description={t("settings.signatureSub")}
        value={answer}
        control={
          <Button variant="ghost" onClick={edit}>
            {t("settings.signatureEdit")}
          </Button>
        }
      />
      <Modal open={open} onClose={close} labelledBy={titleId} intent="form">
        <Heading size="large" className="t-h3 modal-title" id={titleId}>
          {t("settings.signature")}
        </Heading>
        {/* A real form: nothing is written until Save is pressed. */}
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (dirty && !save.isPending) save.mutate(shown);
          }}
        >
          <WriteRefused titleKey="settings.saveFailed" error={save.error} />
          {templated ? (
            <>
              <p className="t-caption">{t("settings.signatureTemplateHint")}</p>
              <Field label={t("settings.signatureTitle")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={shown.title}
                    onChange={(event) => change("title", event.target.value)}
                  />
                )}
              </Field>
              <Field label={t("settings.signaturePhone")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={shown.phone}
                    onChange={(event) => change("phone", event.target.value)}
                  />
                )}
              </Field>
              {rendered?.html ? (
                <SignatureHtml
                  html={rendered.html}
                  title={t("settings.signaturePreview")}
                />
              ) : null}
            </>
          ) : (
            <>
              <Field label={t("settings.signatureLabel")}>
                {(control) => (
                  <Textarea
                    {...control}
                    rows={5}
                    value={shown.body}
                    placeholder={t("settings.signaturePlaceholder")}
                    onChange={(event) => change("body", event.target.value)}
                  />
                )}
              </Field>
              <p className="t-caption">{t("settings.signatureHint")}</p>
            </>
          )}
        </form>
        <div className="actions">
          <Button variant="ghost" onClick={close}>
            {t("settings.signatureCancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            disabled={!save.isPending && !dirty}
            pending={save.isPending}
            busyLabel={t("settings.signatureSaving")}
          >
            {t("record.save")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

// The row's value: the first line identifies a sign-off. The caller claims
// nothing before the read has answered.
function rowAnswer(
  t: ReturnType<typeof useT>,
  templated: boolean,
  body: string,
): string {
  if (templated) {
    return t("settings.signatureFromTemplate");
  }
  const firstLine = body.split("\n")[0].trim();
  return firstLine === "" ? t("settings.signatureNone") : firstLine;
}
