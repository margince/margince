// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FormEvent, ReactNode } from "react";
import type { components } from "../../api/schema";
import { Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { ServiceAccountKeyField } from "../../design-system/serviceaccountkeyfield";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf } from "../../lib/problem";

// How one vendor's credential is typed in and how its state reads. A vendor
// takes either a pasted API key or a service-account key file, and the row
// says which it holds.

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];
export type CredentialKind = ProviderStatus["credential_kind"];

// A server older than the kind field sends none, and every vendor it knows
// takes an API key.
export function credentialKindOf(status: ProviderStatus): CredentialKind {
  return status.credential_kind ?? "api_key";
}

// The field the secret is typed into: a password box for a pasted key, the
// key-file control for a service account.
export function KeyEntry({
  kind,
  configured,
  value,
  disabled,
  hint,
  refusal,
  onChange,
}: Readonly<{
  kind: CredentialKind;
  configured: boolean;
  value: string;
  disabled: boolean;
  hint: string;
  refusal: MessageKey | undefined;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  if (kind === "service_account") {
    return (
      <ServiceAccountKeyField
        value={value}
        disabled={disabled}
        hint={hint}
        error={refusal ? t(refusal) : undefined}
        onChange={onChange}
      />
    );
  }
  return (
    <Field
      label={t("aiProviderKeys.field")}
      hint={hint}
      error={refusal ? t(refusal) : undefined}
    >
      {(control) => (
        <TextInput
          {...control}
          // A password field: the browser does not offer to remember the
          // credential, and a screenshare does not show it.
          type="password"
          autoComplete="off"
          value={value}
          disabled={disabled}
          placeholder={
            configured
              ? t("aiProviderKeys.replacePlaceholder")
              : t("aiProviderKeys.addPlaceholder")
          }
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </Field>
  );
}

export function keyStateLabel(
  status: ProviderStatus,
  keyless: boolean,
  kind: CredentialKind,
): MessageKey {
  if (keyless) {
    return "aiProviderKeys.keyless";
  }
  if (status.configured) {
    return kind === "service_account"
      ? "aiProviderKeys.serviceAccountConfigured"
      : "aiProviderKeys.configured";
  }
  return status.optional ? "aiProviderKeys.optional" : "aiProviderKeys.absent";
}

// A held key, or none needed, is settled. An optional key not held is no gap —
// the adapter calls without one — so it only reports; a required key that is
// missing warns.
export function keyStateTone(
  status: ProviderStatus,
  keyless: boolean,
): "success" | "info" | "warning" {
  if (status.configured || keyless) {
    return "success";
  }
  return status.optional ? "info" : "warning";
}

export function credentialTitleKey(kind: CredentialKind): MessageKey {
  return kind === "service_account"
    ? "serviceAccountKey.label"
    : "aiProviderKeys.field";
}

export function SaveFailure({ failure }: Readonly<{ failure: Error | null }>) {
  const t = useT();
  if (!failure) return null;
  return (
    <Callout
      tone="danger"
      kind="outcome"
      title={t("aiProviderKeys.saveFailed")}
    >
      {problemMessageOf(failure, t)}
    </Callout>
  );
}

/**
 * The key field in the app's form dialog. `failure` shows inside it, because
 * the dialog covers the sheet that would otherwise show the refusal.
 */
export function KeyEntryDialog({
  open,
  kind,
  formId,
  configured,
  value,
  disabled,
  busy,
  hint,
  refusal,
  failure,
  onChange,
  onSubmit,
  onClose,
  actions,
}: Readonly<{
  open: boolean;
  kind: CredentialKind;
  formId: string;
  configured: boolean;
  value: string;
  disabled: boolean;
  busy: boolean;
  hint: string;
  refusal: MessageKey | undefined;
  failure: Error | null;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onClose: () => void;
  actions: ReactNode;
}>) {
  const t = useT();
  return (
    <RecordFormDialog
      open={open}
      title={t(credentialTitleKey(kind))}
      onClose={onClose}
      fieldCount={1}
      unsaved={value !== ""}
      closeDisabled={busy}
      form={
        <form id={formId} className="form-stack" noValidate onSubmit={onSubmit}>
          <KeyEntry
            kind={kind}
            configured={configured}
            value={value}
            disabled={disabled}
            hint={hint}
            refusal={refusal}
            onChange={onChange}
          />
          <SaveFailure failure={failure} />
        </form>
      }
      actions={actions}
    />
  );
}
