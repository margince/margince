// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Checkbox, Field, TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { identifierNumber } from "../../format/format";
import { useT } from "../../i18n";

// Sending from an IMAP mailbox is opt-in: a tick on the connect form, and the
// outgoing server under it. The server checks the SMTP login before it stores
// the connection, so a mailbox never claims a sender it cannot be.

type SmtpSettings = components["schemas"]["ImapSmtpSettings"];
type Security = NonNullable<SmtpSettings["security"]>;

export type ImapSendDraft = Readonly<{
  on: boolean;
  host: string;
  port: string;
  security: Security;
}>;

export const READ_ONLY: ImapSendDraft = {
  on: false,
  host: "",
  port: "",
  security: "tls",
};

const DEFAULT_PORT: Readonly<Record<Security, number>> = {
  tls: 465,
  starttls: 587,
};

/** The SMTP port the draft names; blank is the security's default. */
export function smtpPort(draft: ImapSendDraft): number {
  return draft.port.trim() === ""
    ? DEFAULT_PORT[draft.security]
    : Number(draft.port);
}

/** The `smtp` member of the connect request, or none while sending is off.
 *  A blank host is left to the server, which uses the IMAP host. */
export function smtpRequest(draft: ImapSendDraft): SmtpSettings | undefined {
  if (!draft.on) return undefined;
  const host = draft.host.trim();
  return {
    ...(host === "" ? {} : { host }),
    port: smtpPort(draft),
    security: draft.security,
  };
}

/** The tick and, once it is on, the outgoing server's three settings. */
export function ImapSendFields({
  draft,
  onChange,
  imapHost,
  portError,
}: Readonly<{
  draft: ImapSendDraft;
  onChange: (next: ImapSendDraft) => void;
  /** Shown in the empty server field, which falls back to it. */
  imapHost: string;
  portError?: string;
}>) {
  const t = useT();
  return (
    <div className="imap-mailbox-span imap-send">
      <Checkbox
        checked={draft.on}
        onChange={(event) => onChange({ ...draft, on: event.target.checked })}
        label={
          <>
            {t("connectors.imapSend")}
            <span className="t-caption imap-send-hint">
              {t("connectors.imapSendHint")}
            </span>
          </>
        }
      />
      {draft.on && (
        <div className="imap-mailbox-form">
          <Field label={t("connectors.imapSmtpHost")}>
            {(control) => (
              <TextInput
                {...control}
                value={draft.host}
                placeholder={imapHost.trim()}
                onChange={(event) =>
                  onChange({ ...draft, host: event.target.value })
                }
              />
            )}
          </Field>
          <Field label={t("connectors.imapSmtpSecurity")}>
            {(control) => (
              <Select
                {...control}
                value={draft.security}
                options={[
                  { value: "tls", label: t("connectors.imapSmtpTls") },
                  {
                    value: "starttls",
                    label: t("connectors.imapSmtpStartTls"),
                  },
                ]}
                onChange={(value) =>
                  onChange({
                    ...draft,
                    security: value === "starttls" ? "starttls" : "tls",
                  })
                }
              />
            )}
          </Field>
          <Field label={t("connectors.imapSmtpPort")} error={portError}>
            {(control) => (
              <TextInput
                {...control}
                type="number"
                min={1}
                max={65535}
                value={draft.port}
                placeholder={identifierNumber(DEFAULT_PORT[draft.security])}
                onChange={(event) =>
                  onChange({ ...draft, port: event.target.value })
                }
              />
            )}
          </Field>
        </div>
      )}
    </div>
  );
}
