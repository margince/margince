import { useState } from "react";
import type { components } from "../../api/schema";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
} from "../../components/bulkchange";
import { BulkVerbs } from "../../components/bulkverbs";
import { Button } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { stable } from "../../format/collate";
import { useT } from "../../i18n";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

/**
 * Bulk verbs over selected deals: assign an owner, move to a stage, archive.
 *
 * All three are server bulk changes. Each opens the shared `BulkChangeDialog`.
 * It previews what would change and what would be left alone, runs one audited
 * change with each deal's own version, and offers Undo. A move keeps the rules
 * of the single-record advance per deal, so a deal one refuses is listed in
 * the preview and not moved.
 *
 * Only OPEN stages are offered. Closing a deal asks for a lost reason and
 * freezes an exchange rate. One reason standing for a dozen deals behind one
 * button is not a thing this bar should make easy.
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
  /** Called after any change ran, so the caller clears the selection. */
  onDone: () => void;
}>) {
  const t = useT();
  const [stageId, setStageId] = useState("");
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  const openStages = stages.filter((stage) => stage.semantic === "open");
  const stage = openStages.find((each) => each.id === stageId);

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
    // An open dialog would otherwise describe the new selection with no stage.
    setRequest(null);
  }

  const moveTo = (target: Stage) =>
    setRequest({
      recordType: "deal",
      verb: "move_stage",
      rows: deals.map((deal) => ({
        id: deal.id,
        version: deal.version,
        label: deal.name,
      })),
      stage: { id: target.id, name: target.name },
      openId: crypto.randomUUID(),
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
        onDone={onDone}
        childrenRequire={{ object: "deal", action: "update" }}
      >
        {(reasonId) => (
          <>
            <Select
              aria-label={t("deals.bulkStage")}
              value={stageId}
              placeholder={t("deals.bulkStagePick")}
              disabled={reasonId !== undefined}
              aria-describedby={reasonId}
              onChange={setStageId}
              options={openStages.map((each) => ({
                value: each.id,
                label: each.name,
              }))}
            />
            <Button
              disabled={deals.length === 0 || stage === undefined}
              reasonId={reasonId}
              onClick={() => stage && moveTo(stage)}
            >
              {t("deals.bulkMove")}
            </Button>
          </>
        )}
      </BulkVerbs>
      <BulkChangeDialog
        request={request}
        onClose={() => setRequest(null)}
        onDone={() => {
          setRequest(null);
          setStageId("");
          onDone();
        }}
      />
    </>
  );
}
