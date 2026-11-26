// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, unwrap } from "../../lib/problem";

type GmailSignature = components["schemas"]["GmailSignatureImport"];

// Why nothing was taken, for every answer but a ready signature.
const NOT_TAKEN: Record<
  Exclude<GmailSignature["state"], "ready">,
  MessageKey
> = {
  empty: "settings.signatureGmailEmpty",
  no_mailbox: "settings.signatureGmailNoMailbox",
  needs_reconnect: "settings.signatureGmailReconnect",
  too_long: "settings.signatureGmailTooLong",
};

/**
 * "Use my Gmail signature": reads the signature on the press and hands it to
 * the open draft, where the preview shows what is kept. Nothing is saved
 * until the reader presses Save.
 */
export function GmailSignatureImport({
  disabled,
  onImported,
}: Readonly<{
  disabled: boolean;
  onImported: (html: string, text: string) => void;
}>) {
  const t = useT();
  const read = useMutation({
    mutationFn: async () => unwrap(await api.GET("/me/email-signature/gmail")),
    onSuccess: (answer) => {
      if (answer.state === "ready" && answer.html) {
        onImported(answer.html, answer.text ?? "");
      }
    },
  });
  const answer = read.data;
  return (
    <>
      <Button
        variant="ghost"
        onClick={() => read.mutate()}
        disabled={disabled}
        pending={read.isPending}
        data-testid="signature-use-gmail"
      >
        {t("settings.signatureUseGmail")}
      </Button>
      {answer?.state === "ready" && (
        <Callout
          tone="success"
          kind="outcome"
          live="status"
          title={t("settings.signatureGmailTaken", {
            address: answer.address ?? "",
          })}
        />
      )}
      {answer && answer.state !== "ready" && (
        <Callout
          tone="warning"
          kind="outcome"
          live="status"
          title={t(labelFor(NOT_TAKEN, answer.state))}
        />
      )}
      {read.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          live="status"
          title={problemMessageOf(read.error, t)}
        />
      )}
    </>
  );
}
