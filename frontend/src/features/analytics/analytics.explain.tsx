import { useQuery } from "@tanstack/react-query";
import { createContext, type ReactNode, useContext, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef, useEntityName } from "../../components/entityref";
import { Button, SectionHeader, Skeleton } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Stack } from "../../design-system/stack";
import { rowsState, SurfaceState } from "../../design-system/surfacestate";
import { formatAnalyticsNumber, formatDateTime } from "../../format/format";
import { zoneLabelAt } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import {
  derivationHeader,
  renderDerivationCell,
  shownDerivationColumns,
} from "./analytics.explain.cells";
import { DerivationDefinition } from "./analytics.explain.definition";
import { ExplainDrawer } from "./analytics.explain.drawer";
import { moneyFieldsOf } from "./analytics.questions.money";
import { useAnalyticsSchema } from "./analytics.schema";

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
    const [only] = values;
    if (only === undefined) continue;
    extra[key] = values.length > 1 ? values : only;
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
      return unwrap(
        await api.GET("/reports/{report}/derivation", {
          params: {
            path: { report: derivationReportKey(url ?? "") },
            query: parseDerivationQuery(url ?? ""),
          },
        }),
      );
    },
  });
}

// The source rows the figure reconciles to. The server caps them and says how
// many matched, so a capped list states the rest instead of reading as whole.
function DerivationRows({
  derivation,
  baseCurrency,
}: Readonly<{ derivation: Derivation; baseCurrency: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const schema = useAnalyticsSchema();
  const columns = shownDerivationColumns(derivation);
  const { rows } = derivation;
  const { state, remaining } = rowsState(rows.length, derivation.total_rows);
  const position = new Map(rows.map((row, index) => [row, index]));
  return (
    <>
      <SectionHeader title={t("explain.sources")} level={3} />
      <SurfaceState
        state={state}
        emptyLabel={t("common.empty")}
        loadingLabel={t("explain.sources")}
        detail={{ remaining }}
      >
        {rows.length > 0 && (
          // No amount is drawn before the schema says which columns are money.
          <QueryGate query={schema} pendingLabel={t("common.loading")}>
            {(vocabulary) => (
              <DataTable
                label={t("explain.sources")}
                columns={columns.map((col) => {
                  const money = moneyFieldsOf(
                    vocabulary.entities,
                    derivation.report,
                  );
                  return {
                    key: col,
                    header: derivationHeader(col, derivation.report, {
                      baseCurrency,
                      money,
                      t,
                    }),
                    render: (row: Record<string, unknown>) =>
                      renderDerivationCell(col, row, {
                        baseCurrency,
                        money,
                        locale,
                      }),
                  };
                })}
                rows={rows}
                rowKey={(row) =>
                  typeof row.id === "string"
                    ? `id:${row.id}`
                    : `at:${position.get(row)}`
                }
              />
            )}
          </QueryGate>
        )}
      </SurfaceState>
    </>
  );
}

// A field mask took these records out of the figure AND the rows, so the two
// reconcile; said, so a smaller number reads as governed rather than missing.
export function ExcludedNote({
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
          count: formatAnalyticsNumber(count, locale),
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
      {/* What the figure MEANS: the reader's words where the plan allows. */}
      {query.data ? (
        <DerivationDefinition
          derivation={query.data}
          unset={[parseDerivationQuery(url ?? "").isnull ?? []].flat()}
        />
      ) : (
        <p className="t-sub">{t("analytics.planNote")}</p>
      )}
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
            zone: zoneLabelAt(asOf, timezone, locale),
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
  if (url == null) {
    return <>{children}</>;
  }
  return (
    <ExplainDrawer
      figure={figure}
      body={(opens) => <ExplainBody key={opens} url={url} framed />}
    >
      {children}
    </ExplainDrawer>
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
