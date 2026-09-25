import { useQuery } from "@tanstack/react-query";
import { Info } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useContext,
  useId,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, SectionHeader, Skeleton } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { Modal } from "../design-system/modal";
import { Panel, PanelBody } from "../design-system/panel";
import { Row, Stack } from "../design-system/stack";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDateTime,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { EntityRef } from "./entityref";

// "Explain this number": where a figure on the Analytics screen came from. ONE
// body, two hosts: the panel under a report card explains the whole result, and
// the drawer a row opens explains that row's cell. Both resolve a server-minted
// `derivation_url`, so the two can differ only in which handle they were given.

type Derivation = components["schemas"]["ReportDerivation"];
type ReportRow = components["schemas"]["ReportResult"]["rows"][number];

// The frame a report was cut in, set once by the card that ran it. The source
// rows are written in the same money as the figure they explain and dated in
// the same zone, so every host reads the frame rather than being handed it.
type ExplainFrameValue = Readonly<{
  baseCurrency: string | null;
  timezone: string | null;
}>;

const FrameContext = createContext<ExplainFrameValue>({
  baseCurrency: null,
  timezone: null,
});

export function ExplainFrame({
  frame,
  children,
}: Readonly<{ frame: ExplainFrameValue; children: ReactNode }>) {
  return (
    <FrameContext.Provider value={frame}>{children}</FrameContext.Provider>
  );
}

// A report row's own handle, with its group keys bound. A row the server sent
// without one draws no trigger rather than one that explains something else.
export function rowDerivationUrl(row: ReportRow): string | null {
  const url = row.derivation_url;
  return typeof url === "string" && url !== "" ? url : null;
}

// Parse a server-minted `derivation_url` into the typed derivation query.
// The generated client's derivation query is ONLY `{ by?, agg? }` (no
// predicate params, no index signature), so callers forward just those two;
// the extra predicate keys ride along on the return value for inspection
// only (spec constraint 6: never raw-fetch the URL itself).
export function parseDerivationQuery(
  url: string,
): { by: string[]; agg: string[] } & Record<string, unknown> {
  const qs = new URLSearchParams(url.split("?")[1] ?? "");
  const extra: Record<string, unknown> = {};
  for (const [k, v] of qs.entries()) {
    if (k !== "by" && k !== "agg") extra[k] = v;
  }
  return { ...extra, by: qs.getAll("by"), agg: qs.getAll("agg") };
}

// The derivation URL's path names the report key (prebuilt or saved-report
// id) the typed path param expects.
function derivationReportKey(url: string): string {
  return url.match(/reports\/([^/?]+)\/derivation/)?.[1] ?? "";
}

