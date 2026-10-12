import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Select } from "../design-system/select";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { dealRecordKeys } from "./activitykeys";
import { BulkVerbs } from "./bulkverbs";
import { ProblemError, problemMessageOf, unwrap } from "./common";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

/** One row's outcome in a fan-out: it went through, or it did not and why. */
export type DealBulkOutcome = { id: string; name: string; error?: string };

/**
 * Bulk verbs over selected deals: assign an owner, move to a stage, archive.
 *
 * Owner and archive are the shared `BulkVerbs`: one preview, then one change on
 * the server with each deal's own version. Moving a stage is a client-side
 * fan-out of the record's own advance, because a stage move is not a bulk verb
 * the server offers. Each row sends ITS OWN If-Match from the version the list
 * holds; a row that moved under the reader answers 409, is reported by name,
 * and can be retried once the list has refetched.
 *
 * Only OPEN stages are offered. Closing a deal asks for a lost reason and
 * freezes an exchange rate, and doing that to a dozen deals behind one button
 * — with one reason standing for all of them — is not a thing this bar should
 * make easy.
 */
export function DealBulkBar({
  deals,
  stages,
  onDone,
}: Readonly<{
  /** The selected rows, with the versions the list currently holds. */
  deals: readonly Deal[];
  /** The pipeline's stages; the terminal ones are filtered out here. */
  stages: readonly Stage[];
  /** Called after any run — the caller refetches and clears the selection. */
  onDone: (outcomes: readonly DealBulkOutcome[]) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [stageId, setStageId] = useState("");
  const [outcomes, setOutcomes] = useState<readonly DealBulkOutcome[]>([]);
  const openStages = stages.filter((stage) => stage.semantic === "open");

  // A stage chosen for one selection must not fire at another, so the picker
  // clears when the membership changes. `stable`, not the reader's collation:
  // this string is compared against itself to detect a changed selection.
  const selectionKey = deals
    .map((deal) => deal.id)
    .sort(stable)
    .join(",");
  const [armedFor, setArmedFor] = useState(selectionKey);
  if (armedFor !== selectionKey) {
    setArmedFor(selectionKey);
    setStageId("");
  }

  // Shown only for rows still in the selection. A run that partly failed
  // narrows the selection to the rows that refused, and those messages are
  // exactly what the reader needs; a message about a row they have since
  // deselected is not.
  const failed = outcomes.filter(
    (outcome) => outcome.error && deals.some((deal) => deal.id === outcome.id),
  );

  const move = useMutation({
    // The rows and the stage ride the VARIABLES, never the closure. React Query
    // re-arms a mutation's options in a passive effect, so a verb pressed
    // immediately after the selection changed would otherwise fan out over the
    // PREVIOUS render's selection.
    mutationFn: async ({
      rows,
      toStageId,
    }: {
      rows: readonly Deal[];
      toStageId: string;
    }): Promise<DealBulkOutcome[]> =>
      // Sequential, not Promise.all: a burst of concurrent writes against one
      // reader's own deals buys nothing but contention.
      rows.reduce<Promise<DealBulkOutcome[]>>(async (acc, deal) => {
        const done = await acc;
        const name = deal.name;
        try {
          unwrap(
            await api.POST("/deals/{id}/advance", {
              params: {
                path: { id: deal.id },
                ...ifMatch(requireVersion(deal.version)),
              },
              body: { to_stage_id: toStageId },
            }),
            t,
          );
          done.push({ id: deal.id, name });
        } catch (error) {
          done.push({
            id: deal.id,
            name,
            error:
              error instanceof ProblemError
                ? problemMessageOf(error, t)
                : t("deals.bulkFailedRow"),
          });
        }
        return done;
      }, Promise.resolve([])),
    onSuccess: async (result) => {
      // Awaited: the rows that refused keep their selection so they can be
      // retried, and a retry that fired before the refetch landed would
      // resend the very version that just conflicted.
      await queryClient.invalidateQueries({ queryKey: ["deals"] });
      // Each row that moved carries reads derived from it — the deal status
      // card says what its stage MEANS — and the list key does not reach them.
      for (const outcome of result) {
        if (outcome.error !== undefined) {
          continue;
        }
        for (const queryKey of dealRecordKeys(outcome.id)) {
          queryClient.invalidateQueries({ queryKey });
        }
      }
      setOutcomes(result);
      onDone(result);
    },
  });

  const moveStage = () =>
    move.mutate({
      // A deal already in the target stage is left alone. The server treats
      // every advance as a transition — it writes a stage-history row and
      // emits deal.stage_changed without asking whether anything moved — so
      // sending one for a row already there would record a move that never
      // happened, in the table the velocity reports read.
      rows: deals.filter((deal) => deal.stage_id !== stageId),
      toStageId: stageId,
    });

  return (
    <>
      <BulkVerbs
        recordType="deal"
        rows={deals.map((deal) => ({
          id: deal.id,
          version: deal.version,
          label: deal.name,
        }))}
        busy={move.isPending}
        onDone={() => {
          setOutcomes([]);
          onDone([]);
        }}
      >
        <Select
          aria-label={t("deals.bulkStage")}
          value={stageId}
          placeholder={t("deals.bulkStagePick")}
          disabled={move.isPending}
          onChange={setStageId}
          options={openStages.map((stage) => ({
            value: stage.id,
            label: stage.name,
          }))}
        />
        <Button
          disabled={move.isPending || deals.length === 0 || stageId === ""}
          onClick={moveStage}
        >
          {t("deals.bulkMove")}
        </Button>
      </BulkVerbs>
      {failed.length > 0 && (
        <ErrorLine inline>
          {t("deals.bulkFailed", {
            count: formatNumber(failed.length, locale),
          })}{" "}
          {failed
            .map((outcome) => `${outcome.name}: ${outcome.error}`)
            .join(" · ")}
        </ErrorLine>
      )}
    </>
  );
}
