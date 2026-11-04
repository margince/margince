// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The changes Margince made to the reader's deals, as one summary row with an
// Undo per deal and one for all of them. Counts are deals: three changes to
// one deal are one deal and one Undo.

import { useState } from "react";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCanWrite } from "../lib/capability";
import { undoResultText } from "../lib/changecopy";
import {
  archiveQuestion,
  type MarginceChangeBundle,
  type MarginceChangeSummary,
  useUndoMarginceChanges,
} from "../lib/marginchanges";
import { EntityRef } from "./entityref";

/** How many deals the summary row names; the rest are behind "View all". */
const SUMMARY_DEALS = 5;

const KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  close_lost: "changes.kind.close_lost",
  deal_close_date: "changes.kind.deal_close_date",
  deal_amount: "changes.kind.deal_amount",
  deal_probability: "changes.kind.deal_probability",
  stage_move: "changes.kind.stage_move",
  deal_open: "changes.kind.deal_open",
};

/** What changed on one deal, in words. */
export function bundleKinds(kinds: readonly string[], t: Translator): string {
  return kinds
    .map((kind) => KIND_LABEL[kind])
    .filter((key): key is MessageKey => key !== undefined)
    .map((key) => t(key))
    .join(" · ");
}

/** The summary row Home shows once enough deals changed. */
export function ChangeSummaryRow({
  summary,
}: Readonly<{ summary: MarginceChangeSummary }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (n: number) => formatNumber(n, locale);
  const parts = [
    summary.closed > 0 &&
      plural("changes.part.closed", summary.closed, {
        count: count(summary.closed),
      }),
    summary.moved > 0 &&
      plural("changes.part.moved", summary.moved, {
        count: count(summary.moved),
      }),
    summary.redated > 0 &&
      plural("changes.part.redated", summary.redated, {
        count: count(summary.redated),
      }),
    summary.revalued > 0 &&
      plural("changes.part.revalued", summary.revalued, {
        count: count(summary.revalued),
      }),
    summary.probabilities > 0 &&
      plural("changes.part.probabilities", summary.probabilities, {
        count: count(summary.probabilities),
      }),
  ].filter(Boolean);
  return (
    <div className="magic-line">
      <p className="magic-line-text">
        {plural("changes.summary", summary.deals, {
          count: count(summary.deals),
          parts: parts.join(", "),
        })}
      </p>
      <ul className="magic-lines" aria-label={t("changes.deal.title")}>
        {summary.bundles.slice(0, SUMMARY_DEALS).map((bundle) => (
          <li className="magic-line" key={bundle.deal_id}>
            <p className="magic-line-text">
              <span className="magic-line-subject">
                <EntityRef
                  kind="deal"
                  id={bundle.deal_id}
                  name={bundle.deal_name}
                />
              </span>{" "}
              <span className="t-caption">{bundleKinds(bundle.kinds, t)}</span>
            </p>
            <div className="magic-line-controls">
              <BundleUndo bundles={[bundle]} label={t("common.undo")} />
            </div>
          </li>
        ))}
      </ul>
      <div className="magic-line-controls">
        <AppLink href="#/worklist">{t("changes.viewAll")}</AppLink>
        <BundleUndo bundles={summary.bundles} label={t("changes.undoAll")} />
      </div>
    </div>
  );
}

/**
 * Undoes whole deals. One deal answers in the words for its kinds; many
 * answer with how many were undone and how many kept a change.
 */
export function BundleUndo({
  bundles,
  label,
}: Readonly<{ bundles: readonly MarginceChangeBundle[]; label: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const writable = useCanWrite("deal", "update");
  const undo = useUndoMarginceChanges();
  const [answer, setAnswer] = useState<string | null>(null);
  const [asking, setAsking] = useState<{
    notes: number;
    tasks: number;
    work: string;
  } | null>(null);
  if (answer) {
    return <span role="status">{answer}</span>;
  }
  if (!writable || bundles.length === 0) {
    return null;
  }
  const press = (archiveConfirmedFor?: string) =>
    undo.mutate(
      { bundles, archiveConfirmedFor },
      {
        onSuccess: (results) => {
          const only = results.length === 1 ? results[0] : undefined;
          const question = only && archiveQuestion(only);
          if (question) {
            setAsking(question);
            return;
          }
          setAsking(null);
          if (only) {
            setAnswer(undoResultText(only, t));
            return;
          }
          const kept = results.filter((r) => r.outcome !== "undone").length;
          setAnswer(
            kept === 0
              ? plural("changes.undoAllDone", results.length, {
                  count: formatNumber(results.length, locale),
                })
              : t("changes.undoAllPartly", {
                  undone: formatNumber(results.length - kept, locale),
                  kept: formatNumber(kept, locale),
                }),
          );
        },
      },
    );
  return (
    <>
      {asking ? (
        <>
          <span role="status">
            {t("changes.undo.confirmArchive", {
              notes: formatNumber(asking.notes, locale),
              tasks: formatNumber(asking.tasks, locale),
            })}
          </span>
          <Button
            variant="ghost"
            pending={undo.isPending}
            busyLabel={t("history.undo.busy")}
            onClick={() => press(asking.work)}
          >
            {label}
          </Button>
        </>
      ) : (
        <Button
          variant="ghost"
          pending={undo.isPending}
          busyLabel={t("history.undo.busy")}
          onClick={() => press()}
        >
          {label}
        </Button>
      )}
      {undo.isError && <ErrorLine error={undo.error} />}
    </>
  );
}
