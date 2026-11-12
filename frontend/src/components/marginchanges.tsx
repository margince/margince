// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words for what Margince changed on a deal, and the Undo of one deal or
// many. Counts are deals: three changes to one deal are one deal and one Undo.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCanWrite } from "../lib/capability";
import { undoResultText } from "../lib/changecopy";
import { useHandledForYou } from "../lib/handled";
import {
  archiveQuestion,
  type BundleUndoRun,
  type MarginceChangeBundle,
  pendingChangesQuery,
  stillPending,
  usePendingMarginceChanges,
  useUndoMarginceChanges,
} from "../lib/marginchanges";
import { useViewerId } from "../lib/session";
import {
  rememberedUndoRun,
  rememberUndoRun,
  settledRun,
  useRememberedUndoRun,
} from "../lib/undorun";
import { EntityRef } from "./entityref";

const KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  close_lost: "changes.kind.close_lost",
  deal_close_date: "changes.kind.deal_close_date",
  deal_amount: "changes.kind.deal_amount",
  deal_probability: "changes.kind.deal_probability",
  stage_move: "changes.kind.stage_move",
  deal_open: "changes.kind.deal_open",
  deal_revive: "changes.kind.deal_revive",
};

/** What changed on one deal, in words. */
export function bundleKinds(kinds: readonly string[], t: Translator): string {
  return kinds
    .map((kind) => KIND_LABEL[kind])
    .filter((key): key is MessageKey => key !== undefined)
    .map((key) => t(key))
    .join(" · ");
}

// The question, and the bundles it was asked about: the confirmation goes back
// at that read's version, not at the one the press first sent.
type ArchiveQuestion = NonNullable<ReturnType<typeof archiveQuestion>> & {
  sent: readonly MarginceChangeBundle[];
};

/**
 * Undoes whole deals. One deal answers in the words for its kinds. Many answer
 * with a count for each outcome, and the deals a failed batch never answered
 * for can be sent again. `onDone` hears each answer the server gave.
 * `remember` keeps the answer in this browser until the reader clears it, so
 * a reload still shows what the last run did.
 */
export function BundleUndo({
  bundles,
  label,
  onDone,
  remember = false,
}: Readonly<{
  bundles: readonly MarginceChangeBundle[];
  label: string;
  onDone?: () => void;
  remember?: boolean;
}>) {
  const t = useT();
  const client = useQueryClient();
  const writable = useCanWrite("deal", "update");
  const undo = useUndoMarginceChanges();
  const { run, nameOf, setRun } = useUndoRun(remember, bundles);
  const [asking, setAsking] = useState<ArchiveQuestion | null>(null);
  const [rereading, setRereading] = useState(false);
  if (!run && (!writable || bundles.length === 0)) {
    return null;
  }
  const press = (
    chosen: readonly MarginceChangeBundle[],
    archiveConfirmedFor?: string,
    left: readonly MarginceChangeBundle[] = [],
  ) =>
    undo.mutate(
      { bundles: chosen, archiveConfirmedFor },
      {
        onSuccess: (answer) => {
          const merged = mergeRuns(run, answer, left);
          const only =
            bundles.length === 1 && merged.results.length === 1
              ? merged.results[0]
              : undefined;
          const question = only && archiveQuestion(only);
          if (question) {
            setAsking({ ...question, sent: chosen });
            return;
          }
          setAsking(null);
          setRun(merged);
          if (answer.results.length > 0) onDone?.();
        },
      },
    );
  // A batch that failed may still have landed, so the deals are read again and
  // only what still waits goes back.
  const retry = async (unsent: readonly MarginceChangeBundle[]) => {
    setRereading(true);
    try {
      const fresh = await client.fetchQuery({
        ...pendingChangesQuery(),
        staleTime: 0,
      });
      const again = stillPending(unsent, fresh);
      // A deal the read no longer lists keeps a line of its own as settled.
      const left = unsent.filter(
        (bundle) => !again.some((next) => next.deal_id === bundle.deal_id),
      );
      if (again.length === 0) {
        setRun(
          (last) =>
            last && {
              ...last,
              unsent: [],
              settled: [...(last.settled ?? []), ...left],
              error: undefined,
            },
        );
        onDone?.();
        return;
      }
      press(again, undefined, left);
    } catch (error) {
      setRun((last) => last && { ...last, error });
    } finally {
      setRereading(false);
    }
  };
  if (run && !asking) {
    return (
      <BundleUndoAnswer
        run={run}
        nameOf={nameOf}
        single={
          bundles.length === 1 && run.results.length + run.unsent.length <= 1
        }
        pending={undo.isPending || rereading}
        onRetry={() => void retry(run.unsent)}
        onClear={remember ? () => setRun(null) : undefined}
      />
    );
  }
  return (
    <>
      {asking && (
        <span role="status">
          <ConfirmWords asking={asking} />
        </span>
      )}
      <Button
        variant="ghost"
        pending={undo.isPending}
        busyLabel={t("history.undo.busy")}
        onClick={() => press(asking?.sent ?? bundles, asking?.work)}
      >
        {label}
      </Button>
      {undo.isError && <ErrorLine error={undo.error} />}
    </>
  );
}

type RunUpdate =
  | BundleUndoRun
  | null
  | ((last: BundleUndoRun | null) => BundleUndoRun | null);

/**
 * The run this control answers with, and the name of each deal in it. A deal
 * a failed batch did not send is read against the change history. Remembered,
 * the run is the browser's last one; its error stays with this control.
 */
