import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, Fragment, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanUpsert } from "../app/capability";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { today } from "../format/calendarday";
import { forReader } from "../format/collate";
import { formatDate, formatUsdPerMTok } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { providerName } from "./ai-provider-names";
import {
  problemMessageOf,
  QueryGate,
  QueryStates,
  unwrap,
  useMe,
  WriteRefused,
} from "./common";
import {
  RefreshModelPricesButton,
  RefreshSummary,
  useRefreshModelPrices,
} from "./rate-catalogue-refresh";
import { isPricedProvider, ModelPriceDialog } from "./rate-manual";
import { RefreshFromSources } from "./rate-refresh";
import "./rates.css";

type FxRate = components["schemas"]["FxRate"];
type AiModelRate = components["schemas"]["AiModelRate"];

// trimDecimal drops trailing zeros (and a bare trailing dot) so a
// numeric(20,10) value like "0.9200000000" reads as "0.92".
function trimDecimal(value: string): string {
  if (!value.includes(".")) {
    return value;
  }
  // Lookbehind: the run of zeros is found once, not retried from each zero in
  // it. numeric(20,10) is short, but the shape is the one that hangs a tab.
  return value.replace(/(?<!0)0+$/, "").replace(/\.$/, "");
}

// Withheld, not absent (design-system README, "Absent, disabled, or withheld"):
// a PERMISSION is what denies a price sheet — fx_rate and ai_model_rate grants
// exist only for admin and ops — so the card keeps its place and says so. Both
// sheets sit on settings pages other roles open for their other cards (currency
// rates on Company, model prices on AI), and a sheet that vanished there
// would read as "this installation has no rates" rather than "not yours to see".
//
// It asks the server for NOTHING: each caller keeps `enabled: canRead` on its
// list query, because the answer is already known and fetching the 403 in order
// to render it turns a settled denial into red error text with a Retry that can
// only be refused again. Gated on the /me probe so the notice waits for the
// grants instead of flashing at every reader while they are still in flight.
function WithheldRateCard({
  title,
  reason,
}: Readonly<{ title: string; reason: string }>) {
  const me = useMe();
  return (
    <Panel title={title}>
      <PanelBody>
        <QueryGate query={me} pendingLabel={title}>
          {() => <EmptyState>{reason}</EmptyState>}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// ---- FX rates ----

export function FxRatesCard() {
  const t = useT();
  // Reading the sheet and authoring it are separate grants, so the card asks
  // for each one where it needs it.
  //
  // `useCan` for the read and `useCanUpsert` for the write, which additionally
  // folds the licensing seat — a difference that decides the read-seat case:
  // an admin on a read seat still holds fx_rate:read, so they get the table and
  // the read-only caption below, never the withheld body. Their denial is about
  // WRITING, and saying "only an admin or ops can see this" to an admin who is
  // looking at it would be a lie.
  const canRead = useCan("fx_rate", "read");
  // Either write grant, because the endpoint is one upsert: setting a rate for
  // a (currency, day) inserts under fx_rate:create and replaces an existing one
  // under fx_rate:update, and which it will be is only known once the server
  // has read the row. It admits on either and demands the specific verb inside
  // the transaction, so asking for one here would hide the editor from a
  // principal the server would have let write.
  const canManage = useCanUpsert("fx_rate");
  const [open, setOpen] = useState(false);
  const query = useQuery({
    queryKey: ["fx-rates"],
    enabled: canRead,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/fx-rates", {
          params: { query: {} },
        }),
      );
      return data.data;
    },
  });

  // After every hook, so the number of hooks a render runs stays the same. A
  // principal holding a write verb but not the read lands here too: without the
  // sheet there is nothing to author a new rate against, so the read decides
  // whether this card exists at all.
  if (!canRead) {
    return (
      <WithheldRateCard
        title={t("settings.rates.fxTitle")}
        reason={t("settings.rates.fxWithheld")}
      />
    );
  }

  return (
    // The two verbs ride in the panel's own action band under the sheet they
    // change, not beside the title. Beside the title they were one unwrappable
    // row sized to its max content: at 390px the pair measured 353px inside a
    // 324px card, pushed the page 12px past the viewport and squeezed the
    // title to nothing. The band wraps, so the same two controls cannot widen
    // the card at any viewport.
    <Panel
      title={t("settings.rates.fxTitle")}
      actions={
        canManage ? (
          <>
            <RefreshFromSources path="/fx-rates/propose-refresh" />
            <Button variant="primary" onClick={() => setOpen(true)}>
              {t("settings.rates.fxAdd")}
            </Button>
          </>
        ) : null
      }
    >
      <PanelBody className="form-stack">
        <PanelIntro>{t("settings.rates.fxIntro")}</PanelIntro>
        {/* A sheet whose write affordances are all withheld says so ONCE, here,
            rather than annotating each absent control. The rule (design-system
            README): a permission-withheld SURFACE states it, while individual
            write affordances inside a readable surface may simply be absent —
            provided the surface has said what a reader is looking at. Without
            this the page was a rate table with no editor and no reason given,
            which reads as a bug rather than as a permission.
            This is the READ-GRANTED case alone: the withheld branch has already
            returned, so `!canManage` here means the reader may see the sheet and
            not change it — no write verb on the object, or a read licensing seat.
            On the withheld body these two lines would explain one denial twice,
            in two different ways. */}
        {!canManage && <p>{t("settings.rates.readOnly")}</p>}
        <SettingList>
          {/* The sheet IS this card's subject rather than an answer that fits
              beside a label, so it takes the full width below the naming
              (design-system README, SettingRow: what picks `stack` is
              complexity, not size). The row names what a reader is looking at
              — the rates currently in force — which the table's own column
              headers do not say. */}
          <SettingRow
            label={t("settings.rates.fxTableLabel")}
            layout="stack"
            control={
              <QueryGate
                query={query}
                pendingLabel={t("settings.rates.fxTableLabel")}
              >
                {(rows) =>
                  rows.length === 0 ? (
                    <EmptyState>
                      <b>{t("settings.rates.fxEmpty")}</b>
                    </EmptyState>
                  ) : (
                    <DataTable<FxRate>
                      label={t("settings.rates.fxTableLabel")}
                      rows={rows}
                      rowKey={(row) => row.from_currency}
                      columns={[
                        {
                          key: "from",
                          header: t("settings.rates.colFrom"),
                          render: (row) => row.from_currency,
                        },
                        {
                          key: "rate",
                          header: t("settings.rates.colRate", {
                            base: rows[0]?.to_currency ?? "",
                          }),
                          render: (row) => trimDecimal(row.rate),
                        },
                        {
                          key: "effective",
                          header: t("settings.rates.colEffective"),
                          render: (row) => row.effective_date,
                        },
                      ]}
                    />
                  )
                }
              </QueryGate>
            }
          />
        </SettingList>
        {open ? <FxRateModal onClose={() => setOpen(false)} /> : null}
      </PanelBody>
    </Panel>
  );
}

