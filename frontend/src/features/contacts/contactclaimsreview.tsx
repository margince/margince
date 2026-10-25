// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Disclosure } from "../../design-system/disclosure";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";

type Claim = components["schemas"]["ConversationClaim"];

export function claimsForReview(claims: readonly Claim[]): readonly Claim[] {
  return claims.filter(
    (claim) => claim.needs_review && claim.status === "open",
  );
}

// The dismiss control is handed in: it belongs to another feature, and a
// feature imports no other feature.
export function ContactClaimsReview({
  claims,
  dismiss,
}: Readonly<{
  claims: readonly Claim[];
  dismiss: (claimId: string) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const review = claimsForReview(claims);
  if (review.length === 0) return null;
  return (
    <PanelBody>
      <Disclosure summary={t("contact.loops.review")}>
        {review.map((claim) => (
          <PanelRow key={claim.id}>
            <p>{claim.body}</p>
            <p className="t-caption">
              {[
                claim.source_label,
                claim.occurred_at &&
                  formatActivityWhen(
                    claim.occurred_at,
                    claim.occurred_precision,
                    locale,
                    { record: zone, viewer: zone },
                  ),
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
            <p>{t("commitment.quote", { quote: claim.source_quote })}</p>
            {dismiss(claim.id)}
          </PanelRow>
        ))}
      </Disclosure>
    </PanelBody>
  );
}
