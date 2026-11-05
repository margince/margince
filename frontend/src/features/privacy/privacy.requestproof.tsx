// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Field, TextInput } from "../../design-system/atoms";
import { Select, type SelectOption } from "../../design-system/select";
import { useT } from "../../i18n";
import { isOption } from "../../lib/options";
import {
  type DsrClosing,
  IDENTITY_CHECK_LABEL,
  IDENTITY_CHECKS,
} from "../../lib/privacy.logic";

// What the proof of a closed request records beside the answer.
export function DsrProofFields({
  closing,
  onDraft,
}: Readonly<{
  closing: DsrClosing;
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
