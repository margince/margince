// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, WriteRefused } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { countryOptions } from "../../format/assignedcountries";
import { todayIn } from "../../format/calendarday";
import { typedDecimalText, typedNumberFault } from "../../format/decimalinput";
import { formatDate, formatNumber, formatPercent } from "../../format/format";
import { jurisdictionName } from "../../format/jurisdiction";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import { blankRequired, problemMessageOf, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { WithheldRateCard } from "./rates";

type TaxRate = components["schemas"]["TaxRate"];
type Category = TaxRate["category"];

const TAX_RATES_KEY = ["tax-rates"] as const;

// The server sends a rate in percent with two decimals.
const TAX_RATE_DIGITS = 2;

// The tax rates offers are charged, by country and category. A rate is added
// and never changed: a new rate from a later day replaces it from that day.
export function TaxRatesCard() {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const canRead = useCan("tax_rate", "read");
  const canRecord = useCanWrite("tax_rate", "create");
  const [open, setOpen] = useState(false);
  const query = useQuery({
    queryKey: TAX_RATES_KEY,
    enabled: canRead,
    queryFn: async () => unwrap(await api.GET("/tax-rates")).data,
  });
  if (!canRead) {
    return (
      <WithheldRateCard
        title={t("taxRates.title")}
        reason={t("taxRates.withheld")}
      />
    );
  }
  const rates = query.isError ? undefined : query.data;
  return (
    <Panel
      title={t("taxRates.title")}
      actions={
        canRecord ? (
          <Button variant="primary" onClick={() => setOpen(true)}>
            {t("taxRates.add")}
          </Button>
        ) : null
      }
    >
      <PanelBody className="form-stack">
        <PanelIntro>{t("taxRates.intro")}</PanelIntro>
        {!rates?.length && (
          <QueryGate query={query} pendingLabel={t("taxRates.title")}>
            {() => <EmptyState>{t("taxRates.empty")}</EmptyState>}
          </QueryGate>
        )}
        {open && <TaxRateModal onClose={() => setOpen(false)} />}
      </PanelBody>
      {rates?.length ? (
        <DataTable<TaxRate>
          label={t("taxRates.title")}
          bleed
          rows={rates}
          rowKey={(row) => row.id}
          columns={[
            {
              key: "country",
              header: t("taxRates.colCountry"),
              render: (row) =>
                jurisdictionName(row.country.toUpperCase(), locale),
            },
            {
              key: "category",
              header: t("taxRates.colCategory"),
              render: (row) => t(`taxRates.category.${row.category}`),
            },
            {
              key: "rate",
              header: t("taxRates.colRate"),
              // A statutory rate keeps its decimals: 7.7 % is not 8 %.
              render: (row) =>
                formatPercent(Number(row.rate) / 100, locale, TAX_RATE_DIGITS),
            },
            {
              key: "from",
              header: t("taxRates.colFrom"),
              render: (row) => formatDate(row.valid_from, locale, readerZone),
            },
          ]}
        />
      ) : null}
    </Panel>
  );
}

type Draft = Readonly<{
  country: string;
  category: Category;
  rate: string;
  validFrom: string;
}>;

function TaxRateModal({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const qc = useQueryClient();
  const toast = useToast();
  const plural = usePlural();
  const labelId = useId();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const labels = {
    country: t("taxRates.colCountry"),
    rate: t("taxRates.colRate"),
  };
  // The sheet counts its days in the installation's zone, so the form opens
  // on the day the server reads as today.
  const zone = useCompanyZone();
  const [draft, setDraft] = useState<Draft>(() => ({
    country: "",
    category: "standard",
    rate: "",
    validFrom: todayIn(zone),
  }));
  const save = useMutation({
    mutationFn: async (input: Draft) =>
      unwrap(
        await api.POST("/tax-rates", {
          body: {
            country: input.country,
            category: input.category,
            rate: typedDecimalText(input.rate, locale) ?? input.rate,
            valid_from: input.validFrom,
          },
        }),
      ),
    onSuccess: (recorded) => {
      void qc.invalidateQueries({ queryKey: TAX_RATES_KEY });
      void qc.invalidateQueries({ queryKey: ["products"] });
      toast.show(
        plural("taxRates.recorded", recorded.classified_products, {
          count: formatNumber(recorded.classified_products, locale),
        }),
      );
      onClose();
    },
  });
  const rateFault = typedNumberFault(draft.rate, locale, t);
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const found = blankRequired(locale, {
      country: [labels.country, draft.country],
      rate: [labels.rate, draft.rate],
    });
    if (rateFault !== undefined && !found.has("rate")) {
      found.set("rate", rateFault);
    }
    if (faults.show(found) || save.isPending) return;
    if (refuseUnreadableField(event.currentTarget)) return;
    save.mutate(draft);
  };
  const set = (patch: Partial<Draft>) => {
    setDraft((current) => ({ ...current, ...patch }));
    for (const key of Object.keys(patch)) faults.edited(key);
  };
  return (
    <Modal open onClose={onClose} labelledBy={labelId} intent="form">
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {t("taxRates.modalTitle")}
      </Heading>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={submit}
      >
        <Field label={labels.country} required error={faults.of("country")}>
          {(control) => (
            <Select
              {...control}
              value={draft.country}
              placeholder={t("taxRates.countryPlaceholder")}
              options={countryOptions(locale).map((option) => ({
                value: option.value.toLowerCase(),
                label: option.label,
              }))}
              onChange={(country) => set({ country })}
            />
          )}
        </Field>
        <Field label={t("taxRates.colCategory")}>
          {(control) => (
            <Select
              {...control}
              value={draft.category}
              options={(["standard", "reduced"] as const).map((category) => ({
                value: category,
                label: t(`taxRates.category.${category}`),
              }))}
              onChange={(category) =>
                set({
                  category: category === "reduced" ? "reduced" : "standard",
                })
              }
            />
          )}
        </Field>
        <Field
          label={labels.rate}
          hint={t("taxRates.rateHint")}
          required
          error={faults.of("rate") ?? rateFault}
        >
          {(control) => (
            <TextInput
              {...control}
              value={draft.rate}
              inputMode="decimal"
              placeholder="19"
              onChange={(event) => set({ rate: event.target.value })}
            />
          )}
        </Field>
        <Field label={t("taxRates.colFrom")} hint={t("taxRates.fromHint")}>
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(draft.validFrom)}
              onChange={(event) => set({ validFrom: event.target.value })}
            />
          )}
        </Field>
        <WriteRefused
          titleKey="taxRates.notSaved"
          message={save.isError ? problemMessageOf(save.error, t) : null}
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
          {t("taxRates.save")}
        </Button>
      </div>
    </Modal>
  );
}
