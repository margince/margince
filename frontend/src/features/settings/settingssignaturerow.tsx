import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { SignatureHtml } from "../../components/signaturehtml";
import { useUnsavedGuard } from "../../components/unsaved";
import {
  Button,
  Field,
  Modal,
  SegmentedControl,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { WriteRefused } from "../../design-system/querystates";
import { SettingRow } from "../../design-system/settingrow";
import type { Toast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useSignOff } from "../mail/composesignoff";
import { useSaveSignature } from "./settingssignature";

type Format = "text" | "html";
const FORMATS: readonly Format[] = ["text", "html"];
type Draft = {
  body: string;
  title: string;
  phone: string;
  html: string;
  format: Format;
};

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

  const answer = signature.isPending
    ? undefined
    : stored.html
      ? t("settings.signatureOwnHtml")
      : rowAnswer(t, templated, stored.body);

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
            templated={templated}
            rendered={rendered}
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

function SignatureFields({
  shown,
  templated,
  rendered,
  change,
  disabled,
}: Readonly<{
  shown: Draft;
  templated: boolean;
  disabled: boolean;
  rendered: ReturnType<typeof useSignOff>["signOff"];
  change: (field: Exclude<keyof Draft, "format">, value: string) => void;
}>) {
  const t = useT();
  return (
    <>
      {shown.format === "html" ? (
        <>
          <Field label={t("settings.signatureHtmlLabel")}>
            {(control) => (
              <Textarea
                {...control}
                disabled={disabled}
                rows={8}
                value={shown.html}
                onChange={(event) => change("html", event.target.value)}
              />
            )}
          </Field>
          <p className="t-caption">
            {t(
              templated
                ? "settings.signatureHtmlHintTemplate"
                : "settings.signatureHtmlHintPlain",
            )}
          </p>
          {rendered?.html ? (
            <SignatureHtml
              html={rendered.html}
              title={t("settings.signaturePreview")}
            />
          ) : null}
        </>
      ) : templated ? (
        <>
          <p className="t-caption">{t("settings.signatureTemplateHint")}</p>
          <Field label={t("settings.signatureTitle")}>
            {(control) => (
              <TextInput
                {...control}
                disabled={disabled}
                value={shown.title}
                onChange={(event) => change("title", event.target.value)}
              />
            )}
          </Field>
          <Field label={t("settings.signaturePhone")}>
            {(control) => (
              <TextInput
                {...control}
                disabled={disabled}
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
                disabled={disabled}
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
  const firstLine = (body.split("\n")[0] ?? "").trim();
  return firstLine === "" ? t("settings.signatureNone") : firstLine;
}