function useDerivation(url: string | null) {
  return useQuery({
    queryKey: ["derivation", url],
    enabled: url != null,
    queryFn: async () => {
      // parsed carries by/agg PLUS every equality predicate from the handle
      // (group-key values + plan filters). The endpoint treats each extra key
      // as a predicate, so forward the whole object — dropping the predicates
      // would explain the wrong slice (or 422 on a bound grouping dimension).
      const { data, error } = await api.GET("/reports/{report}/derivation", {
        params: {
          path: { report: derivationReportKey(url ?? "") },
          query: parseDerivationQuery(url ?? ""),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The vocabulary's own words for the columns a drill-through can carry.
// A column outside it keeps its wire name, which is honest: the reader sees
// what the plan selected rather than a guess at what it meant.
const DERIVATION_HEADERS: Readonly<Record<string, MessageKey>> = {
  label: "explain.col.record",
  amount_base_minor: "analytics.unweighted",
  weighted_base_minor: "analytics.weighted",
  amount_minor: "analytics.unweighted",
  currency: "analytics.currency",
  stage_id: "explain.col.stage",
  owner_id: "explain.col.owner",
  pipeline_id: "explain.col.pipeline",
  company_id: "analytics.company",
  partner_company_id: "analytics.company",
};

function derivationHeader(col: string, t: (key: MessageKey) => string): string {
  const key = DERIVATION_HEADERS[col];
  return key ? t(key) : col;
}

// The server names the row and the reader reads the name, so the raw id
// becomes noise beside it — but only once EVERY row has a name.
//
// Labelling is per row: the seam withholds a name for a record this reader may
// not read, and the column appears as soon as one row was named. Dropping the
// id on that alone would blank the withheld rows' only identifier, so the rows
// a reader can least account for become the ones they cannot identify at all.
//
// Which record a row IS leads, whatever order the plan selected: in a narrow
// drawer the columns past the edge are the ones scrolled to, and a row showing
// its owner and currency but not its name is a row nobody can place.
export function derivationColumns(derivation: Derivation): string[] {
  const rows = derivation.rows ?? [];
  const everyRowNamed =
    derivation.columns.includes("label") &&
    rows.length > 0 &&
    rows.every((row) => typeof row.label === "string" && row.label !== "");
  const shown = derivation.columns.filter(
    (col) => !everyRowNamed || col !== "id",
  );
  const identity = IDENTITY_COLUMNS.filter((col) => shown.includes(col));
  return [...identity, ...shown.filter((col) => !identity.includes(col))];
}

const IDENTITY_COLUMNS = ["label", "id"];

// Which money a row's minor-unit figure is written in.
//
// The two are not the same column. A `_base_minor` measure was converted by the
// server, so it is in the installation's base currency; a plain `_minor` is the
// deal's OWN amount, and the forecast's rows carry that currency beside it —
// reading the base currency there would put a euro sign on a dollar deal.
export function derivationCellCurrency(
  col: string,
  row: Record<string, unknown>,
  baseCurrency: string | null,
): string | null {
  if (col.endsWith("_base_minor")) {
    return baseCurrency;
  }
  const own = row.currency;
  return typeof own === "string" && own !== "" ? own : null;
}

// Money on these rows is stored in minor units, and a minor-unit integer
// printed raw is the single most misread thing on this screen: 500000 next
// to €5,000.00 are the same number wearing different clothes.
function renderDerivationCell(
  col: string,
  row: Record<string, unknown>,
  baseCurrency: string | null,
  locale: Locale,
): ReactNode {
  const value = row[col];
  if (value == null) {
    return "";
  }
  if (typeof value === "string" && value !== "") {
    if (col === "pipeline_id")
      return <DerivationPipelineName pipelineId={value} />;
    if (col === "stage_id" && typeof row.pipeline_id === "string") {
      return (
        <DerivationPipelineName pipelineId={row.pipeline_id} stageId={value} />
      );
    }
    if (col === "owner_id") return <EntityRef kind="user" id={value} />;
    if (col === "company_id" || col === "partner_company_id") {
      return <EntityRef kind="company" id={value} />;
    }
  }
  if (col.endsWith("_minor") && typeof value === "number") {
    return formatMoneyOrAbsent(
      value,
      derivationCellCurrency(col, row, baseCurrency),
      locale,
    );
  }
  return String(value);
}

function DerivationPipelineName({
  pipelineId,
  stageId,
}: Readonly<{ pipelineId: string; stageId?: string }>) {
  const t = useT();
  const pipeline = useQuery({
    queryKey: ["pipeline", pipelineId],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines/{id}", {
        params: { path: { id: pipelineId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  if (pipeline.isPending) return <>{t("common.loading")}</>;
  if (pipeline.isError) return <>{t("common.error")}</>;
  return (
    <>
      {stageId
        ? (pipeline.data?.stages?.find((stage) => stage.id === stageId)?.name ??
          t("common.empty"))
        : pipeline.data?.name}
    </>
  );
}

// The source rows the explained figure reconciles to. The server caps them at
// its row limit and says how many matched, so a capped list states the rest
// under it instead of reading as the whole set.
function DerivationRows({
  derivation,
  baseCurrency,
}: Readonly<{ derivation: Derivation; baseCurrency: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const columns = derivationColumns(derivation);
  const remaining = (derivation.total_rows ?? 0) - derivation.rows.length;
  return (
    <>
      <SectionHeader title={t("explain.sources")} level={3} />
      <SurfaceState
        state={derivation.rows.length === 0 ? "empty" : partialOr(remaining)}
        emptyLabel={t("common.empty")}
        loadingLabel={t("explain.sources")}
        detail={{ remaining }}
      >
        <DataTable
          label={t("explain.sources")}
          columns={columns.map((col) => ({
            key: col,
            header: derivationHeader(col, t),
            render: (row: Record<string, unknown>) =>
              renderDerivationCell(col, row, baseCurrency, locale),
          }))}
          rows={derivation.rows}
          rowKey={(row) => derivation.rows.indexOf(row).toString()}
        />
      </SurfaceState>
    </>
  );
}

function partialOr(remaining: number): "partial" | "ready" {
  return remaining > 0 ? "partial" : "ready";
}

// A field mask took these records out of the figure AND out of the rows, so the
// two still reconcile. Said, because an unexplained smaller number reads as
// missing data rather than as a permission boundary.
function ExcludedNote({
  count,
}: Readonly<{ count: number | null | undefined }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (count == null || count <= 0) {
    return null;
  }
  return (
    <p className="surfacestate-withheld">
      {plural("explain.excluded", count, {
        count: formatNumber(count, locale),
      })}
    </p>
  );
}

// Everything a host shows about one handle, in the order a doubting reader
// needs it: what the figure means, the caveats, then the rows and their frame.
function ExplainBody({ url }: Readonly<{ url: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const { baseCurrency, timezone } = useContext(FrameContext);
  const query = useDerivation(url);
  const asOf = query.data?.as_of;
  return (
    <>
      {/* What the figure MEANS, in the server's own words. */}
      <p className="t-sub">
        {query.data?.definition ?? t("analytics.planNote")}
      </p>
      {url == null && <p>{t("common.empty")}</p>}
      {url != null && query.isPending && (
        <Stack gap="2">
          <Skeleton width="60%" />
          <Skeleton width="90%" />
        </Stack>
      )}
      {query.isError && (
        <>
          <ErrorLine error={query.error} />
          <div className="card-actions">
            <Button onClick={() => query.refetch()}>{t("common.retry")}</Button>
          </div>
        </>
      )}
      {/* A link minted before the handle carried an instant. The figures below
          were recomputed at a NEW moment, so a rate sheet effective in between
          makes them disagree with the number they explain — and a reader
          already doubting a figure takes that as proof, not as a discrepancy. */}
      {query.data?.as_of_pinned === false && (
        <p className="surfacestate-stale">{t("explain.mayHaveMoved")}</p>
      )}
      {query.data && (
        <>
          <ExcludedNote count={query.data.excluded_by_permission} />
          <DerivationRows derivation={query.data} baseCurrency={baseCurrency} />
        </>
      )}
      {asOf && timezone && (
        <p className="t-caption">
          {t("analytics.frame", {
            asOf: formatDateTime(asOf, locale, timezone),
            zone: timezone,
          })}
        </p>
      )}
    </>
  );
}

// The host under a report card: the whole result's handle.
export function ExplainPanel({
  id,
  url,
}: Readonly<{ id: string; url: string | null }>) {
  const t = useT();
  return (
    // The toggle names this panel with `aria-controls` and Panel mints its own
    // ids, so the handle the toggle was given lives on the wrapper.
    <div id={id}>
      <Panel title={t("explain.title")}>
        <PanelBody>
          <ExplainBody url={url} />
        </PanelBody>
      </Panel>
    </div>
  );
}

// The host beside one row or tile: that cell's handle, in a drawer so the table
// it explains stays legible behind it. `figure` names the row, and it is both
// the trigger's name and the drawer's second line, so a reader told "Explain
// Qualify" lands on a drawer that says Qualify. `children` is the cell the
// trigger sits beside, drawn alone where the row carries no handle.
export function CellExplain({
  url,
  figure,
  children,
}: Readonly<{ url: string | null; figure: string; children?: ReactNode }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const figureId = useId();
  if (url == null) {
    return <>{children}</>;
  }
  const trigger = (
    <>
      <IconAction
        inline
        label={t("explain.cell", { figure })}
        icon={<Info aria-hidden />}
        onClick={() => setOpen(true)}
      />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={`${titleId} ${figureId}`}
        placement="right"
      >
        <Heading size="large" id={titleId}>
          {t("explain.title")}
        </Heading>
        <p className="t-label" id={figureId}>
          {figure}
        </p>
        <ExplainBody url={url} />
      </Modal>
    </>
  );
  if (children === undefined) {
    return trigger;
  }
  return (
    <Row gap="1" wrap={false}>
      {children}
      {trigger}
    </Row>
  );
}
