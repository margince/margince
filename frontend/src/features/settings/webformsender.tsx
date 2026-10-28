// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ClipboardList, Globe, Server } from "lucide-react";
import type { ReactNode } from "react";
import { Field, TextInput } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { ProviderMark } from "../../design-system/provider-mark";
import { useT } from "../../i18n";
import { needsDeveloper, type WebFormSender } from "./webformkinds";

// The vendor's own mark where the design system carries one. Tally publishes
// none under a licence we can ship, so it takes a glyph like the two kinds.
const SENDER_MARKS: Readonly<Record<WebFormSender, ReactNode>> = {
  browser: <Globe aria-hidden="true" />,
  typeform: <ProviderMark providerKey="typeform" />,
  tally: <ClipboardList aria-hidden="true" />,
  server: <Server aria-hidden="true" />,
  webflow: <ProviderMark providerKey="webflow" />,
};

// Where the form lives: fixed once it exists, so an edit only names it.
export function SenderField({
  sender,
  editing,
  senders,
  error,
  onChange,
}: Readonly<{
  sender: WebFormSender;
  editing: boolean;
  senders: readonly WebFormSender[];
  error?: string;
  onChange: (sender: WebFormSender) => void;
}>) {
  const t = useT();
  if (editing) {
    return (
      <Field
        label={t("webForms.field.sender")}
        hint={t("webForms.field.senderFixed")}
      >
        {(field) => (
          <TextInput
            {...field}
            value={t(`webForms.sender.${sender}`)}
            disabled
          />
        )}
      </Field>
    );
  }
  // Each tile carries one short line; the full sentence is said once, for
  // the chosen answer, under the tiles.
  return (
    <div className="form-stack">
      <ChoiceList
        legend={t("webForms.field.sender")}
        layout="tiles"
        value={sender}
        choices={senders.map((choice) => ({
          value: choice,
          label: t(`webForms.sender.${choice}`),
          description: t(`webForms.senderLine.${choice}`),
          mark: SENDER_MARKS[choice],
          group: needsDeveloper(choice)
            ? t("webForms.senderGroup.developer")
            : t("webForms.senderGroup.noDeveloper"),
        }))}
        onChange={onChange}
      />
      <p className="webform-sender-detail" aria-live="polite">
        {t(`webForms.senderHint.${sender}`)}
      </p>
      {error && <ErrorLine>{error}</ErrorLine>}
    </div>
  );
}
