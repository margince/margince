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
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { countryOptions } from "../../format/assignedcountries";
import { todayIn } from "../../format/calendarday";
import { formatNumber } from "../../format/format";
import { jurisdictionName } from "../../format/jurisdiction";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { WithheldRateCard } from "./rates";

type TaxRate = components["schemas"]["TaxRate"];
type Category = TaxRate["category"];

const TAX_RATES_KEY = ["tax-rates"] as const;

// The tax rates offers are charged, by country and category. A rate is added
// and never changed: a new rate from a later day replaces it from that day.
export function TaxRatesCard() {
  const t = useT();
  const { locale } = useLocale();
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
              render: (row) => `${row.rate} %`,
            },
            {
              key: "from",
              header: t("taxRates.colFrom"),
              render: (row) => row.valid_from,
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
  // The sheet counts its days in the installation's zone, so the form opens
  // on the day the server reads as today.
  const zone = useRecordZone();
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
            rate: input.rate.trim().replace(",", "."),
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
  const ready = draft.country !== "" && draft.rate.trim() !== "";
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || save.isPending || refuseUnreadableField(event.currentTarget))
      return;
    save.mutate(draft);
  };
  const set = (patch: Partial<Draft>) =>
    setDraft((current) => ({ ...current, ...patch }));
  return (
    <Modal open onClose={onClose} labelledBy={labelId} intent="form">
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {t("taxRates.modalTitle")}
      </Heading>
      <form id={formId} className="form-stack" onSubmit={submit}>
        <Field label={t("taxRates.colCountry")}>
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
        <Field label={t("taxRates.colRate")} hint={t("taxRates.rateHint")}>
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
          disabled={!ready || save.isPending}
        >
          {t("taxRates.save")}
        </Button>
      </div>
    </Modal>
  );
}
