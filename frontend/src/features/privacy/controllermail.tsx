// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { EmailText } from "../../design-system/emailtext";
import { ErrorLine } from "../../design-system/errorline";
import { FactList } from "../../design-system/factlist";
import { Select } from "../../design-system/select";
import { localeNameKey, type Translator, useT } from "../../i18n";
import { problemMessageOf, problemStatus, unwrap } from "../../lib/problem";

type MailPreview = components["schemas"]["ControllerMailPreview"];
type MailLanguage = components["schemas"]["MailLanguage"];
export type ControllerMailKind = MailPreview["kind"];
export type ControllerMailIssued =
  components["schemas"]["ConfirmRequestIssued"];

const COPY = {
  privacy_notice: { title: "notice.sendTitle", send: "notice.sendConfirm" },
  record_confirmation: {
    title: "controllerMail.confirmTitle",
    send: "controllerMail.confirmSend",
  },
} as const;

// The exact mail the installation's relay would send, sent only on the confirm.
export function ControllerMailDialog({
  contactId,
  kind,
  open,
  onClose,
  onSent,
}: Readonly<{
  contactId: string;
  kind: ControllerMailKind;
  open: boolean;
  onClose: () => void;
  onSent: (issued: ControllerMailIssued) => void;
}>) {
  const t = useT();
  // Unset until the reader picks, so the first read asks for the server's default.
  const [language, setLanguage] = useState<MailLanguage | undefined>();
  const preview = useQuery({
    queryKey: ["controller-mail-preview", contactId, kind, language ?? null],
    enabled: open && contactId !== "",
    placeholderData: keepPreviousData,
    queryFn: async () =>
      unwrap(
        await api.GET("/contacts/{id}/consent/mail-preview", {
          params: { path: { id: contactId }, query: { kind, language } },
        }),
      ),
  });
  const send = useMutation({
    mutationKey: ["controller-mail-send", kind, contactId],
    mutationFn: async (vars: {
      contactId: string;
      kind: ControllerMailKind;
      language: MailLanguage;
    }) => {
      const route =
        vars.kind === "privacy_notice"
          ? "/contacts/{id}/consent/privacy-notice"
          : "/contacts/{id}/consent/confirm-request";
      return unwrap(
        await api.POST(route, {
          params: { path: { id: vars.contactId } },
          body: { language: vars.language },
        }),
      );
    },
    onSuccess: (issued) => {
      close();
      onSent(issued);
    },
  });
  function close() {
    setLanguage(undefined);
    send.reset();
    onClose();
  }

  // The mail on screen is the mail sent: never a language still loading.
  const shown = preview.isPlaceholderData ? undefined : preview.data;
  return (
    <ConfirmModal
      open={open}
      onClose={close}
      intent="drawer-reading"
      title={t(COPY[kind].title)}
      confirmLabel={t(COPY[kind].send)}
      onConfirm={() => {
        if (shown) send.mutate({ contactId, kind, language: shown.language });
      }}
      pending={send.isPending}
      confirmDisabled={!preview.isError && (!shown || preview.isFetching)}
      confirmReason={
        preview.isError ? t("controllerMail.noPreview") : undefined
      }
      error={send.isError ? problemMessageOf(send.error, t) : null}
    >
      <Callout title={t("controllerMail.relay")} />
      {preview.isError ? (
        <ErrorLine
          error={preview.error}
          actions={
            refusal(preview.error) ? undefined : (
              <Button variant="link" onClick={() => void preview.refetch()}>
                {t("common.retry")}
              </Button>
            )
          }
        />
      ) : preview.data ? (
        <MailPreviewBody
          preview={preview.data}
          busy={preview.isPlaceholderData}
          onLanguage={setLanguage}
          t={t}
        />
      ) : (
        <p role="status">{t("controllerMail.loading")}</p>
      )}
    </ConfirmModal>
  );
}

// A 4xx is the server's settled answer; asking again changes nothing.
function refusal(error: unknown): boolean {
  const status = problemStatus(error);
  return status >= 400 && status < 500;
}

function MailPreviewBody({
  preview,
  busy,
  onLanguage,
  t,
}: Readonly<{
  preview: MailPreview;
  busy: boolean;
  onLanguage: (language: MailLanguage) => void;
  t: Translator;
}>) {
  return (
    <>
      <Field
        label={t("controllerMail.language")}
        hint={t("controllerMail.defaultLanguage", {
          language: t(localeNameKey(preview.default_language)),
        })}
      >
        {(control) => (
          <Select
            {...control}
            value={preview.language}
            aria-busy={busy}
            // Each name is in its own language, so each option carries `lang`.
            options={preview.languages.map((language) => ({
              value: language,
              label: t(localeNameKey(language)),
              lang: language,
            }))}
            onChange={(next) => {
              const picked = preview.languages.find((one) => one === next);
              if (picked) onLanguage(picked);
            }}
          />
        )}
      </Field>
      <div aria-busy={busy}>
        <FactList
          facts={[
            {
              key: "from",
              term: t("email.detail.from"),
              value: preview.from_address ?? t("controllerMail.noSender"),
            },
            {
              key: "to",
              term: t("email.detail.to"),
              value: preview.to_address,
            },
            {
              key: "subject",
              term: t("compose.subject"),
              value: preview.subject,
            },
          ]}
        />
        <EmailText body={preview.text} />
      </div>
    </>
  );
}
