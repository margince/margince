// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import {
  ContactLink,
  CopyEmail,
  EmailFace,
} from "../../design-system/contactlink";
import { InlineText } from "../../design-system/inlinetext";
import { ReachCard, ReachLine, reachRow } from "../../design-system/reachcard";
import type { DetailRow } from "../../design-system/recorddetails";
import { useT } from "../../i18n";
import type { Lead } from "./leads";

export function isBlank(value: string | null | undefined): boolean {
  return !value?.trim();
}

// The address as the door to writing to the lead; its pill opens the edit.
export function EmailValue({
  lead,
  canEdit,
  readOnlyReason,
  onSave,
}: Readonly<{
  lead: Lead;
  canEdit: boolean;
  readOnlyReason?: string;
  onSave: (next: string) => Promise<void>;
}>) {
  const t = useT();
  return (
    <InlineText
      label={t("create.email")}
      value={lead.email ?? ""}
      type="email"
      link={
        lead.email && (
          <span className="lead-row-email">
            <ContactLink
              kind="email"
              value={lead.email}
              record={{ entityType: "lead", entityId: lead.id }}
              readOnly={Boolean(lead.archived_at)}
              className="link-button lead-row-link"
              textClassName="lead-row-link"
            >
              <EmailFace address={lead.email} />
            </ContactLink>
            <CopyEmail address={lead.email} />
          </span>
        )
      }
      placeholder={t("field.unset")}
      canEdit={canEdit}
      readOnlyReason={readOnlyReason}
      onSave={onSave}
    />
  );
}

// The address and number share one card, as on a contact. One that is not
// set folds as its own empty row, so "Show empty fields" still offers it.
export function reachRows(
  lead: Lead,
  t: ReturnType<typeof useT>,
  values: Readonly<{ email: ReactNode; phone: ReactNode }>,
): DetailRow[] {
  const hasEmail = !isBlank(lead.email);
  const hasPhone = !isBlank(lead.phone);
  const empty = (key: "email" | "phone", value: ReactNode): DetailRow => ({
    key,
    term: t(key === "email" ? "create.email" : "create.phone"),
    empty: true,
    value,
  });
  return [
    ...(hasEmail || hasPhone
      ? [
          reachRow(
            t("record.reach.label"),
            <ReachCard>
              {hasEmail && (
                <ReachLine kind="email" text={lead.email ?? ""}>
                  {values.email}
                </ReachLine>
              )}
              {hasPhone && (
                <ReachLine kind="phone" text={lead.phone ?? ""}>
                  {values.phone}
                </ReachLine>
              )}
            </ReachCard>,
          ),
        ]
      : []),
    ...(hasEmail ? [] : [empty("email", values.email)]),
    ...(hasPhone ? [] : [empty("phone", values.phone)]),
  ];
}
