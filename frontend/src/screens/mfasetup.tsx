// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useId, useMemo, useState } from "react";
import { encode } from "uqr";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemMessageOf, unwrap } from "../lib/problem";
import { enteredCode, MfaCodeField } from "./mfacode";
import "./mfasetup.css";

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

export function mfaFailureMessage(
  error: unknown,
  t: ReturnType<typeof useT>,
): string {
  return problemCodeOf(error) === "mfa_already_enrolled"
    ? t("mfa.alreadyOn")
    : problemMessageOf(error, t);
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

function CopyRow({
  text,
  label,
  labels,
  testId,
}: Readonly<{
  text: string;
  label: string;
  labels: Parameters<typeof useClipboardCopy>[1];
  testId?: string;
}>) {
  const copy = useClipboardCopy(text, labels);
  return (
    <>
      <div className="mfa-copy-row">
        <CopyableText text={text} label={label} testId={testId} />
        <Button onClick={copy.copy}>{copy.label}</Button>
      </div>
      {copy.notice}
    </>
  );
}

function SecretKey({ secret }: Readonly<{ secret: string }>) {
  const t = useT();
  return (
    <>
      <p>{t("mfa.secretHint")}</p>
      <CopyRow
        text={secret}
        label={t("mfa.secretLabel")}
        labels={{
          copy: t("mfa.copyKey"),
          copied: t("mfa.copied"),
          remedy: t("mfa.copyKeyFailed"),
        }}
      />
    </>
  );
}

function RecoveryCodes({ codes }: Readonly<{ codes: readonly string[] }>) {
  const t = useT();
  return (
    <div className="form-stack">
      <Callout kind="outcome" tone="success" title={t("mfa.onTitle")}>
        {t("mfa.onBody")}
      </Callout>
      <Callout kind="standing" tone="warning" title={t("mfa.saveCodesTitle")}>
        {t("mfa.saveCodes")}
      </Callout>
      <CopyRow
        text={codes.join("\n")}
        label={t("mfa.codesLabel")}
        testId="mfa-recovery-codes"
        labels={{
          copy: t("mfa.copyCodes"),
          copied: t("mfa.copied"),
          remedy: t("mfa.copyCodesFailed"),
        }}
      />
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

/** Starts or restarts an enrolment; the secret it returns never reaches a cache. */
export function useTotpStart() {
  const t = useT();
  return useMutation({
    gcTime: 0,
    mutationFn: async () => unwrap(await api.POST("/me/mfa/totp"), t),
  });
}

export function useTotpConfirm() {
  const t = useT();
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
  const entered = enteredCode(code);
  return {
    confirm,
    code,
    setCode,
    entered,
    codes: confirm.data?.refusal === null ? confirm.data.codes : null,
    submit: () => {
      if (entered !== "" && !confirm.isPending) confirm.mutate(entered);
    },
    reset: () => {
      confirm.reset();
      setCode("");
    },
  };
}

export type TotpConfirm = ReturnType<typeof useTotpConfirm>;

/** The steps of a setup, shared by the Account dialog and the forced screen. */
export function MfaSetupSteps({
  flow,
  formId,
  enrolment,
  starting,
  startError,
  onRetry,
}: Readonly<{
  flow: TotpConfirm;
  formId: string;
  enrolment: TotpEnrolment | undefined;
  starting: boolean;
  startError: unknown;
  onRetry: () => void;
}>) {
  const t = useT();
  const refused = refusalCopy(flow.confirm.data?.refusal);
  if (flow.codes) return <RecoveryCodes codes={flow.codes} />;
  if (!enrolment) {
    return (
      <StartState
        starting={starting}
        startError={startError}
        onRetry={onRetry}
      />
    );
  }
  return (
    <form
      id={formId}
      className="form-stack"
      onSubmit={(event) => {
        event.preventDefault();
        flow.submit();
      }}
    >
      <p>{t("mfa.scan")}</p>
      <QrCode value={enrolment.otpauth_uri} label={t("mfa.qrLabel")} />
      <SecretKey secret={enrolment.secret} />
      {flow.confirm.isError && (
        <Callout
          kind="outcome"
          tone="danger"
          title={t("mfa.confirmFailedTitle")}
        >
          {mfaFailureMessage(flow.confirm.error, t)}
        </Callout>
      )}
      <p>{t("mfa.signsOut")}</p>
      <MfaCodeField
        label={t("mfa.code")}
        error={refused ? t(refused) : undefined}
        value={flow.code}
        onChange={flow.setCode}
      />
    </form>
  );
}

/** Turn on until the codes show, then Done; `lead` goes before Turn on. */
export function MfaSetupActions({
  flow,
  formId,
  enrolment,
  onDone,
  lead,
  className = "actions",
}: Readonly<{
  flow: TotpConfirm;
  formId: string;
  enrolment: TotpEnrolment | undefined;
  onDone: () => void;
  lead?: ReactNode;
  className?: string;
}>) {
  const t = useT();
  const pending = flow.confirm.isPending;
  return (
    <div className={className}>
      {flow.codes ? (
        <Button variant="primary" onClick={onDone}>
          {t("mfa.done")}
        </Button>
      ) : (
        <>
          {lead}
          <Button
            type="submit"
            form={formId}
            variant="primary"
            disabled={!pending && (!enrolment || flow.entered === "")}
            pending={pending}
            busyLabel={t("mfa.turningOn")}
          >
            {t("mfa.turnOn")}
          </Button>
        </>
      )}
    </div>
  );
}

/**
 * Setting up an authenticator from the Account page. The secret and the codes
 * never outlive the dialog.
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
  const flow = useTotpConfirm();
  const close = () => {
    flow.reset();
    onClose();
  };
  // A pending confirm still enrols on the server, and its codes exist nowhere else.
  const dismiss = () => {
    if (!flow.confirm.isPending && !flow.codes) close();
  };

  return (
    <Modal
      open={open}
      onClose={dismiss}
      labelledBy={titleId}
      intent="form"
      closeDisabled={flow.confirm.isPending}
      closeReason={flow.codes ? t("mfa.keepCodes") : undefined}
    >
      <Heading size="large" className="t-h3 modal-title" id={titleId}>
        {t("mfa.setupTitle")}
      </Heading>
      <MfaSetupSteps
        flow={flow}
        formId={formId}
        enrolment={enrolment}
        starting={starting}
        startError={startError}
        onRetry={onRetry}
      />
      <MfaSetupActions
        flow={flow}
        formId={formId}
        enrolment={enrolment}
        onDone={close}
        lead={
          <Button
            variant="ghost"
            onClick={close}
            disabled={flow.confirm.isPending}
          >
            {t("mfa.cancel")}
          </Button>
        }
      />
    </Modal>
  );
}
