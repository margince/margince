// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useT } from "../i18n";
import { EmailReference } from "./emailreference";
import "./activityreferencelist.css";

// The activities a derived number was computed from, as rows a reader can check.
//
// A count of receipts is not a receipt. "Computed from 12 activities" is a claim
// a reader cannot check: they cannot tell whether it counts the exchange they
// remember, and they cannot open any of it. This is the same list with each row
// named.
//
// An email takes the product's one email citation, so a receipt behind a score
// and a citation under a brief lead to the same drawer. Every other kind is
// prose: a note and a call have no page of their own, and a row that looked
// pressable and opened nothing would teach a reader that the list does not work.
//
// A row this reader may know about but not read says so in the same words the
// message components use, and offers nothing to press.

type ActivityReference = components["schemas"]["ActivityReference"];

export function ActivityReferenceList({
  references,
  onOpenEmail,
  formatWhen,
}: Readonly<{
  references: readonly ActivityReference[];
  /**
   * Opens one email in the host's own drawer. A host that mounts none passes
   * nothing, and the row renders as a citation with no opener — the same
   * arrangement every other surface that cites mail uses.
   */
  onOpenEmail?: (activityId: string) => void;
  /** Formatted by the caller, which owns the reader's timezone. */
  formatWhen: (occurredAt: string) => string;
}>) {
  const t = useT();
  if (references.length === 0) {
    return null;
  }
  return (
    <ul className="activityrefs">
      {references.map((reference) => {
        const withheld = reference.content_state === "withheld";
        const when = formatWhen(reference.occurred_at);
        return (
          <li key={reference.activity_id} className="activityrefs__row">
            {reference.kind === "email" ? (
              <EmailReference
                subject={reference.subject}
                occurredAt={when}
                withheld={withheld}
                onOpen={
                  onOpenEmail
                    ? () => onOpenEmail(reference.activity_id)
                    : undefined
                }
              />
            ) : (
              <span className="activityrefs__other">
                <span className="activityrefs__kind">
                  {t(`timeline.kind.${reference.kind}`)}
                </span>
                {/* Withheld first: the subject is null in that case anyway,
                    and saying WHY is what separates a limited exchange from
                    one that carried no subject of its own. */}
                <span className="activityrefs__subject">
                  {withheld
                    ? t("email.withheldSubject")
                    : (reference.subject?.trim() ?? "")}
                </span>
                <span className="activityrefs__when t-mono">{when}</span>
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