function useUndoRun(
  remember: boolean,
  bundles: readonly MarginceChangeBundle[],
): Readonly<{
  run: BundleUndoRun | null;
  nameOf: (dealId: string) => string | undefined;
  setRun: (next: RunUpdate) => void;
}> {
  const [own, setOwn] = useState<BundleUndoRun | null>(null);
  const [error, setError] = useState<unknown>(undefined);
  const seat = useViewerId();
  const kept = useRememberedUndoRun();
  const receipts = useHandledForYou().data?.receipts;
  const pending = usePendingMarginceChanges().data;
  const held = remember ? kept && { ...kept.run, error } : own;
  const named = (dealId: string) =>
    bundles.find((bundle) => bundle.deal_id === dealId)?.deal_name;
  const setRun = (next: RunUpdate) => {
    if (!remember) {
      setOwn(next);
      return;
    }
    const last = rememberedUndoRun(seat);
    const run =
      typeof next === "function" ? next(last && { ...last.run, error }) : next;
    setError(run?.error);
    rememberUndoRun(
      seat,
      run && {
        run,
        names: {
          ...last?.names,
          ...Object.fromEntries(
            bundles.map((bundle) => [bundle.deal_id, bundle.deal_name]),
          ),
        },
      },
    );
  };
  return {
    run: held && settledRun(held, receipts, pending),
    nameOf: (dealId) => named(dealId) ?? kept?.names[dealId],
    setRun,
  };
}

function ConfirmWords({ asking }: Readonly<{ asking: ArchiveQuestion }>) {
  const t = useT();
  const { locale } = useLocale();
  return t(
    asking.kind === "deal_revive"
      ? "changes.undo.confirmRelost"
      : "changes.undo.confirmArchive",
    {
      notes: formatNumber(asking.notes, locale),
      tasks: formatNumber(asking.tasks, locale),
    },
  );
}

// A retry answers for the deals it sent again, so their earlier answer is
// replaced and every other one is kept. `left` are the deals the retry found
// settled and did not send.
function mergeRuns(
  last: BundleUndoRun | null,
  answer: BundleUndoRun,
  left: readonly MarginceChangeBundle[],
): BundleUndoRun {
  const answered = new Set(answer.results.map((result) => result.deal_id));
  const kept = (last?.results ?? []).filter(
    (result) => !answered.has(result.deal_id),
  );
  return {
    ...answer,
    results: [...kept, ...answer.results],
    settled: [...(last?.settled ?? []), ...left],
  };
}

function BundleUndoAnswer({
  run,
  nameOf,
  single,
  pending,
  onRetry,
  onClear,
}: Readonly<{
  run: BundleUndoRun;
  nameOf: (dealId: string) => string | undefined;
  single: boolean;
  pending: boolean;
  onRetry: () => void;
  onClear?: () => void;
}>) {
  const t = useT();
  const only = single && run.unsent.length === 0 ? run.results[0] : undefined;
  return (
    <>
      <span role="status">
        {only ? undoResultText(only, t) : <BulkCounts run={run} />}
      </span>
      {!only && <UnfinishedDeals run={run} nameOf={nameOf} />}
      {run.unsent.length > 0 && (
        <Button
          variant="ghost"
          pending={pending}
          busyLabel={t("history.undo.busy")}
          onClick={onRetry}
        >
          {t("changes.bulk.retry")}
        </Button>
      )}
      {run.error !== undefined && run.error !== null && (
        <ErrorLine error={run.error} />
      )}
      {onClear && (
        <Button variant="link" onClick={onClear}>
          {t("changes.bulk.clear")}
        </Button>
      )}
    </>
  );
}

/**
 * Each deal the run did not fully undo, linked, with what happened to it. A
 * deal that needs a confirmation is confirmed on its own page, where the
 * single-deal Undo asks the question against that deal's current changes.
 */
function UnfinishedDeals({
  run,
  nameOf,
}: Readonly<{
  run: BundleUndoRun;
  nameOf: (dealId: string) => string | undefined;
}>) {
  const t = useT();
  const unfinished = run.results.filter(
    (result) => result.outcome !== "undone",
  );
  if (unfinished.length === 0) return null;
  return (
    <ul aria-label={t("changes.bulk.unfinished")}>
      {unfinished.map((result) => (
        <li key={result.deal_id}>
          <EntityRef
            kind="deal"
            id={result.deal_id}
            name={nameOf(result.deal_id)}
          />{" "}
          {archiveQuestion(result)
            ? t("changes.bulk.confirmOnDeal")
            : undoResultText(result, t)}
        </li>
      ))}
    </ul>
  );
}

/** How many deals each outcome reached, one sentence per outcome. */
function BulkCounts({ run }: Readonly<{ run: BundleUndoRun }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (key: Parameters<typeof plural>[0], n: number) =>
    n > 0 ? plural(key, n, { count: formatNumber(n, locale) }) : null;
  const confirm = run.results.filter((result) => archiveQuestion(result));
  const refused = run.results.filter(
    (result) => result.outcome === "refused" && !archiveQuestion(result),
  );
  return [
    count(
      "changes.bulk.undone",
      run.results.filter((result) => result.outcome === "undone").length,
    ),
    count(
      "changes.bulk.partly",
      run.results.filter((result) => result.outcome === "partly_undone").length,
    ),
    count("changes.bulk.kept", refused.length),
    count("changes.bulk.confirm", confirm.length),
    count("changes.bulk.settled", run.settled?.length ?? 0),
    count("changes.bulk.unsent", run.unsent.length),
  ]
    .filter(Boolean)
    .join(" ");
}