function FxRateModal({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const qc = useQueryClient();
  const labelId = useId();
  const formId = useId();
  const [from, setFrom] = useState("");
  const [rate, setRate] = useState("");
  const [effectiveDate, setEffectiveDate] = useState(today());
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.POST("/fx-rates", {
          body: {
            from_currency: from.trim().toUpperCase(),
            rate: rate.trim(),
            effective_date: effectiveDate,
          },
        }),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["fx-rates"] });
      onClose();
    },
    onError: (err: Error) => setError(problemMessageOf(err, t)),
  });
  const ready = from.trim() !== "" && rate.trim() !== "";
  function submit(event: FormEvent) {
    event.preventDefault();
    if (!ready || save.isPending) return;
    setError(null);
    save.mutate();
  }

  return (
    <Modal open onClose={onClose} labelledBy={labelId} intent="form">
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {t("settings.rates.fxModalTitle")}
      </Heading>
      <form id={formId} className="form-stack" onSubmit={submit}>
        <Field label={t("settings.rates.colFrom")}>
          {(control) => (
            <TextInput
              {...control}
              value={from}
              maxLength={3}
              onChange={(e) => setFrom(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("settings.rates.rateToBase")}>
          {(control) => (
            <TextInput
              {...control}
              value={rate}
              inputMode="decimal"
              placeholder="0.92"
              onChange={(e) => setRate(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("settings.rates.colEffective")}>
          {(control) => (
            <TextInput
              {...control}
              type="date"
              min={today()}
              value={effectiveDate}
              onChange={(e) => setEffectiveDate(e.target.value)}
            />
          )}
        </Field>
        <WriteRefused titleKey="settings.rates.notSaved" message={error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!ready || save.isPending}
        >
          {t("settings.rates.setRate")}
        </Button>
      </div>
    </Modal>
  );
}

// ---- AI model costs ----

export function ModelCostsCard() {
  const t = useT();
  // Read and write asked separately, for the same reasons as FxRatesCard above —
  // including the read seat, which withholds the WRITE from an admin who plainly
  // still reads the sheet.
  const canRead = useCan("ai_model_rate", "read");
  // Either write grant, for the same reason as FxRatesCard above: one endpoint,
  // insert or replace, the specific verb resolved inside the transaction.
  const canManage = useCanUpsert("ai_model_rate");
  const [open, setOpen] = useState(false);
  const refresh = useRefreshModelPrices();
  const query = useQuery({
    queryKey: ["ai-model-rates"],
    enabled: canRead,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/ai-model-rates", {
          params: { query: {} },
        }),
      );
      return data.data;
    },
  });

  const priced = query.data?.filter((row) => isPricedProvider(row.provider));

  // After every hook, as in FxRatesCard.
  if (!canRead) {
    return (
      <WithheldRateCard
        title={t("settings.rates.modelTitle")}
        reason={t("settings.rates.modelWithheld")}
      />
    );
  }

  return (
    // The verbs in the action band, for the reason on FxRatesCard: beside the
    // title they were an unwrappable row wider than a 390px viewport.
    <Panel
      title={t("settings.rates.modelTitle")}
      actions={
        canManage ? (
          <>
            <RefreshModelPricesButton refresh={refresh} />
            <Button variant="primary" onClick={() => setOpen(true)}>
              {t("settings.rates.modelAdd")}
            </Button>
          </>
        ) : null
      }
    >
      <PanelBody className="form-stack">
        <PanelIntro>{t("settings.rates.modelIntro")}</PanelIntro>
        {/* Said once for the sheet, for the reason on FxRatesCard. */}
        {!canManage && <p>{t("settings.rates.readOnly")}</p>}
        {query.data === undefined && (
          <QueryStates
            query={query}
            pendingLabel={t("settings.rates.modelTableLabel")}
          >
            {null}
          </QueryStates>
        )}
        {priced?.length === 0 && (
          <EmptyState>
            <b>{t("settings.rates.modelEmpty")}</b>
          </EmptyState>
        )}
      </PanelBody>
      {priced && <ModelPriceGroups rows={priced} />}
      {open ? <ModelPriceDialog onClose={() => setOpen(false)} /> : null}
      {/* Under the sheet and above the band that holds the button: what the
          last refresh did, or why it was refused. */}
      <RefreshSummary refresh={refresh} />
    </Panel>
  );
}

function ModelPriceGroups({
  rows,
}: Readonly<{ rows: readonly AiModelRate[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const groups = new Map<string, AiModelRate[]>();
  for (const row of rows) {
    groups.set(row.provider, [...(groups.get(row.provider) ?? []), row]);
  }
  const named = [...groups]
    .map(([provider, prices]) => ({
      provider,
      name: providerName(provider, t),
      prices,
    }))
    .sort((a, b) => forReader(a.name, b.name, locale));
  const price = (
    key: string,
    header: string,
    pick: (row: AiModelRate) => string,
  ): DataTableColumn<AiModelRate> => ({
    key,
    header,
    align: "end",
    render: (row) => formatUsdPerMTok(pick(row), locale),
  });
  const columns: DataTableColumn<AiModelRate>[] = [
    {
      key: "model",
      header: t("settings.rates.colModel"),
      grow: true,
      render: (row) => <code>{row.model_id}</code>,
    },
    price("in", t("settings.rates.colIn"), (row) => row.input_per_mtok),
    price("out", t("settings.rates.colOut"), (row) => row.output_per_mtok),
    price(
      "cacheRead",
      t("settings.rates.colCacheReadShort"),
      (row) => row.cache_read_per_mtok,
    ),
    price(
      "cacheWrite",
      t("settings.rates.colCacheWriteShort"),
      (row) => row.cache_write_per_mtok,
    ),
    {
      key: "effective",
      header: t("settings.rates.colEffective"),
      render: (row) => formatDate(row.effective_date, locale, viewerZone()),
    },
  ];
  return named.map((group) => (
    <Fragment key={group.provider}>
      <PanelGroupHead title={group.name} level="h3" />
      <DataTable<AiModelRate>
        bleed
        stickyFirst
        label={t("settings.rates.modelGroupLabel", { provider: group.name })}
        rows={group.prices}
        rowKey={(row) => `${row.model_id}/${row.effective_date}`}
        rowTestId={(row) => `price-${group.provider}-${row.model_id}`}
        columns={columns}
      />
    </Fragment>
  ));
}
