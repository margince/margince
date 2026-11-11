// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words for what Margince changed on a deal, and the Undo of one deal or
// many. Counts are deals: three changes to one deal are one deal and one Undo.

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "../design-system/atoms";
import { Disclosure } from "../design-system/disclosure";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCanWrite } from "../lib/capability";
import { undoResultText } from "../lib/changecopy";
import {
  archiveQuestion,
  type BundleUndoRun,
  type MarginceChangeBundle,
  pendingChangesQuery,
  stillPending,
  useUndoMarginceChanges,
} from "../lib/marginchanges";
import { useSavedUndoRuns, useUndoManifest } from "../lib/marginchanges.runs";
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
 */
export function BundleUndo({
  bundles,
  label,
  onDone,
}: Readonly<{
  bundles: readonly MarginceChangeBundle[];
  label: string;
  onDone?: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const writable = useCanWrite("deal", "update");
  const undo = useUndoMarginceChanges();
  const manifest = useUndoManifest();
  const [run, setRun] = useState<BundleUndoRun | null>(null);
  const [asking, setAsking] = useState<ArchiveQuestion | null>(null);
  const [rereading, setRereading] = useState(false);
  if (!run && (!writable || bundles.length === 0)) {
    return null;
  }
  const press = (
    chosen: readonly MarginceChangeBundle[],
    archiveConfirmedFor?: string,
  ) =>
    undo.mutate(
      { bundles: chosen, archiveConfirmedFor },
      {
        onSuccess: (answer) => {
          const merged = mergeRuns(run, answer);
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
    if (bundles.length > 1 && manifest) {
      undo.mutate({ bundles, resume: manifest }, { onSuccess: setRun });
      return;
    }
    setRereading(true);
    try {
      const fresh = await client.fetchQuery({
        ...pendingChangesQuery(),
        staleTime: 0,
      });
      const again = stillPending(unsent, fresh);
      if (again.length === 0) {
        setRun((last) => last && { ...last, unsent: [], error: undefined });
        onDone?.();
        return;
      }
      press(again);
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
        bundles={bundles}
        single={bundles.length === 1}
        pending={undo.isPending || rereading}
        onRetry={() => void retry(run.unsent)}
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
// replaced and every other one is kept.
function mergeRuns(
  last: BundleUndoRun | null,
  answer: BundleUndoRun,
): BundleUndoRun {
  const answered = new Set(answer.results.map((result) => result.deal_id));
  const kept = (last?.results ?? []).filter(
    (result) => !answered.has(result.deal_id),
  );
  return { ...answer, results: [...kept, ...answer.results] };
}

export function LastBulkUndo() {
  const t = useT();
  const saved = useSavedUndoRuns();
  const undo = useUndoMarginceChanges();
  const [resultsOpen, setResultsOpen] = useState(false);
  const [shown, setShown] = useState(50);
  if (!saved.manifest) return null;
  const completed = new Set(
    saved.data?.filter((run) => run.complete).map((run) => run.run_id),
  );
  const answered = new Set(
    saved.data?.flatMap((run) => run.results.map((result) => result.deal_id)),
  );
  const unsent: MarginceChangeBundle[] = saved.manifest.batches
    .filter((batch) => !completed.has(batch.id))
    .flatMap((batch) =>
      batch.items.filter((item) => !answered.has(item.deal_id)),
    )
    .map((item) => ({ ...item, deal_name: "", kinds: [], latest_at: "" }));
  const run: BundleUndoRun = {
    results: saved.data?.flatMap((batch) => batch.results) ?? [],
    unsent,
  };
  return (
    <section aria-label={t("changes.bulk.last")}>
      <p>{t("changes.bulk.last")}</p>
      {saved.isPending ? (
        <span role="status">{t("history.undo.busy")}</span>
      ) : saved.isSuccess ? (
        <BundleUndoAnswer
          run={run}
          bundles={[]}
          single={false}
          pending={undo.isPending}
          onRetry={() => {
            if (saved.manifest)
              undo.mutate({ bundles: [], resume: saved.manifest });
          }}
        />
      ) : (
        <ErrorLine
          error={saved.error}
          actions={
            <Button variant="ghost" onClick={() => void saved.refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      )}
      {undo.isError && <ErrorLine error={undo.error} />}
      {saved.data && saved.data.some((batch) => batch.results.length > 0) && (
        <Disclosure
          summary={t("changes.bulk.results")}
          open={resultsOpen}
          onToggle={setResultsOpen}
        >
          {resultsOpen && (
            <ul>
              {saved.data
                .flatMap((batch) => batch.results)
                .slice(0, shown)
                .map((result) => (
                  <li key={result.deal_id}>
                    <EntityRef kind="deal" id={result.deal_id} />{" "}
                    {undoResultText(result, t)}
                  </li>
                ))}
            </ul>
          )}
          {resultsOpen && run.results.length > shown && (
            <Button
              variant="ghost"
              onClick={() => setShown((count) => count + 50)}
            >
              {t("list.loadMore")}
            </Button>
          )}
        </Disclosure>
      )}
    </section>
  );
}

function BundleUndoAnswer({
  run,
  bundles,
  single,
  pending,
  onRetry,
}: Readonly<{
  run: BundleUndoRun;
  bundles: readonly MarginceChangeBundle[];
  single: boolean;
  pending: boolean;
  onRetry: () => void;
}>) {
  const t = useT();
  const only = single && run.unsent.length === 0 ? run.results[0] : undefined;
  return (
    <>
      <span role="status">
        {only ? undoResultText(only, t) : <BulkCounts run={run} />}
      </span>
      {!only && <UnfinishedDeals run={run} bundles={bundles} />}
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
  bundles,
}: Readonly<{
  run: BundleUndoRun;
  bundles: readonly MarginceChangeBundle[];
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
            name={
              bundles.find((bundle) => bundle.deal_id === result.deal_id)
                ?.deal_name
            }
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
    count("changes.bulk.unsent", run.unsent.length),
  ]
    .filter(Boolean)
    .join(" ");
}
