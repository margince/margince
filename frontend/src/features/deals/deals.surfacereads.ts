import { listFetchLimit } from "../../components/listquery";
import {
  type DealFilters,
  useDeals,
  useStageTotals,
} from "../../lib/deals.queries";
import type { usePipelines } from "../../lib/pipelines.queries";
import { useStageColumns } from "./dealboard.queries";
import type { Stage } from "./deals";

// Each surface reads only while it shows, so neither pays for the other.
// Their pipeline scope arrives with the pipelines read. Asking before it lands
// asks for every deal and throws the answer away; no pipelines answers `[]`,
// which is knowing. The table waits on that read, so it shows its failure.
export function useSurfaceReads(
  f: DealFilters,
  stages: readonly Stage[],
  view: "board" | "table",
  refused: boolean,
  pipelinesQuery: ReturnType<typeof usePipelines>,
  perPage: number,
) {
  const scopeKnown = pipelinesQuery.data !== undefined;
  const dealsQuery = useDeals(
    f,
    view === "table" && !refused && scopeKnown,
    listFetchLimit(perPage),
  );
  const stageColumns = useStageColumns(
    f,
    stages.map((stage) => stage.id),
    view === "board" && !refused,
  );
  // The board's column totals: a per-stage server aggregate over every
  // matching deal, not only the pages the columns have read. Built from the
  // same filter dials, so cards and totals agree on which deals are in view.
  // The table draws no column header to put one in.
  const stageTotalsQuery = useStageTotals(
    f,
    view === "board" && !refused && scopeKnown,
  );
  const readError = view === "board" ? stageColumns.error : dealsQuery.error;
  const listRead = scopeKnown ? dealsQuery : pipelinesQuery;
  return { dealsQuery, stageColumns, stageTotalsQuery, readError, listRead };
}
