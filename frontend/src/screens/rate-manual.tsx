// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { calendarDay } from "../format/calendarday";
import { formatUsdPerMTok } from "../format/format";
import { PER_MTOK_PRICE } from "../format/priceinput";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { ModelLane } from "./ai-models";
import {
  offeredModels,
  useAiModelCatalogue,
  useAvailableModels,
} from "./ai-models";
import { problemMessageOf, throwProblem, WriteRefused } from "./common";

// The reader's own today, for the same reason the sheets read effective dates
// against their calendar rather than UTC's. Shared with the currency sheet.
export function today(): string {
  return calendarDay(new Date(), viewerZone());
}

/**
 * ModelPriceDialog sets one model's price by hand: the four per-million-token
 * buckets, effective today or later.
 *
 * With a `provider` the dialog is about that vendor alone. Its model box offers
 * what the vendor serves (and what the sheet already prices), the models the
 * sheet already prices for it are listed to edit, and a save leaves the dialog
 * open showing the price it wrote. Without one it is the sheet's free-form
 * "add a rate" form, which closes on save.
 */
export function ModelPriceDialog({
  provider: fixedProvider,
  onClose,
}: Readonly<{ provider?: string; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const qc = useQueryClient();
  const labelId = useId();
  const [typedProvider, setProvider] = useState("");
  const provider = fixedProvider ?? typedProvider;
  const [modelId, setModelId] = useState("");
  const [input, setInput] = useState("");
  const [output, setOutput] = useState("");
  const [cacheRead, setCacheRead] = useState("0");
  const [cacheWrite, setCacheWrite] = useState("0");
  const [effectiveDate, setEffectiveDate] = useState(today());
  // What the model is FOR. Always sent: a price filed without one is filed as
  // chat, and a new embedder then never reaches the embeddings picker.
  const [lane, setLane] = useState<ModelLane>("chat");
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const sheet = useAiModelCatalogue();

  const save = useMutation({
    mutationFn: async () => {
      const { data, error: err } = await api.POST("/ai-model-rates", {
        body: {
          provider: provider.trim(),
          model_id: modelId.trim(),
          input_per_mtok: input.trim(),
          output_per_mtok: output.trim(),
          cache_read_per_mtok: cacheRead.trim() || "0",
          cache_write_per_mtok: cacheWrite.trim() || "0",
          lane,
          effective_date: effectiveDate,
        },
      });
      if (err) {
        throwProblem(err);
      }
      return data;
    },
    onSuccess: (row) => {
      qc.invalidateQueries({ queryKey: ["ai-model-rates"] });
      if (fixedProvider === undefined || row === undefined) {
        onClose();
        return;
      }
      setSaved(
        t("aiRates.manual.saved", {
          model: row.model_id,
          price: t("aiAdmin.rates", {
            input: formatUsdPerMTok(row.input_per_mtok, locale),
            output: formatUsdPerMTok(row.output_per_mtok, locale),
          }),
        }),
      );
    },
    onError: (err: Error) => setError(problemMessageOf(err, t)),
  });

  const existing = (sheet.data ?? []).filter(
    (r) => fixedProvider !== undefined && r.provider === fixedProvider,
  );
  const decimals = [input, output, cacheRead || "0", cacheWrite || "0"];
  const malformed = decimals.some(
    (v) => v.trim() !== "" && !PER_MTOK_PRICE.test(v.trim()),
  );

  // One box of this dialog. `Field` owns the id and hands it to the input.
  const field = (
    label: string,
    value: string,
    set: (v: string) => void,
    opts: Readonly<{
      inputMode?: "text" | "decimal";
      placeholder?: string;
    }> = {},
  ) => (
    <Field label={label}>
      {(control) => (
        <TextInput
          {...control}
          value={value}
          inputMode={opts.inputMode ?? "decimal"}
          placeholder={opts.placeholder ?? ""}
          onChange={(e) => {
            setSaved(null);
            set(e.target.value);
          }}
        />
      )}
    </Field>
  );

  const edit = (row: (typeof existing)[number]) => {
    setSaved(null);
    setError(null);
    setModelId(row.model_id);
    setLane(row.lane);
    // A scheduled price is edited on its own date; an in-force one, from today.
    setEffectiveDate(
      row.effective_date > today() ? row.effective_date : today(),
    );
    setInput(row.input_per_mtok);
    setOutput(row.output_per_mtok);
    setCacheRead(row.cache_read_per_mtok);
    setCacheWrite(row.cache_write_per_mtok);
  };

  const pricedModels = fixedProvider !== undefined && existing.length > 0;
  const submit = (
    <div className="actions rates-manual-actions">
      <Button variant="ghost" onClick={onClose}>
        {fixedProvider === undefined
          ? t("create.cancel")
          : t("aiRates.manual.done")}
      </Button>
      <Button
        variant="primary"
        onClick={() => {
          setError(null);
          setSaved(null);
          save.mutate();
        }}
        disabled={
          save.isPending ||
          malformed ||
          provider.trim() === "" ||
          modelId.trim() === "" ||
          input.trim() === "" ||
          output.trim() === ""
        }
      >
        {t("settings.rates.setRate")}
      </Button>
    </div>
  );

  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={labelId}
      size={fixedProvider === undefined ? "default" : "wide"}
    >
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {fixedProvider === undefined
          ? t("settings.rates.modelModalTitle")
          : t("aiRates.manual.title", { provider: fixedProvider })}
      </Heading>
      <div className="form-stack">
        {fixedProvider !== undefined ? (
          <Heading size="small" className="t-h3">
            {t("aiRates.manual.formTitle")}
          </Heading>
        ) : null}
        {fixedProvider === undefined
          ? field(t("settings.rates.colProvider"), typedProvider, setProvider, {
              inputMode: "text",
            })
          : null}
        {fixedProvider === undefined ? (
          field(t("settings.rates.colModel"), modelId, setModelId, {
            inputMode: "text",
          })
        ) : (
          <VendorModelField
            provider={fixedProvider}
            lane={lane}
            value={modelId}
            onChange={(next) => {
              setSaved(null);
              setModelId(next);
            }}
          />
        )}
        <div className="form-row">
          <LaneField
            lane={lane}
            onChange={(next) => {
              setSaved(null);
              setLane(next);
            }}
          />
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
        </div>
        <div className="form-row">
          {field(t("settings.rates.colInput"), input, setInput, {
            placeholder: "5.00",
          })}
          {field(t("settings.rates.colOutput"), output, setOutput, {
            placeholder: "25.00",
          })}
        </div>
        <div className="form-row">
          {field(t("settings.rates.colCacheRead"), cacheRead, setCacheRead)}
          {field(t("settings.rates.colCacheWrite"), cacheWrite, setCacheWrite)}
        </div>
        {malformed ? (
          <ErrorLine>{t("aiRates.manual.malformed")}</ErrorLine>
        ) : null}
        {saved ? (
          <p className="t-caption" role="status">
            {saved}
          </p>
        ) : null}
        <WriteRefused titleKey="settings.rates.notSaved" message={error} />
        {pricedModels ? (
          <PricedModels
            rows={existing}
            current={modelId.trim()}
            onEdit={edit}
          />
        ) : null}
        {submit}
      </div>
    </Modal>
  );
}

