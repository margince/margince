// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The messages behind a claim, listed as citations.
//
// The verdict panel and the map's edge detail both cite receipts, and a
// citation drawn two ways is two answers to "which message was that": one
// list, so a mail is the same reference on both.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { EmailReference } from "../../design-system/emailreference";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";

type Receipt = components["schemas"]["ContactGraphReceipt"];

/**
 * ReceiptList names each message a count was read from.
 *
 * A mail takes the product's one email citation. The graph also counts
 * attendees and organizers, so a receipt can be a meeting or a call — those
 * keep the plain line, because an email's icon on a meeting would tell a
 * reader it was a mail.
 */
export function ReceiptList({
  receipts,
  onOpenEmail,
}: Readonly<{
  receipts: readonly Receipt[];
  /**
   * Opens one of these messages in the contact page's email drawer.
   *
   * No withheld flag travels with it: a graph receipt is already checked row by
   * row before it reaches this list, so a row that is here is one this reader
   * may open. Passing a withheld state the shape does not carry would be
   * inventing an answer to a question already settled upstream.
   */
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <ul className="pn-receipts">
      {receipts.map((r) => (
        <li key={r.activity_id}>
          {r.kind === "email" ? (
            <EmailReference
              subject={r.subject}
              occurredAt={formatDate(r.occurred_at, locale, recordZone)}
              onOpen={
                onOpenEmail ? () => onOpenEmail(r.activity_id) : undefined
              }
            />
          ) : (
            <>
              {r.subject ?? t("contact.graph.untitledMessage")} ·{" "}
              {formatDate(r.occurred_at, locale, recordZone)}
            </>
          )}
        </li>
      ))}
    </ul>
  );
}
