// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import {
  ContactLink,
  CopyEmail,
  EmailFace,
} from "../../design-system/contactlink";
import { ReachCard, ReachLine, reachRow } from "../../design-system/reachcard";
import type { DetailRow } from "../../design-system/recorddetails";
import type { useT } from "../../i18n";
import type { Lead } from "./leads";

export function isBlank(value: string | null | undefined): boolean {
  return !value?.trim();
}

// The address as the door to writing to the lead.
export function EmailValue({ lead }: Readonly<{ lead: Lead }>) {
  return (
    <span className="lead-row-email">
      <ContactLink
        kind="email"
        value={lead.email ?? ""}
        record={{ entityType: "lead", entityId: lead.id }}
        readOnly={Boolean(lead.archived_at)}
        className="link-button lead-row-link"
        textClassName="lead-row-link"
      >
        <EmailFace address={lead.email ?? ""} />
      </ContactLink>
      <CopyEmail address={lead.email ?? ""} />
    </span>
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
  const empty = (key: "email" | "phone"): DetailRow => ({
    key,
    term: t(key === "email" ? "create.email" : "create.phone"),
    empty: true,
    value: t("field.unset"),
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
    ...(hasEmail ? [] : [empty("email")]),
    ...(hasPhone ? [] : [empty("phone")]),
  ];
}
