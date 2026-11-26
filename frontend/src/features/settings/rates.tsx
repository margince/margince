import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Disclosure,
  EmptyState,
  Field,
  Modal,
  OptionCount,
  TextInput,
} from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { isAssignedCurrency } from "../../format/assignedcurrencies";
import { todayIn } from "../../format/calendarday";
import { forReader } from "../../format/collate";
import { typedDecimalText, typedNumberFault } from "../../format/decimalinput";
import { formatDate, formatUsdPerMTok } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { providerName } from "../../lib/ai-provider-names";
import { useCan, useCanUpsert } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import {
  fieldCheckSentence,
  refusalSentences,
  unwrap,
} from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useViewerZone } from "../../lib/viewerzone";
import { isPricedProvider } from "./rate-manual";
import { RefreshFromSources } from "./rate-refresh";
import { useRemoveFxRate } from "./rate-remove";
import "./rates.css";
import { DialogRefusal } from "../../design-system/panelnotices";

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

// Withheld, not absent: a vanished sheet reads as "no rates here", not "not
// yours to see". Callers skip the query, since fetching the 403 shows a Retry.
export function WithheldRateCard({
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
  const { locale } = useLocale();
  const readerZone = useViewerZone();
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
  const remove = useRemoveFxRate();
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

  const rates = query.isError ? undefined : query.data;

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
        {!rates?.length && (
          <QueryGate query={query} pendingLabel={t("settings.rates.fxTitle")}>
            {() => <EmptyState>{t("settings.rates.fxEmpty")}</EmptyState>}
          </QueryGate>
        )}
        {open ? <FxRateModal onClose={() => setOpen(false)} /> : null}
        {remove.dialog}
      </PanelBody>
      {/* The sheet is the panel's subject, so it runs edge to edge. */}
      {rates?.length ? (
        <DataTable<FxRate>
          label={t("settings.rates.fxTableLabel")}
          bleed
          rows={rates}
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
                base: rates[0]?.to_currency ?? "",
              }),
              render: (row) => trimDecimal(row.rate),
            },
            {
              key: "effective",
              header: t("settings.rates.colEffective"),
              render: (row) =>
                formatDate(row.effective_date, locale, readerZone),
            },
            ...remove.columns,
          ]}
        />
      ) : null}
    </Panel>
  );
}

type FxRateBody = components["schemas"]["SetFxRateRequest"];

// The server's limits on a rate: positive, at most 10 + 10 digits.
const RATE_DIGITS = 10;

function rateFault(text: string): boolean {
  const [whole = "", fraction = ""] = text.split(".");
  return (
    Number(text) <= 0 ||
    whole.replace(/^-?0*/, "").length > RATE_DIGITS ||
    fraction.length > RATE_DIGITS
  );
}