type SheetRow = components["schemas"]["AiModelRate"];

// The provider's priced models as a table, the row a form is editing marked.
function PricedModels({
  rows,
  current,
  onEdit,
}: Readonly<{
  rows: readonly SheetRow[];
  current: string;
  onEdit: (row: SheetRow) => void;
}>) {
  const t = useT();
  const existing = [...rows];
  const modelId = current;
  const edit = onEdit;
  return (
    <>
      <Heading size="small" className="t-h3">
        {t("aiRates.manual.priced")}
      </Heading>
      <div className="rates-manual-table">
        <DataTable
          label={t("aiRates.manual.priced")}
          rows={existing}
          rowKey={(row) => row.model_id}
          rowClassName={(row) =>
            row.model_id === modelId.trim() ? "row-current" : undefined
          }
          columns={[
            {
              key: "model",
              header: t("settings.rates.colModel"),
              grow: true,
              render: (row) => (
                <span className="rates-manual-model">
                  {row.model_id}
                  {row.effective_date > today() ? (
                    <Badge>
                      {t("aiRates.manual.from", {
                        date: row.effective_date,
                      })}
                    </Badge>
                  ) : null}
                </span>
              ),
            },
            {
              key: "in",
              header: t("settings.rates.colInput"),
              align: "end",
              render: (row) => row.input_per_mtok,
            },
            {
              key: "out",
              header: t("settings.rates.colOutput"),
              align: "end",
              render: (row) => row.output_per_mtok,
            },
            {
              key: "cr",
              header: t("settings.rates.colCacheRead"),
              align: "end",
              render: (row) => row.cache_read_per_mtok,
            },
            {
              key: "cw",
              header: t("settings.rates.colCacheWrite"),
              align: "end",
              render: (row) => row.cache_write_per_mtok,
            },
            {
              key: "edit",
              header: t("aiRates.manual.edit"),
              align: "end",
              render: (row) => (
                <Button variant="ghost" onClick={() => edit(row)}>
                  {t("aiRates.manual.edit")}
                  <span className="sr-only"> {row.model_id}</span>
                </Button>
              ),
            },
          ]}
        />
      </div>
    </>
  );
}

const LANES: readonly ModelLane[] = ["chat", "embeddings", "decisions"];

// What the model is FOR. A closed choice: the list is the contract's own, so a
// pick is looked up in it rather than trusted as a string.
function LaneField({
  lane,
  onChange,
}: Readonly<{ lane: ModelLane; onChange: (next: ModelLane) => void }>) {
  const t = useT();
  const labels: Readonly<Record<ModelLane, string>> = {
    chat: t("aiRates.manual.laneChat"),
    embeddings: t("aiRates.manual.laneEmbeddings"),
    decisions: t("aiRates.manual.laneDecisions"),
  };
  return (
    <Field label={t("aiRates.manual.lane")}>
      {(control) => (
        <Select
          {...control}
          value={lane}
          options={LANES.map((value) => ({ value, label: labels[value] }))}
          onChange={(next) => {
            const picked = LANES.find((candidate) => candidate === next);
            if (picked) onChange(picked);
          }}
        />
      )}
    </Field>
  );
}

// The model box for one vendor: what it serves, and what the sheet prices
// already, offered as suggestions to a box that still takes any id typed.
function VendorModelField({
  provider,
  lane,
  value,
  onChange,
}: Readonly<{
  provider: string;
  lane: ModelLane;
  value: string;
  onChange: (next: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const sheet = useAiModelCatalogue();
  const available = useAvailableModels(provider, "", true);
  return (
    <Field label={t("settings.rates.colModel")}>
      {(control) => (
        <ComboBox
          {...control}
          value={value}
          suggestions={offeredModels(
            available.data,
            sheet.data,
            provider,
            lane,
            locale,
          )}
          onChange={onChange}
        />
      )}
    </Field>
  );
}
