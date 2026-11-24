// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Field, TextInput } from "../../design-system/atoms";
import { Select, type SelectOption } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { emailFault } from "../../lib/emailshape";
import { isOption } from "../../lib/options";
import {
  type DsrClosing,
  type DsrStatus,
  IDENTITY_CHECK_LABEL,
  IDENTITY_CHECKS,
  isTerminal,
  nextStatuses,
} from "../../lib/privacy.logic";
import { refusalSentences } from "../../lib/problem";

export const REQUESTER_EMAIL = "requester_email";

// A close's own check of the requester's address, and a refusal of the close
// split between that field and the drawer's foot.
export function useRequesterEmail() {
  const t = useT();
  const { locale } = useLocale();
  const label = t("privacy.requesterEmail");
  return {
    check: (address: string): ReadonlyMap<string, string> => {
      const fault = emailFault(address, locale, label);
      return fault ? new Map([[REQUESTER_EMAIL, fault]]) : new Map();
    },
    refused: (error: unknown) =>
      refusalSentences(error, t, { [REQUESTER_EMAIL]: label }),
  };
}

// What the proof of a closed request records beside the answer.
export function DsrProofFields({
  closing,
  emailError,
  onDraft,
}: Readonly<{
  closing: DsrClosing;
  emailError?: string;
  onDraft: (change: Partial<DsrClosing>) => void;
}>) {
  const t = useT();
  const options: SelectOption[] = [
    { value: "", label: t("privacy.identityCheckPick"), disabled: true },
    ...IDENTITY_CHECKS.map((check) => ({
      value: check,
      label: t(IDENTITY_CHECK_LABEL[check]),
    })),
  ];
  return (
    <>
      <Field
        label={t("privacy.identityCheck")}
        hint={t("privacy.identityCheckHint")}
      >
        {(control) => (
          <Select
            {...control}
            options={options}
            value={closing.identityCheck}
            onChange={(next) => {
              if (isOption(next, IDENTITY_CHECKS)) {
                onDraft({ identityCheck: next });
              }
            }}
          />
        )}
      </Field>
      <Field
        label={t("privacy.requesterEmail")}
        hint={t("privacy.requesterEmailHint")}
        error={emailError}
      >
        {(control) => (
          <TextInput
            {...control}
            type="email"
            value={closing.requesterEmail}
            onChange={(event) =>
              onDraft({ requesterEmail: event.target.value })
            }
          />
        )}
      </Field>
    </>
  );
}

// A complaint holds the closed request's proof past its retention date.
export function DsrDispute({
  open,
  pending,
  onChange,
}: Readonly<{
  open: boolean;
  pending: boolean;
  onChange: (next: boolean) => void;
}>) {
  const t = useT();
  return (
    <div className="form-stack">
      <p className="t-caption">{t("privacy.disputeHint")}</p>
      <div className="dsr-actions">
        <Button disabled={pending} onClick={() => onChange(!open)}>
          {open ? t("privacy.closeDispute") : t("privacy.openDispute")}
        </Button>
      </div>
    </div>
  );
}

// nextStatuses never routes back to open, so three verbs cover every move.
function transitionLabelKey(status: DsrStatus): MessageKey {
  if (status === "in_progress") return "privacy.inProgress";
  if (status === "fulfilled") return "privacy.fulfil";
  return "privacy.reject";
}

// A close needs its answer and its identity check (closingReady).
export function DsrTransitions({
  status,
  ready,
  pending,
  onTransition,
}: Readonly<{
  status: DsrStatus;
  ready: (next: DsrStatus) => boolean;
  pending: boolean;
  onTransition: (next: DsrStatus) => void;
}>) {
  const t = useT();
  return (
    <div className="dsr-actions">
      {nextStatuses(status).map((next) => (
        <Button
          key={next}
          disabled={pending || (isTerminal(next) && !ready(next))}
          onClick={() => onTransition(next)}
        >
          {t(transitionLabelKey(next))}
        </Button>
      ))}
    </div>
  );
}
