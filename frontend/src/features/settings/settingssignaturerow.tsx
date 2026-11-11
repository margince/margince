import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, Modal, SegmentedControl } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { WriteRefused } from "../../design-system/querystates";
import { SettingRow } from "../../design-system/settingrow";
import type { Toast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useSignOff } from "../mail/composesignoff";
import { useSaveSignature } from "./settingssignature";

import {
  type SignatureDraft as Draft,
  type SignatureFormat as Format,
  SignatureFields,
} from "./settingssignatureeditor";

const FORMATS: readonly Format[] = ["text", "html"];

// Personal HTML takes precedence over the company template and plain text.
export function SignatureSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Draft | null>(null);
  const signature = useQuery({
    queryKey: ["me-email-signature"],
    queryFn: async () => {
      return unwrap(await api.GET("/me/email-signature"));
    },
  });
  const templated = signature.data?.template_active === true;
  const save = useSaveSignature((saved) => {
    // Hand the edit back to the server's answer: it trims what it stores.
    setDraft(
      saved
        ? {
            body: saved.body,
            title: saved.title,
            phone: saved.phone,
            html: saved.html,
            format: saved.html ? "html" : "text",
          }
        : null,
    );
    setOpen(false);
    toast.show(t("settings.saved"));
  });

  const stored: Draft = {
    body: signature.data?.body ?? "",
    title: signature.data?.title ?? "",
    phone: signature.data?.phone ?? "",
    html: signature.data?.html ?? "",
    format: signature.data?.html ? "html" : "text",
  };
  // The saved value until the member types; theirs from then on.
  const shown = draft ?? stored;
  const dirty =
    shown.body !== stored.body ||
    shown.title !== stored.title ||
    shown.phone !== stored.phone ||
    (shown.format === "html" ? shown.html : "") !== stored.html;
  useUnsavedGuard(open && dirty);
  // Preview uses the same draft fields the save submits.
  const rendered = useSignOff("", "", {
    title: shown.title,
    phone: shown.phone,
    html: shown.format === "html" ? shown.html : "",
  }).signOff;
  const change = (field: Exclude<keyof Draft, "format">, value: string) =>
    setDraft({ ...shown, [field]: value });

  const answer = rowAnswer(t, signature.isPending, templated, stored);

  const close = () => {
    if (!save.isPending) setOpen(false);
  };
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
          <Button variant="ghost" onClick={edit} disabled={save.isPending}>
            {t("settings.signatureEdit")}
          </Button>
        }
      />
      <Modal
        open={open}
        onClose={close}
        labelledBy={titleId}
        intent="form"
        unsaved={dirty && !save.isPending}
      >
        <Heading size="large" className="t-h2 modal-title" id={titleId}>
          {t("settings.signature")}
        </Heading>
        {/* A real form: nothing is written until Save is pressed. */}
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (dirty && !save.isPending)
              save.mutate({
                body: shown.body,
                title: shown.title,
                phone: shown.phone,
                html: shown.format === "html" ? shown.html : "",
              });
          }}
        >
          <WriteRefused titleKey="settings.saveFailed" error={save.error} />
          <SegmentedControl
            options={FORMATS}
            value={shown.format}
            onChange={(format) => {
              if (!save.isPending) setDraft({ ...shown, format });
            }}
            labels={{
              text: t("settings.signatureFormatText"),
              html: t("settings.signatureFormatHtml"),
            }}
            label={t("settings.signatureFormat")}
          />
          <SignatureFields
            shown={shown}
            mode={editorMode(shown.format, templated)}
            previewHtml={
              shown.format === "html" || templated ? rendered?.html : undefined
            }
            htmlHint={
              templated
                ? "settings.signatureHtmlHintTemplate"
                : "settings.signatureHtmlHintPlain"
            }
            disabled={save.isPending}
            change={change}
          />
        </form>
        <div className="actions">
          <Button variant="ghost" onClick={close} disabled={save.isPending}>
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
  pending: boolean,
  templated: boolean,
  stored: Draft,
): string | undefined {
  if (pending) return undefined;
  if (stored.html) return t("settings.signatureOwnHtml");
  if (templated) {
    return t("settings.signatureFromTemplate");
  }
  const firstLine = (stored.body.split("\n")[0] ?? "").trim();
  return firstLine === "" ? t("settings.signatureNone") : firstLine;
}

function editorMode(format: Format, templated: boolean) {
  if (format === "html") return "html";
  return templated ? "template" : "plain";
}
