// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { calendarDay } from "../format/calendarday";
import { PER_MTOK_PRICE } from "../format/priceinput";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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

type SheetRow = components["schemas"]["AiModelRate"];

// A scheduled price is edited on its own date; an in-force one, from today.
function startDate(row: SheetRow | undefined): string {
  return row && row.effective_date > today() ? row.effective_date : today();
}

/**
 * ModelPriceDialog is the sheet's free-form "add a rate": any provider, any
 * model, in a dialog that closes on save. A vendor's own prices are managed in
 * its sheet, through the same `PriceForm`.
 */
export function ModelPriceDialog({
  onClose,
}: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const labelId = useId();
  return (
    <Modal open onClose={onClose} labelledBy={labelId}>
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {t("settings.rates.modelModalTitle")}
      </Heading>
      <PriceForm onDone={onClose} onCancel={onClose} />
    </Modal>
  );
}

/**
 * PriceForm sets one model's price by hand: the four per-million-token
 * buckets, effective today or later.
 *
 * Three ways in, one form. With `initial` it edits that row — the model and what
 * it is for are the row's identity, so they are named above the fields rather
 * than offered again. With a `provider` it adds a price for that vendor, whose
 * model box offers what the vendor serves and what the sheet already prices.
 * With neither it is the free-form "add a rate".
 */
export function PriceForm({
  provider: fixedProvider,
  initial,
  onDone,
  onCancel,
}: Readonly<{
  provider?: string;
  initial?: SheetRow;
  onDone: () => void;
  onCancel: () => void;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const [typedProvider, setProvider] = useState("");
  const provider = initial?.provider ?? fixedProvider ?? typedProvider;
  const [modelId, setModelId] = useState(initial?.model_id ?? "");
  const [input, setInput] = useState(initial?.input_per_mtok ?? "");
  const [output, setOutput] = useState(initial?.output_per_mtok ?? "");
  const [cacheRead, setCacheRead] = useState(
    initial?.cache_read_per_mtok ?? "0",
  );
  const [cacheWrite, setCacheWrite] = useState(
    initial?.cache_write_per_mtok ?? "0",
  );
  const [effectiveDate, setEffectiveDate] = useState(startDate(initial));
  // What the model is FOR. Always sent: a price filed without one is filed as
  // chat, and a new embedder then never reaches the embeddings picker.
  const [lane, setLane] = useState<ModelLane>(initial?.lane ?? "chat");
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST("/ai-model-rates", {
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
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ai-model-rates"] });
      onDone();
    },
    onError: (err: Error) => setError(problemMessageOf(err, t)),
  });

  const decimals = [input, output, cacheRead || "0", cacheWrite || "0"];
  const malformed = decimals.some(
    (v) => v.trim() !== "" && !PER_MTOK_PRICE.test(v.trim()),
  );

  // One box of this form. `Field` owns the id and hands it to the input.
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
          onChange={(e) => set(e.target.value)}
        />
      )}
    </Field>
  );

  return (
    <div className="form-stack">
      {initial ? (
        <p className="t-sub">
          {t("aiRates.manual.editing", {
            model: initial.model_id,
            lane: t(LANE_LABEL[initial.lane]),
          })}
        </p>
      ) : null}
      {initial ? null : (
        <NewPriceIdentity
          fixedProvider={fixedProvider}
          typedProvider={typedProvider}
          onProvider={setProvider}
          modelId={modelId}
          onModel={setModelId}
          lane={lane}
        />
      )}
      <div className="form-row">
        {initial ? null : <LaneField lane={lane} onChange={setLane} />}
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
      <WriteRefused titleKey="settings.rates.notSaved" message={error} />
      <div className="actions rates-manual-actions">
        <Button variant="ghost" onClick={onCancel}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          onClick={() => {
            setError(null);
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
    </div>
  );
}

// Who and what a NEW price is for. A vendor's own sheet fixes the provider and
// offers what it serves; the free-form add asks for both as text.
function NewPriceIdentity({
  fixedProvider,
  typedProvider,
  onProvider,
  modelId,
  onModel,
  lane,
}: Readonly<{
  fixedProvider: string | undefined;
  typedProvider: string;
  onProvider: (next: string) => void;
  modelId: string;
  onModel: (next: string) => void;
  lane: ModelLane;
}>) {
  const t = useT();
  if (fixedProvider !== undefined) {
    return (
      <VendorModelField
        provider={fixedProvider}
        lane={lane}
        value={modelId}
        onChange={onModel}
      />
    );
  }
  const text = (label: string, value: string, set: (v: string) => void) => (
    <Field label={label}>
      {(control) => (
        <TextInput
          {...control}
          value={value}
          inputMode="text"
          onChange={(e) => set(e.target.value)}
        />
      )}
    </Field>
  );
  return (
    <>
      {text(t("settings.rates.colProvider"), typedProvider, onProvider)}
      {text(t("settings.rates.colModel"), modelId, onModel)}
    </>
  );
}

const LANE_LABEL = {
  chat: "aiRates.manual.laneChat",
  embeddings: "aiRates.manual.laneEmbeddings",
  decisions: "aiRates.manual.laneDecisions",
} as const satisfies Record<ModelLane, MessageKey>;

const LANES: readonly ModelLane[] = ["chat", "embeddings", "decisions"];

// What the model is FOR. A closed choice: the list is the contract's own, so a
// pick is looked up in it rather than trusted as a string.
function LaneField({
  lane,
  onChange,
}: Readonly<{ lane: ModelLane; onChange: (next: ModelLane) => void }>) {
  const t = useT();
  return (
    <Field label={t("aiRates.manual.lane")}>
      {(control) => (
        <Select
          {...control}
          value={lane}
          options={LANES.map((value) => ({
            value,
            label: t(LANE_LABEL[value]),
          }))}
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
