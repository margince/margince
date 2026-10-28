// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { DismissClaimButton } from "../../../components/claimdismiss";
import { useClaimSettle } from "../../../components/taskactions";
import { Badge, Button } from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
  OverviewText,
} from "../../../design-system/overviewcard";
import { formatDate, formatMoneyWhole } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { DEAL_COMMITMENTS_KEY } from "../../../lib/activitykeys";
import { useCanWrite } from "../../../lib/capability";
import { problemStatus, unwrap } from "../../../lib/problem";
import { useRecordZone } from "../../../lib/recordzone";

type DealCommitments = components["schemas"]["DealCommitments"];
type DealCommitment = DealCommitments["data"][number];

/**
 * What the customer committed to in messages filed on this deal. Whether a
 * promise is overdue, and whether it names another price than the deal, is
 * the server's verdict; the card only draws it.
 */
export function DealCommitmentsCard({
  dealId,
  onOpenEmail,
}: Readonly<{
  dealId: string;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const query = useQuery({
    queryKey: [...DEAL_COMMITMENTS_KEY, dealId],
    queryFn: async (): Promise<DealCommitments> =>
      unwrap(
        await api.GET("/deals/{id}/commitments", {
          params: { path: { id: dealId } },
        }),
        t,
      ),
  });
  const title = t("deal.commitments.title");
  if (query.isError) {
    // A reader without the contact or message grant is told so, never shown
    // an empty card that reads as "nothing is owed".
    return (
      <OverviewCard title={title}>
        {problemStatus(query.error) === 403 ? (
          <OverviewText>{t("deal.commitments.denied")}</OverviewText>
        ) : (
          <ErrorLine error={query.error} standing />
        )}
      </OverviewCard>
    );
  }
  if (!query.data) {
    return <OverviewCard title={title} />;
  }
  const { data, complete, has_more } = query.data;
  const foot = [
    !complete && t("deal.commitments.incomplete"),
    has_more && t("deal.commitments.more"),
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <OverviewCard
      title={title}
      empty={t("deal.commitments.empty")}
      foot={foot || undefined}
      footTone={complete ? undefined : "warn"}
    >
      {data.length > 0 && (
        <OverviewList>
          {data.map((row) => (
            <CommitmentRow key={row.id} row={row} onOpenEmail={onOpenEmail} />
          ))}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

/**
 * The commitment without a leading repeat of its speaker's name. The card
 * names the speaker on their own, and the model often starts the sentence
 * with the same name.
 */
function withoutSpeaker(body: string, speaker: string): string {
  const name = speaker.trim();
  if (name === "" || !body.toLowerCase().startsWith(name.toLowerCase())) {
    return body;
  }
  const rest = body.slice(name.length).replace(/^[\s:,–-]+/u, "");
  if (rest === "") {
    return body;
  }
  return rest.charAt(0).toLocaleUpperCase() + rest.slice(1);
}

function CommitmentRow({
  row,
  onOpenEmail,
}: Readonly<{
  row: DealCommitment;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const differs = row.differs_from_deal;
  return (
    <OverviewRow
      title={
        <>
          <strong>{row.contact_name}</strong>{" "}
          {withoutSpeaker(row.body, row.contact_name)}
        </>
      }
      sub={
        <>
          <span>{t("commitment.quote", { quote: row.source_quote })}</span>{" "}
          <span>
            {row.due_at
              ? t("co.next.due", { when: formatDate(row.due_at, locale, zone) })
              : t("co.next.undated")}
          </span>{" "}
          {differs && (
            <Badge tone="warning">
              {t("deal.commitments.differs", {
                amount: formatMoneyWhole(
                  differs.amount_minor,
                  differs.currency,
                  locale,
                ),
              })}
            </Badge>
          )}{" "}
          {onOpenEmail && row.source_kind === "email" && (
            <Button
              variant="link"
              onClick={() => onOpenEmail(row.source_activity_id)}
            >
              {t("deal.commitments.source")}
            </Button>
          )}{" "}
          {row.overdue ? (
            <OverdueAnswers id={row.id} />
          ) : (
            <DismissClaimButton id={row.id} />
          )}
        </>
      }
      right={
        row.overdue ? (
          <Badge tone="danger">{t("deal.commitments.overdue")}</Badge>
        ) : undefined
      }
    />
  );
}

/** The two answers a past-due commitment asks for, through the claim's settle. */
function OverdueAnswers({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const settle = useClaimSettle([]);
  // Settling writes the contact's record, so a reader who may not update
  // contacts is not offered a verb the server would refuse.
  const canSettle = useCanWrite("contact", "update");
  if (!canSettle) {
    return null;
  }
  return (
    <>
      <Button
        variant="link"
        pending={settle.isPending}
        onClick={() => settle.mutate({ id, outcome: "done" })}
      >
        {t("deal.commitments.done")}
      </Button>{" "}
      <Button
        variant="link"
        pending={settle.isPending}
        onClick={() => settle.mutate({ id, outcome: "dismissed" })}
      >
        {t("deal.commitments.noLongerTrue")}
      </Button>
      <ErrorLine error={settle.error} inline />
    </>
  );
}
