// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { Handshake } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { type Choice, ChoiceList } from "../design-system/choicelist";
import { ErrorLine } from "../design-system/errorline";
import { useToast } from "../design-system/toast";
import {
  formatDayMonthNearYear,
  formatMoneyWhole,
  MONEY_ABSENT,
} from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
import { type Translator, useLocale, useT } from "../i18n";
import { TASK_QUEUE_KEY } from "../lib/activitykeys";
import { unwrap } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";
import { worklistKey } from "../lib/workkeys";
import { WHICH_DEAL_SOURCE, WhichDealDecline } from "./whichdealdecline";
import "./whichdealchoice.css";

type Activity = components["schemas"]["Activity"];
type Deal = components["schemas"]["Deal"];

/** The deals the question offered, as capture stored them on the task. */
function offeredDeals(task: Activity): string[] {
  const offered = task.raw?.offered_deal_ids;
  return Array.isArray(offered)
    ? offered.filter((id): id is string => typeof id === "string")
    : [];
}

/** Whether the task still asks which deal its mail is about. */
export function asksWhichDeal(task: Activity): boolean {
  return (
    task.source_system === WHICH_DEAL_SOURCE &&
    Boolean(task.source_activity_id) &&
    !task.is_done &&
    offeredDeals(task).length > 0
  );
}

/**
 * The deal question as a choice: each deal the mail may be about as a card
 * with its value and close date, then Link mail to deal. Filing the mail on a
 * deal answers the question, so the task leaves the list.
 */
export function WhichDealChoice({ task }: Readonly<{ task: Activity }>) {
  const t = useT();
  const [picked, setPicked] = useState("");
  const mail = task.source_activity_id;
  const offered = offeredDeals(task);
  const deals = useQueries({
    queries: offered.map((id) => ({
      queryKey: ["deal", id],
      queryFn: async () =>
        unwrap(await api.GET("/deals/{id}", { params: { path: { id } } })),
    })),
  });
  const file = useFileOnDeal(task.id);
  const choices = useDealChoices(
    deals.flatMap((read) => (read.data ? [read.data] : [])),
  );
  if (!mail || !asksWhichDeal(task)) return null;
  return (
    <div className="which-deal-choice">
      {choices.length > 0 && (
        <ChoiceList
          legend={t("tasks.whichDeal.legend")}
          hideLegend
          layout="cards"
          value={picked}
          onChange={setPicked}
          choices={choices}
        />
      )}
      <div className="which-deal-verbs">
        {choices.length > 0 && (
          <Button
            variant="primary"
            disabled={!picked || file.isPending}
            onClick={() => file.mutate({ mail, deal: picked })}
          >
            {t("tasks.whichDeal.file")}
          </Button>
        )}
        <WhichDealDecline task={task} />
      </div>
      {file.isError && <ErrorLine error={file.error} inline />}
    </div>
  );
}

function useDealChoices(deals: readonly Deal[]): Choice<string>[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return deals
    .filter((deal) => deal.status === "open")
    .map((deal) => ({
      value: deal.id,
      label: deal.name,
      mark: <Handshake aria-hidden="true" />,
      description: dealFacts(deal, t, locale, zone),
    }));
}

function dealFacts(
  deal: Deal,
  t: Translator,
  locale: Parameters<typeof formatMoneyOrWord>[2],
  zone: string,
): string {
  const facts = [
    formatMoneyOrWord(
      deal.amount_minor,
      deal.currency,
      locale,
      MONEY_ABSENT,
      formatMoneyWhole,
    ),
  ];
  if (deal.expected_close_date) {
    facts.push(
      t("deal.closes", {
        date: formatDayMonthNearYear(
          deal.expected_close_date,
          locale,
          zone,
          new Date(),
        ),
      }),
    );
  }
  return facts.join(" · ");
}

function useFileOnDeal(taskId: string) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ mail, deal }: { mail: string; deal: string }) =>
      unwrap(
        await api.POST("/activities/relink-thread", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: {
            activity_id: mail,
            entity_type: "deal",
            entity_id: deal,
            replace_existing_of_type: false,
          },
        }),
        t,
      ),
    onSuccess: async () => {
      toast.show(t("tasks.whichDeal.filed"));
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["activity", taskId] }),
        queryClient.invalidateQueries({ queryKey: TASK_QUEUE_KEY }),
        queryClient.invalidateQueries({ queryKey: worklistKey }),
      ]);
    },
  });
}
