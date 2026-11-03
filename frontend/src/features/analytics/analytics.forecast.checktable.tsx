// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelBody } from "../../design-system/panel";
import type {
  ResolveAnswer,
  ResolveSheetLabels,
} from "../../design-system/resolvesheet";
import { formatDate, formatMoneyOrAbsent } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { OutOfDateBadge } from "./analytics.forecast.freshness";
import { InputCheckAnswerSheet } from "./inputcheck.answer";
import { checkLabel, SubjectCell } from "./inputcheck.facts";

type InputCheck = components["schemas"]["InputCheck"];
type Resolution = {
  id: string;
  answer: ResolveAnswer;
  dealId?: string;
  // A customer review's wait and lost answers also write the deal, through
  // their own routes.
  review?: boolean;
  // The reason a review closed lost is lost for, from the loss reason list.
  lostReasonCode?: string;
};

// The findings, and the one sheet that answers whichever was picked.
//
// The order is the server's: severity first, then money at stake, which is the
// endpoint's stated contract. Re-sorting here would be a second opinion on it.
export function CheckTable({
  checks,
  locale,
  title,
}: Readonly<{ checks: InputCheck[]; locale: Locale; title: string }>) {
  const t = useT();
  const zone = useRecordZone();
  const [params] = useUrlParams();
  // A door from a finding names it; a door from a group names its type.
  const focus = params.get("finding");
  const only = params.get("check");
  const rows = only ? checks.filter((check) => check.type === only) : checks;
  const gone = !!focus && !checks.some((check) => check.id === focus);
  // Closing keeps `id`, so the sheet's key holds and the Modal closes and
  // returns focus normally; a DIFFERENT finding changes the key and starts blank.
  const [sheet, setSheet] = useState<{ id: string; open: boolean } | null>(
    null,
  );
  const table = useRef<HTMLDivElement>(null);
  const opener = useRef<{ button: HTMLElement; row: number } | null>(null);
  const close = () => setSheet((was) => was && { ...was, open: false });
  // A save outlives its sheet. The reader may cancel and open another finding
  // before the first save lands, so only the saved finding's sheet closes.
  const resolve = useResolveCheck((id) =>
    setSheet((was) => (was?.id === id ? { ...was, open: false } : was)),
  );
  // A finding a refetch took away is no longer the server's to answer. So the
  // sheet follows the list rather than the click that opened it.
  const open =
    sheet?.open === true && checks.some((check) => check.id === sheet.id);

  const answerButtons = (): HTMLElement[] => [
    ...(table.current?.querySelectorAll<HTMLElement>(".cell-actions button") ??
      []),
  ];
  const answer = (check: InputCheck, button: HTMLElement) => {
    opener.current = { button, row: answerButtons().indexOf(button) };
    resolve.reset();
    setSheet({ id: check.id, open: true });
  };
  // The named finding is brought into view and given focus once it is drawn.
  useEffect(() => {
    if (!focus || !rows.some((check) => check.id === focus)) return;
    const target = table.current?.querySelector<HTMLElement>(
      `[data-finding="${CSS.escape(focus)}"]`,
    );
    target?.scrollIntoView?.({ block: "center" });
    target?.focus();
  }, [focus, rows]);
  // An answered finding leaves the list with its own button. The Answer now at
  // its row index is the next finding's, or the last one's when it was last.
  // So a keyboard reader moves down the list rather than back up it.
  const focusAfterClose = (): HTMLElement | null => {
    const from = opener.current;
    if (from === null || from.button.isConnected) {
      return from?.button ?? null;
    }
    const left = answerButtons();
    return left[Math.min(from.row, left.length - 1)] ?? null;
  };

  return (
    <>
      <PanelBody>
        {gone && <p className="sub">{t("review.findingGone")}</p>}
        {only && (
          <p className="sub">
            {t("review.onlyType", { finding: t(checkLabel(only)) })}{" "}
            <Button
              variant="link"
              onClick={() => replaceDial("check", undefined)}
            >
              {t("review.showAll")}
            </Button>
          </p>
        )}
        {rows.length === 0 ? (
          <p className="sub">{t("review.nothingToCheck")}</p>
        ) : (
          <div ref={table}>
            <DataTable<InputCheck>
              label={title}
              rows={rows}
              rowKey={(check) => check.id}
              columns={checkColumns(t, locale, zone, answer)}
            />
          </div>
        )}
      </PanelBody>
      <InputCheckAnswerSheet
        check={checks.find((check) => check.id === sheet?.id)}
        key={sheet?.id}
        open={open}
        pending={resolve.isPending}
        error={resolve.error}
        labels={sheetLabels(t)}
        returnFocusTo={focusAfterClose}
        onSubmit={(given, dealId, lostReasonCode) => {
          if (sheet !== null) {
            const review =
              checks.find((check) => check.id === sheet.id)?.type ===
              "deal_viability";
            resolve.mutate({
              id: sheet.id,
              answer: given,
              dealId,
              review,
              lostReasonCode,
            });
          }
        }}
        onClose={close}
      />
    </>
  );
}

