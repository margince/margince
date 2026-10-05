// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Field, TextInput } from "../design-system/atoms";
import { ServiceAccountKeyField } from "../design-system/serviceaccountkeyfield";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

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
// key-file control for a service account. Save and Remove ride along.
export function KeyEntry({
  kind,
  configured,
  value,
  disabled,
  hint,
  refusal,
  onChange,
  verbs,
}: Readonly<{
  kind: CredentialKind;
  configured: boolean;
  value: string;
  disabled: boolean;
  hint: string;
  refusal: MessageKey | undefined;
  onChange: (value: string) => void;
  verbs: ReactNode;
}>) {
  const t = useT();
  if (kind === "service_account") {
    // One column with one rhythm: the field, its file box and the verbs each
    // carry their own label or hint, and stacked bare they ran into each other.
    return (
      <div className="ai-key-stack">
        <ServiceAccountKeyField
          value={value}
          disabled={disabled}
          hint={hint}
          error={refusal ? t(refusal) : undefined}
          onChange={onChange}
        />
        <div className="ai-key-verbs">{verbs}</div>
      </div>
    );
  }
  return (
    <Field label={t("aiProviderKeys.field")} hint={hint}>
      {/* One paste and the verbs that act on it, on one line. */}
      {(control) => (
        <div className="ai-key-entry">
          <TextInput
            {...control}
            // A password field, so the browser does not offer to remember
            // a credential this app deliberately never stores
            // client-side, and so a screenshare does not carry it.
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
          {verbs}
        </div>
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
