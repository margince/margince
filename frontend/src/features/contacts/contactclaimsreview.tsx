// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Disclosure } from "../../design-system/disclosure";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { DismissClaimButton } from "../records/claimdismiss";

type Claim = components["schemas"]["ConversationClaim"];

export function claimsForReview(claims: readonly Claim[]): readonly Claim[] {
  return claims.filter(
    (claim) => claim.needs_review && claim.status === "open",
  );
}

export function ContactClaimsReview({
  claims,
}: Readonly<{ claims: readonly Claim[] }>) {
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
                  formatDateTime(claim.occurred_at, locale, zone),
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
            <p>{t("commitment.quote", { quote: claim.source_quote })}</p>
            <DismissClaimButton id={claim.id} />
          </PanelRow>
        ))}
      </Disclosure>
    </PanelBody>
  );
}