function checkColumns(
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
  answer: (check: InputCheck, button: HTMLElement) => void,
): DataTableColumn<InputCheck>[] {
  return [
    {
      key: "severity",
      header: t("review.colSeverity"),
      render: (check) => <SeverityBadge severity={check.severity} />,
    },
    {
      key: "finding",
      header: t("review.colFinding"),
      render: (check) => (
        <>
          {t(checkLabel(check.type))}{" "}
          {check.inputs_changed && <OutOfDateBadge />}
        </>
      ),
    },
    {
      key: "deal",
      header: t("review.colDeal"),
      render: (check) => <SubjectCell check={check} />,
    },
    {
      key: "stake",
      header: t("review.colAtStake"),
      render: (check) => (
        <span className="t-num">
          {formatMoneyOrAbsent(
            check.affected_minor ?? null,
            check.currency ?? "",
            locale,
          )}
        </span>
      ),
    },
    {
      key: "since",
      // The record's clock, not the reader's: colleagues quote this date to
      // each other, and two of them must read the same day.
      header: t("review.colSeenSince"),
      render: (check) => formatDate(check.first_seen_at, locale, zone),
    },
    {
      key: "answer",
      header: t("table.actions"),
      headerHidden: true,
      render: (check) =>
        // A reader who may not answer is shown the finding to read, never a
        // button the server would refuse.
        check.destination?.mode === "read" ? (
          <span className="t-caption" tabIndex={-1} data-finding={check.id}>
            {t("review.readOnly")}
          </span>
        ) : (
          <div className="cell-actions">
            <Button
              data-finding={check.id}
              onClick={(event) => answer(check, event.currentTarget)}
            >
              {t("review.answer")}
            </Button>
          </div>
        ),
    },
  ];
}

// The answer to one finding, sent.
//
// The finding and the answer travel as VARIABLES. Read from the closure they
// would be whatever the last render saw. That is the wrong finding when a
// save races a refetch.
function useResolveCheck(onResolved: (id: string) => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      answer,
      dealId,
      review,
      lostReasonCode,
    }: Resolution) => {
      if (review && answer.outcome === "remind_later") {
        unwrap(
          await api.POST("/forecast/assurance/exceptions/{id}/wait", {
            params: { path: { id } },
            body: { until: answer.remindAt ?? "" },
          }),
        );
        return;
      }
      if (review && answer.outcome === "not_relevant") {
        unwrap(
          await api.POST("/forecast/assurance/exceptions/{id}/close-lost", {
            params: { path: { id } },
            body: {
              lost_reason: answer.reason ?? "",
              lost_reason_code: lostReasonCode,
            },
          }),
        );
        return;
      }
      unwrap(
        await api.POST("/forecast/assurance/exceptions/{id}/resolve", {
          params: { path: { id } },
          body: {
            outcome: answer.outcome,
            deal_id: answer.outcome === "fixed_record" ? dealId : undefined,
            reason: answer.reason,
            remind_at: answer.remindAt,
            expires_at: answer.expiresAt,
          },
        }),
      );
    },
    onSuccess: async (_data, { id }) => {
      // Both lists move: the finding leaves this one, and the run's readiness
      // may change with it.
      await client.invalidateQueries({ queryKey: ["input-checks"] });
      await client.invalidateQueries({ queryKey: ["forecast-assurance"] });
      onResolved(id);
    },
  });
}

// How urgent a finding is, as a word in the tone the rest of the product gives
// that urgency.
export function SeverityBadge({
  severity,
}: Readonly<{ severity: InputCheck["severity"] }>) {
  const t = useT();
  const tone = { high: "danger", medium: "warning", low: "default" } as const;
  const label = {
    high: "review.severityHigh",
    medium: "review.severityMedium",
    low: "review.severityLow",
  } as const;
  return <Badge tone={tone[severity]}>{t(label[severity])}</Badge>;
}
export function sheetLabels(t: ReturnType<typeof useT>): ResolveSheetLabels {
  return {
    title: t("review.sheetTitle"),
    outcomeLegend: t("review.outcomeLegend"),
    outcomes: [
      { value: "fixed_record", label: t("review.fixedRecord") },
      { value: "added_evidence", label: t("review.addedEvidence") },
      {
        value: "value_correct",
        label: t("review.valueCorrect"),
        description: t("review.hidesUntilExpiry"),
      },
      {
        value: "not_relevant",
        label: t("review.notRelevant"),
        description: t("review.hidesUntilExpiry"),
      },
      { value: "remind_later", label: t("review.remindLater") },
      { value: "reassign", label: t("review.reassign") },
    ],
    reason: t("review.reason"),
    reasonHelp: t("review.reasonHelp"),
    remindAt: t("review.remindAt"),
    expiresAt: t("review.expiresAt"),
    expiresHelp: t("review.expiresHelp"),
    cancel: t("review.cancel"),
    submit: t("review.submit"),
  };
}
