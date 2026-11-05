// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { Badge } from "../../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../../design-system/overviewcard";
import { formatDateAbbrev } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useCan } from "../../../lib/capability";
import { unwrap } from "../../../lib/problem";
import { useRecordZone } from "../../../lib/recordzone";

type DealViability = components["schemas"]["DealViability"];

const QUALIFICATION_TONE = {
  eligible: "success",
  excluded: "danger",
  unsupported: "warning",
  unknown: "default",
  closed: "default",
} as const;

export function dealViabilityKey(dealId: string) {
  return ["deal-viability", dealId] as const;
}

/**
 * DealViabilityCard says whether the customer still supports the deal, as the
 * qualified forecast counts it, and shows the open review. The review stays
 * while it is open, even after its task is done: a done task is not an answer.
 */
export function DealViabilityCard({ dealId }: Readonly<{ dealId: string }>) {
  const canRead = useCan("forecast", "read");
  const viability = useQuery({
    queryKey: dealViabilityKey(dealId),
    enabled: canRead,
    queryFn: async ({ signal }): Promise<DealViability> =>
      unwrap(
        await api.GET("/deals/{id}/viability", {
          params: { path: { id: dealId } },
          signal,
        }),
      ),
  });
  if (!canRead || !viability.data) {
    return null;
  }
  return <DealViabilityView viability={viability.data} />;
}

/** DealViabilityView draws one deal's viability as the card shows it. */
export function DealViabilityView({
  viability: data,
}: Readonly<{ viability: DealViability }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const review = data.review;
  return (
    <OverviewCard title={t("deals.viability.title")}>
      <OverviewList>
        <OverviewRow
          title={t("deals.viability.standing")}
          sub={
            data.qualification_mode === "enforcing"
              ? t("deals.viability.enforced")
              : t("deals.viability.notEnforced")
          }
          right={
            <Badge tone={QUALIFICATION_TONE[data.qualification]}>
              {t(`deals.viability.qualification.${data.qualification}`)}
            </Badge>
          }
        />
        {data.assessed_at && (
          <OverviewRow
            title={t("deals.viability.assessed")}
            // The forecast stops taking a check once it lapses, its policy
            // changes or the deal's evidence moves; the deal is then unknown.
            sub={
              data.receipt && !data.receipt.current
                ? t("deals.viability.checkOutOfDate")
                : undefined
            }
            right={formatDateAbbrev(data.assessed_at, locale, zone)}
          />
        )}
        {review && (
          <OverviewRow
            title={t("deals.viability.review")}
            sub={t("deals.viability.reviewOpenedOn", {
              date: formatDateAbbrev(review.opened_at, locale, zone),
            })}
            right={
              <Badge tone="warning">
                {review.task?.done
                  ? t("deals.viability.taskDoneReviewOpen")
                  : t("deals.viability.reviewOpen")}
              </Badge>
            }
          />
        )}
      </OverviewList>
    </OverviewCard>
  );
}
