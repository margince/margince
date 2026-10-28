// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ClipboardList,
  Globe,
  type LucideIcon,
  MessageSquareText,
  Server,
  Webhook,
} from "lucide-react";
import { Field, TextInput } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { needsDeveloper, type WebFormSender } from "./webformkinds";

// A glyph per kind of form, not the vendors' logos. The label names the
// vendor; the mark only lets the eye find a plate again.
const SENDER_MARKS: Readonly<Record<WebFormSender, LucideIcon>> = {
  browser: Globe,
  typeform: MessageSquareText,
  tally: ClipboardList,
  server: Server,
  webflow: Webhook,
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
  return (
    <>
      <ChoiceList
        legend={t("webForms.field.sender")}
        layout="cards"
        value={sender}
        choices={senders.map((choice) => {
          const Mark = SENDER_MARKS[choice];
          return {
            value: choice,
            label: t(`webForms.sender.${choice}`),
            description: t(`webForms.senderHint.${choice}`),
            mark: <Mark aria-hidden="true" />,
            group: needsDeveloper(choice)
              ? t("webForms.senderGroup.developer")
              : t("webForms.senderGroup.noDeveloper"),
          };
        })}
        onChange={onChange}
      />
      {error && <ErrorLine>{error}</ErrorLine>}
    </>
  );
}
