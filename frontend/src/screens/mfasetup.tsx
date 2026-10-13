// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useId, useMemo, useState } from "react";
import { encode } from "uqr";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemMessageOf, unwrap } from "./common";
import "./mfacard.css";

export type TotpEnrolment = components["schemas"]["TotpEnrolment"];

export type CodeRefusal = "wrong-code" | "too-many";

/** A refused code is an answer the form shows, not a failure to log. */
export function codeRefusalOf(status: number): CodeRefusal | null {
  if (status === 401) return "wrong-code";
  if (status === 429) return "too-many";
  return null;
}

const REFUSAL_COPY: Readonly<Record<CodeRefusal, MessageKey>> = {
  "wrong-code": "mfa.codeRefused",
  "too-many": "mfa.tooManyCodes",
};

export function refusalCopy(refusal: CodeRefusal | null | undefined) {
  return refusal ? REFUSAL_COPY[refusal] : null;
}

/** Whitespace only: an authenticator groups its digits, and no code holds a space. */
export function enteredCode(typed: string) {
  return typed.replace(/\s/g, "");
}

export function mfaFailureMessage(
  error: unknown,
  t: ReturnType<typeof useT>,
): string {
  return problemCodeOf(error) === "mfa_already_enrolled"
    ? t("mfa.alreadyOn")
    : problemMessageOf(error, t);
}

/** One field for an authenticator code or a recovery code, which is case-sensitive. */
export function MfaCodeField({
  label,
  hint,
  error,
  value,
  onChange,
}: Readonly<{
  label: string;
  hint?: string;
  error?: string;
  value: string;
  onChange: (value: string) => void;
}>) {
  return (
    <Field label={label} hint={hint} error={error} required>
      {(control) => (
        <TextInput
          {...control}
          name="one-time-code"
          autoComplete="one-time-code"
          inputMode="text"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}

function QrCode({ value, label }: Readonly<{ value: string; label: string }>) {
  const qr = useMemo(() => encode(value, { ecc: "M", border: 4 }), [value]);
  const modules = qr.data
    .flatMap((row, y) =>
      row.flatMap((dark, x) => (dark ? [`M${x} ${y}h1v1h-1z`] : [])),
    )
    .join("");
  return (
    <svg
      className="mfa-qr"
      role="img"
      aria-label={label}
      viewBox={`0 0 ${qr.size} ${qr.size}`}
      shapeRendering="crispEdges"
    >
      <rect className="mfa-qr-ground" width={qr.size} height={qr.size} />
      <path className="mfa-qr-ink" d={modules} />
    </svg>
  );
}

function SecretKey({ secret }: Readonly<{ secret: string }>) {
  const t = useT();
  const copy = useClipboardCopy(secret, {
    copy: t("mfa.copyKey"),
    copied: t("mfa.copied"),
    remedy: t("mfa.copyKeyFailed"),
  });
  return (
    <>
      <p>{t("mfa.secretHint")}</p>
      <div className="mfa-copy-row">
        <CopyableText text={secret} label={t("mfa.secretLabel")} />
        <Button onClick={copy.copy}>{copy.label}</Button>
      </div>
      {copy.notice}
    </>
  );
}

function RecoveryCodes({ codes }: Readonly<{ codes: readonly string[] }>) {
  const t = useT();
  const text = codes.join("\n");
  const copy = useClipboardCopy(text, {
    copy: t("mfa.copyCodes"),
    copied: t("mfa.copied"),
    remedy: t("mfa.copyCodesFailed"),
  });
  return (
    <div className="form-stack">
      <Callout kind="outcome" tone="success" title={t("mfa.onTitle")}>
        {t("mfa.onBody")}
      </Callout>
      <Callout kind="standing" tone="warning" title={t("mfa.saveCodesTitle")}>
        {t("mfa.saveCodes")}
      </Callout>
      <div className="mfa-copy-row">
        <CopyableText
          text={text}
          label={t("mfa.codesLabel")}
          testId="mfa-recovery-codes"
        />
        <Button onClick={copy.copy}>{copy.label}</Button>
      </div>
      {copy.notice}
    </div>
  );
}

function StartState({
  starting,
  startError,
  onRetry,
}: Readonly<{
  starting: boolean;
  startError: unknown;
  onRetry: () => void;
}>) {
  const t = useT();
  if (starting) return <p>{t("common.loading")}</p>;
  const enrolled = problemCodeOf(startError) === "mfa_already_enrolled";
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={t("mfa.startFailedTitle")}
      actions={
        enrolled ? undefined : (
          <Button onClick={onRetry}>{t("common.retry")}</Button>
        )
      }
    >
      {mfaFailureMessage(startError, t)}
    </Callout>
  );
}

/**
 * Setting up an authenticator: scan, confirm with a code, then the recovery
 * codes once. The secret and the codes never outlive the dialog.
 */
export function MfaSetupModal({
  open,
  enrolment,
  starting,
  startError,
  onRetry,
  onClose,
}: Readonly<{
  open: boolean;
  enrolment: TotpEnrolment | undefined;
  starting: boolean;
  startError: unknown;
  onRetry: () => void;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const [code, setCode] = useState("");
  const confirm = useMutation({
    gcTime: 0,
    mutationFn: async (typed: string) => {
      const result = await api.POST("/me/mfa/totp/confirm", {
        body: { code: typed },
      });
      const refusal = codeRefusalOf(result.response.status);
      if (refusal) return { refusal, codes: [] };
      return { refusal: null, codes: unwrap(result, t).recovery_codes };
    },
    onSuccess: (answer) => {
      if (answer.refusal === "wrong-code") setCode("");
    },
  });
  const codes = confirm.data?.refusal === null ? confirm.data.codes : null;
  const refused = refusalCopy(confirm.data?.refusal);
  const entered = enteredCode(code);

  const close = () => {
    confirm.reset();
    setCode("");
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={close}
      labelledBy={titleId}
      intent="form"
      closeDisabled={confirm.isPending}
    >
      <Heading size="large" className="t-h3 modal-title" id={titleId}>
        {t("mfa.setupTitle")}
      </Heading>
      {codes && <RecoveryCodes codes={codes} />}
      {!codes && !enrolment && (
        <StartState
          starting={starting}
          startError={startError}
          onRetry={onRetry}
        />
      )}
      {!codes && enrolment && (
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (entered !== "" && !confirm.isPending) confirm.mutate(entered);
          }}
        >
          <p>{t("mfa.scan")}</p>
          <QrCode value={enrolment.otpauth_uri} label={t("mfa.qrLabel")} />
          <SecretKey secret={enrolment.secret} />
          {confirm.isError && (
            <Callout
              kind="outcome"
              tone="danger"
              title={t("mfa.confirmFailedTitle")}
            >
              {mfaFailureMessage(confirm.error, t)}
            </Callout>
          )}
          <p>{t("mfa.signsOut")}</p>
          <MfaCodeField
            label={t("mfa.code")}
            error={refused ? t(refused) : undefined}
            value={code}
            onChange={setCode}
          />
        </form>
      )}
      <div className="actions">
        {codes ? (
          <Button variant="primary" onClick={close}>
            {t("mfa.done")}
          </Button>
        ) : (
          <>
            <Button
              variant="ghost"
              onClick={close}
              disabled={confirm.isPending}
            >
              {t("mfa.cancel")}
            </Button>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              disabled={!confirm.isPending && (!enrolment || entered === "")}
              pending={confirm.isPending}
              busyLabel={t("mfa.turningOn")}
            >
              {t("mfa.turnOn")}
            </Button>
          </>
        )}
      </div>
    </Modal>
  );
}
