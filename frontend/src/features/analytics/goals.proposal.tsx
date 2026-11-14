// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Twelve months proposed from a team member's sales won so far. The proposal
// saves nothing: the manager takes it into the editor and saves the draft.

import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AiPending } from "../../design-system/aipending";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { QuietVerbs } from "../../design-system/verbvolume";
import { formatAnalyticsMoney, identifierNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { GoalBars } from "./goals.bars";
import type { SalesGoalPlan } from "./progress.queries";

export type GoalProposal = components["schemas"]["SalesGoalProposal"];

function useGoalProposal(plan: SalesGoalPlan) {
  return useMutation({
    mutationFn: async (yearly: number | null) =>
      unwrap(
        await api.POST(
          "/analytics/sales-goals/{owner_id}/{year}/proposed-months",
          {
            params: { path: { owner_id: plan.owner_id, year: plan.year } },
            body: yearly === null ? {} : { yearly_total_minor: yearly },
          },
        ),
      ),
  });
}

/**
 * The ask beside the yearly goal, and its answer. A yearly goal already
 * entered is spread; without one the proposal sets the year too. The answer
 * stays until the manager takes it or dismisses it.
 */
export function GoalProposalStep({
  plan,
  yearly,
  onApply,
}: Readonly<{
  plan: SalesGoalPlan;
  yearly: number | null;
  onApply: (proposal: GoalProposal) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const propose = useGoalProposal(plan);
  const ask = () => {
    if (!propose.isPending)
      propose.mutate(yearly !== null && yearly > 0 ? yearly : null);
  };
  const refusal = propose.isError ? problemCodeOf(propose.error) : null;
  if (refusal === "ai_not_configured")
    return (
      <div className="goal-proposal">
        <ErrorLine>{t("goals.propose.noModel")}</ErrorLine>
      </div>
    );
  const proposal = propose.data;
  return (
    // The editor's footer holds the one fill, so this step stays quiet.
    <QuietVerbs>
      <Button
        variant="ai"
        onClick={ask}
        pending={propose.isPending}
        busyLabel={t("goals.propose.busy")}
      >
        {t("goals.propose")}
      </Button>
      {(propose.isPending || propose.isError || proposal) && (
        <div className="goal-proposal">
          {propose.isPending && (
            <AiPending label={t("goals.propose.busy")} lines={2} />
          )}
          {refusal === "no_sales_history" && (
            <ErrorLine>
              {t("goals.propose.noHistory", {
                name: plan.owner_name,
                year: identifierNumber(plan.year),
              })}
            </ErrorLine>
          )}
          {propose.isError && refusal !== "no_sales_history" && (
            <ErrorLine
              error={propose.error}
              actions={
                <Button variant="ghost" onClick={ask}>
                  {t("reporting.setup.retry")}
                </Button>
              }
            />
          )}
          {proposal && (
            <Callout
              tone="ai"
              live="status"
              title={t("goals.propose.title", {
                total: formatAnalyticsMoney(
                  proposal.total_minor,
                  plan.currency,
                  locale,
                ),
              })}
              actions={
                <>
                  <Button
                    variant="ai"
                    onClick={() => {
                      onApply(proposal);
                      propose.reset();
                    }}
                  >
                    {t("goals.propose.apply")}
                  </Button>
                  <Button variant="ghost" onClick={() => propose.reset()}>
                    {t("reporting.setup.dismiss")}
                  </Button>
                </>
              }
            >
              <p>{proposal.reason}</p>
              <div className="goal-proposal-bars">
                <GoalBars months={proposal.months} />
              </div>
              <p className="t-caption">{t("goals.propose.review")}</p>
            </Callout>
          )}
        </div>
      )}
    </QuietVerbs>
  );
}
