// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the customer committed to that this deal waits on.
//
// A customer's commitment is never a task here, because nobody on our side can
// do it; it is watched. This card is where the deal shows it: who committed,
// to what, by when, in which words, with the message to check it against and a
// way to dismiss a reading that got it wrong.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import { DEAL_COMMITMENTS_KEY, DismissClaimButton } from "../taskactions";
import "./deal360.css";

type DealCommitments = components["schemas"]["DealCommitments"];

/** The one read of a deal's watched commitments. */
export function useDealCommitments(dealId: string) {
  return useQuery({
    queryKey: [...DEAL_COMMITMENTS_KEY, dealId],
    queryFn: async (): Promise<DealCommitments> => {
      const { data, error } = await api.GET("/deals/{id}/commitments", {
        params: { path: { id: dealId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * DealWatchCard lists the customer's open commitments on this deal's account.
 * It draws nothing while there is nothing to watch, so a deal with no
 * commitment carries no empty card; a list the reader sees only part of says
 * so rather than reading as complete.
 */
export function DealWatchCard({
  dealId,
  onOpenEmail,
}: Readonly<{
  dealId: string;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const query = useDealCommitments(dealId);
  // A read that failed — refused, or broken — is said, never drawn as an
  // account that owes nothing.
  if (query.isError) {
    return (
      <Panel title={t("deal.watch.title")}>
        <PanelBody>
          <ErrorLine error={query.error} />
        </PanelBody>
      </Panel>
    );
  }
  // A server older than this card answers without the list, and a card
  // guessing at it would claim what nobody read.
  if (!query.data?.data) {
    return null;
  }
  return <DealWatchList commitments={query.data} onOpenEmail={onOpenEmail} />;
}

/** DealWatchList draws one answer of the read, for the card and its stories. */
export function DealWatchList({
  commitments,
  onOpenEmail,
}: Readonly<{
  commitments: DealCommitments;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  if (commitments.data.length === 0 && commitments.complete) {
    return null;
  }
  return (
    <Panel title={t("deal.watch.title")}>
      {!commitments.complete && (
        <PanelBody>
          <p className="t-caption">{t("deal.watch.incomplete")}</p>
        </PanelBody>
      )}
      {commitments.data.map((row) => (
        <PanelRow key={row.id} className="deal-watch-row">
          <span className="t-body">{`${row.contact_name}: ${row.body}`}</span>
          <span className="t-caption deal-watch-quote">
            {t("commitment.quote", { quote: row.source_quote })}
          </span>
          <span className="t-caption deal-watch-meta">
            {row.due_at
              ? t("co.next.due", {
                  when: formatDate(row.due_at, locale, recordZone),
                })
              : t("co.next.undated")}
            {/* Only a message opens here: the page's drawer reads mail, and
                a meeting's words are already quoted above. */}
            {onOpenEmail && row.source_kind === "email" && (
              <Button
                variant="link"
                onClick={() => onOpenEmail(row.source_activity_id)}
              >
                {t("deal.watch.source")}
              </Button>
            )}
            <DismissClaimButton id={row.id} />
          </span>
        </PanelRow>
      ))}
    </Panel>
  );
}