function FxRateModal({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const qc = useQueryClient();
  const labelId = useId();
  const formId = useId();
  // The FX sheet counts its days in the installation's zone, so the form
  // opens on the day the server will read as today.
  const zone = useCompanyZone();
  const base = useInstallationSettings().data?.base_currency;
  const [from, setFrom] = useState("");
  const [rate, setRate] = useState("");
  const [effectiveDate, setEffectiveDate] = useState(() => todayIn(zone));
  const [faults, faultsRoot] = useFieldFaults();
  const labels = {
    from_currency: t("settings.rates.colFrom"),
    rate: t("settings.rates.rateToBase"),
    effective_date: t("settings.rates.colEffective"),
  };

  const save = useMutation({
    mutationFn: async (body: FxRateBody) => {
      unwrap(await api.POST("/fx-rates", { body }));
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["fx-rates"] });
      onClose();
    },
    onError: (err: Error) =>
      faults.show(refusalSentences(err, t, labels).byField),
  });
  const refused = refusalSentences(save.error, t, labels);
  const rateText = typedDecimalText(rate, locale);
  const typedFault = typedNumberFault(rate, locale, t);

  // The checks the server makes that need no database, in its own words.
  function checked(code: string): ReadonlyMap<string, string> {
    const required = (field: keyof typeof labels) =>
      fieldCheckSentence("required", locale, labels[field]);
    const found = new Map<string, string>();
    if (code === "") found.set("from_currency", required("from_currency"));
    else if (!isAssignedCurrency(code)) {
      found.set("from_currency", t("settings.rates.fromNotCurrency"));
    } else if (code === base) {
      found.set("from_currency", t("settings.rates.fromIsBase"));
    }
    if (rate.trim() === "") found.set("rate", required("rate"));
    else if (typedFault !== undefined) found.set("rate", typedFault);
    else if (rateText === undefined || rateFault(rateText)) {
      found.set("rate", t("settings.rates.rateNotPositive"));
    }
    if (effectiveDate === "") {
      found.set("effective_date", required("effective_date"));
    }
    return found;
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (save.isPending || refuseUnreadableField(event.currentTarget)) return;
    const code = from.trim().toUpperCase();
    if (faults.show(checked(code)) || rateText === undefined) {
      save.reset();
      return;
    }
    save.mutate({
      from_currency: code,
      rate: rateText,
      effective_date: effectiveDate,
    });
  }

  return (
    <Modal open onClose={onClose} labelledBy={labelId} intent="form">
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {t("settings.rates.fxModalTitle")}
      </Heading>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={submit}
      >
        <Field
          label={labels.from_currency}
          required
          error={faults.of("from_currency")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={from}
              maxLength={3}
              onChange={(e) => {
                setFrom(e.target.value);
                faults.edited("from_currency");
              }}
            />
          )}
        </Field>
        <Field
          label={labels.rate}
          required
          error={faults.of("rate") ?? typedFault}
        >
          {(control) => (
            <TextInput
              {...control}
              value={rate}
              inputMode="decimal"
              placeholder="0.92"
              onChange={(e) => {
                setRate(e.target.value);
                faults.edited("rate");
              }}
            />
          )}
        </Field>
        <Field
          label={labels.effective_date}
          required
          error={faults.of("effective_date")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(effectiveDate)}
              onChange={(e) => {
                setEffectiveDate(e.target.value);
                faults.edited("effective_date");
              }}
            />
          )}
        </Field>
        <DialogRefusal
          title={t("settings.rates.notSaved")}
          message={refused.rest}
        />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          pending={save.isPending}
        >
          {t("settings.rates.setRate")}
        </Button>
      </div>
    </Modal>
  );
}

// ---- AI model costs ----

/**
 * The prices in force, one fold per provider, inside the model prices panel
 * on AI models. The caller holds the read grant and the verbs that write.
 */
export function ModelPriceSheet({
  shown,
}: Readonly<{ shown: (provider: string) => boolean }>) {
  const t = useT();
  const query = useQuery({
    queryKey: ["ai-model-rates"],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/ai-model-rates", {
          params: { query: {} },
        }),
      );
      return data.data;
    },
  });
  const priced = query.data?.filter(
    (row) => isPricedProvider(row.provider) && shown(row.provider),
  );
  return (
    <PanelBody>
      {query.data === undefined && (
        <QueryStates
          query={query}
          pendingLabel={t("settings.rates.modelTableLabel")}
        >
          {null}
        </QueryStates>
      )}
      {priced?.length === 0 && (
        <EmptyState>{t("settings.rates.modelEmpty")}</EmptyState>
      )}
      {priced && priced.length > 0 && <ModelPriceGroups rows={priced} />}
    </PanelBody>
  );
}

function ModelPriceGroups({
  rows,
}: Readonly<{ rows: readonly AiModelRate[] }>) {
  const readerZone = useViewerZone();
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
      render: (row) => formatDate(row.effective_date, locale, readerZone),
    },
  ];
  // Multi-open: a reader compares one provider's prices with another's.
  return named.map((group) => (
    <Disclosure
      key={group.provider}
      summary={
        <span className="rates-price-group">
          {group.name}
          <OptionCount count={group.prices.length} />
        </span>
      }
    >
      <DataTable<AiModelRate>
        stickyFirst
        label={t("settings.rates.modelGroupLabel", { provider: group.name })}
        rows={group.prices}
        rowKey={(row) => `${row.model_id}/${row.effective_date}`}
        rowTestId={(row) =>
          `price-${group.provider}-${row.model_id}-${row.effective_date}`
        }
        columns={columns}
      />
    </Disclosure>
  ));
}
