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
import { EntityRef, useEntityName } from "./entityref";

// "Explain this number": where a figure on the Analytics screen came from. ONE
// body, two hosts: the panel under a report card explains the whole result, and
// the drawer a row opens explains that row's cell. Both resolve a server-minted
// `derivation_url`, so the two can differ only in which handle they were given.

type Derivation = components["schemas"]["ReportDerivation"];
type ReportRow = components["schemas"]["ReportResult"]["rows"][number];

// The frame a report was cut in, set once by the card that ran it, so source
// rows are written in the figure's money and dated in its zone.
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

// A handle's query string as the typed client's query: every key is forwarded,
// and a repeated one (`isnull` per unset group key) stays a list of all its values.
export function parseDerivationQuery(
  url: string,
): { by: string[]; agg: string[] } & Record<string, string | string[]> {
  const qs = new URLSearchParams(url.split("?")[1] ?? "");
  const extra: Record<string, string | string[]> = {};
  for (const key of new Set(qs.keys())) {
    const values = qs.getAll(key);
    extra[key] = values.length > 1 ? values : values[0];
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
      // Every predicate is forwarded: dropping one explains a broader slice
      // than the figure, or 422s on a bound grouping dimension.
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

// The vocabulary's words for a drill-through's columns. A column outside it
// keeps its wire name: what the plan selected, not a guess at what it meant.
const DERIVATION_HEADERS: Readonly<Record<string, MessageKey>> = {
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

// What a source row IS, per report: the label names the record the row stands
// for, and the three delivery reports count projects rather than deals.
const RECORD_NOUN: Readonly<Record<string, MessageKey>> = {
  "projects-by-phase": "analytics.project",
  "project-commitments": "analytics.project",
  "projects-gone-quiet": "analytics.project",
};

function derivationHeader(
  col: string,
  report: string,
  t: (key: MessageKey) => string,
): string {
  const key =
    col === "label"
      ? (RECORD_NOUN[report] ?? "explain.col.record")
      : DERIVATION_HEADERS[col];
  return key ? t(key) : col;
}

// The id is noise beside a name, but only once EVERY row has one: a name is
// withheld per row, and dropping the id then blanks that row's only identifier.
// Which record a row IS leads, whatever order the plan selected, because the
// columns past a narrow drawer's edge are the ones a reader never sees.
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

// A `_base_minor` measure was converted into the base currency; a plain `_minor`
// is the deal's OWN amount, in the currency its row carries beside it.
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

// Money is stored in minor units, and 500000 printed raw beside €5,000.00 is the
// most misread thing on this screen.
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

// The source rows the figure reconciles to. The server caps them and says how
// many matched, so a capped list states the rest instead of reading as whole.
function DerivationRows({
  derivation,
  baseCurrency,
}: Readonly<{ derivation: Derivation; baseCurrency: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const columns = derivationColumns(derivation);
  const { rows } = derivation;
  const remaining = (derivation.total_rows ?? 0) - rows.length;
  const position = new Map(rows.map((row, index) => [row, index]));
  return (
    <>
      <SectionHeader title={t("explain.sources")} level={3} />
      <SurfaceState
        state={rowsState(rows.length, remaining)}
        emptyLabel={t("common.empty")}
        loadingLabel={t("explain.sources")}
        detail={{ remaining }}
      >
        {rows.length > 0 && (
          <DataTable
            label={t("explain.sources")}
            columns={columns.map((col) => ({
              key: col,
              header: derivationHeader(col, derivation.report, t),
              render: (row: Record<string, unknown>) =>
                renderDerivationCell(col, row, baseCurrency, locale),
            }))}
            rows={rows}
            rowKey={(row) =>
              typeof row.id === "string"
                ? `id:${row.id}`
                : `at:${position.get(row)}`
            }
          />
        )}
      </SurfaceState>
    </>
  );
}

// "None" only when the server matched none: a capped answer that returned no
// rows still matched some, and says how many are not shown.
function rowsState(
  shown: number,
  remaining: number,
): "empty" | "partial" | "ready" {
  if (remaining > 0) return "partial";
  return shown === 0 ? "empty" : "ready";
}

// A field mask took these records out of the figure AND the rows, so the two
// reconcile; said, so a smaller number reads as governed rather than missing.
function ExcludedNote({
  count,
}: Readonly<{ count: number | null | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (count == null || count <= 0) {
    return null;
  }
  return (
    <SurfaceState
      state="withheld"
      emptyLabel={t("common.empty")}
      loadingLabel={t("explain.sources")}
      detail={{
        withheldReason: plural("explain.excluded", count, {
          count: formatNumber(count, locale),
        }),
      }}
    >
      {null}
    </SurfaceState>
  );
}

// Everything a host shows about one handle, in the order a doubting reader
// needs it: what the figure means, the caveats, then the rows and their frame.
function ExplainBody({
  url: opened,
  framed,
}: Readonly<{ url: string | null; framed: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const live = useContext(FrameContext);
  // The handle AND its frame as they were when this opened: a refetch mints a
  // new handle, and old rows must not be written in a new frame's money or zone.
  const [{ url, frame }] = useState(() => ({ url: opened, frame: live }));
  const { baseCurrency, timezone } = frame;
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
      {framed && asOf && timezone && (
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

// The host under a report card: the whole result's handle. It draws no frame of
// its own, because the card above it already states the one it was cut in.
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
          <ExplainBody url={url} framed={false} />
        </PanelBody>
      </Panel>
    </div>
  );
}

// The host beside one row or tile: that cell's handle, in a drawer so the table
// stays legible behind it. `figure` names the trigger and the drawer alike;
// `children` is the cell the trigger sits beside, alone where there is no handle.
export function CellExplain({
  url,
  figure,
  children,
}: Readonly<{ url: string | null; figure: string; children?: ReactNode }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // Counts the opens, so each one mounts a fresh body that takes the handle
  // and frame of that moment, even while the last close is still animating.
  const [opens, setOpens] = useState(0);
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
        onClick={() => {
          setOpens((count) => count + 1);
          setOpen(true);
        }}
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
        <ExplainBody key={opens} url={url} framed />
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

// A company row's trigger. The name is a read of its own, shared with the
// reference beside it; a company, or the no-company group, split across two
// currencies is two rows, and the code is what tells their triggers apart.
export function CompanyCellExplain({
  url,
  currency,
  companyId,
  split,
}: Readonly<{
  url: string | null;
  currency: string | null;
  companyId: string | null;
  split: boolean;
}>) {
  const t = useT();
  const named = useEntityName("company", companyId).name;
  const company =
    companyId == null
      ? t("analytics.noCompany")
      : (named ?? t("analytics.company"));
  return (
    <CellExplain
      url={url}
      figure={split && currency ? `${company} ${currency}` : company}
    >
      {companyId == null ? (
        // Deals with no company at all, grouped into one row: a fact about the
        // data rather than a permission, so it says "none".
        <span>{company}</span>
      ) : (
        <EntityRef kind="company" id={companyId} />
      )}
    </CellExplain>
  );
}
