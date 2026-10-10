// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { today } from "../format/calendarday";
import { PER_MTOK_PRICE } from "../format/priceinput";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { ModelLane } from "./ai-models";
import {
  offeredModels,
  useAiModelCatalogue,
  useAvailableModels,
} from "./ai-models";
import { DECISION_PROVIDERS, PROVIDERS } from "./ai-routing-fields";
import { problemMessageOf, unwrap, WriteRefused } from "./common";
import "./rates.css";

type SheetRow = components["schemas"]["AiModelRate"];
type PriceWrite = components["schemas"]["SetAiModelRateRequest"];

/** A model routing binds, with the lane it is bound in. */
export type BoundModel = Readonly<{ model: string; lane: ModelLane }>;

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
    <Modal open onClose={onClose} labelledBy={labelId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={labelId} className="t-h2 modal-title">
          {t("settings.rates.modelModalTitle")}
        </Heading>
      </DrawerHead>
      <PriceForm
        onDone={onClose}
        onCancel={onClose}
        frame={(fields, actions) => (
          <>
            <DrawerBody>{fields}</DrawerBody>
            <DrawerFoot className="actions">{actions}</DrawerFoot>
          </>
        )}
      />
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
  draft,
  boundModels,
  onDone,
  onCancel,
  frame = inlineFrame,
}: Readonly<{
  provider?: string;
  // A model to start the form on, for one that is in use and has no price.
  draft?: BoundModel;
  // The models routing binds on this vendor, offered first in the model box;
  // picking one files the price in the lane it is bound in.
  boundModels?: readonly BoundModel[];
  initial?: SheetRow;
  onDone: () => void;
  onCancel: () => void;
  // Where the fields and the verbs go: a drawer pins the verbs in its foot.
  frame?: (fields: ReactNode, actions: ReactNode) => ReactNode;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const [typedProvider, setProvider] = useState("");
  const provider = initial?.provider ?? fixedProvider ?? typedProvider;
  const [modelId, setModelId] = useState(
    initial?.model_id ?? draft?.model ?? "",
  );
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
  const [lane, setLane] = useState<ModelLane>(
    initial?.lane ?? draft?.lane ?? "chat",
  );
  const [error, setError] = useState<string | null>(null);
  // A refusal describes the values that were sent; once any of them changes it
  // no longer describes the form.

  // biome-ignore lint/correctness/useExhaustiveDependencies: the fields are the trigger, not read
  useEffect(() => {
    setError(null);
  }, [
    provider,
    modelId,
    input,
    output,
    cacheRead,
    cacheWrite,
    effectiveDate,
    lane,
  ]);

  const save = useMutation({
    mutationFn: async (body: PriceWrite) => {
      unwrap(await api.POST("/ai-model-rates", { body }));
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
  // A bound model is priced in the lane it is bound in: the free-form pick
  // starts on chat, and an embedder filed there never reaches its picker.
  const pickModel = (next: string) => {
    setModelId(next);
    const bound = boundModels?.find((b) => b.model === next);
    if (bound) setLane(bound.lane);
  };

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

  const fields = (
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
          onModel={pickModel}
          lane={lane}
          boundModels={boundModels}
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
    </div>
  );
  const actions = (
    <>
      <Button variant="ghost" onClick={onCancel}>
        {t("create.cancel")}
      </Button>
      <Button
        variant="primary"
        onClick={() => {
          setError(null);
          save.mutate({
            provider: provider.trim(),
            model_id: modelId.trim(),
            input_per_mtok: input.trim(),
            output_per_mtok: output.trim(),
            cache_read_per_mtok: cacheRead.trim() || "0",
            cache_write_per_mtok: cacheWrite.trim() || "0",
            lane,
            effective_date: effectiveDate,
          });
        }}
        // A cleared date box reports "", and the server refuses an empty
        // effective date; the refusal is spelled here, before the write.
        disabled={
          save.isPending ||
          malformed ||
          provider.trim() === "" ||
          modelId.trim() === "" ||
          input.trim() === "" ||
          output.trim() === "" ||
          effectiveDate === ""
        }
      >
        {t("settings.rates.setRate")}
      </Button>
    </>
  );
  return frame(fields, actions);
}

// Who and what a NEW price is for. A vendor's own sheet fixes the provider; the
// free-form add picks one from the vendors the product knows. Either way the
// model box is the same one, offering what that vendor serves.
function NewPriceIdentity({
  fixedProvider,
  typedProvider,
  onProvider,
  modelId,
  onModel,
  lane,
  boundModels,
}: Readonly<{
  fixedProvider: string | undefined;
  typedProvider: string;
  onProvider: (next: string) => void;
  modelId: string;
  onModel: (next: string) => void;
  lane: ModelLane;
  boundModels?: readonly BoundModel[];
}>) {
  const t = useT();
  const provider = fixedProvider ?? typedProvider;
  return (
    <>
      {fixedProvider === undefined && (
        <Field label={t("settings.rates.colProvider")}>
          {(control) => (
            <Select
              {...control}
              value={typedProvider}
              options={PRICED_PROVIDERS.map((value) => ({
                value,
                label: value,
              }))}
              onChange={(next) => {
                onProvider(next);
                onModel("");
              }}
            />
          )}
        </Field>
      )}
      {provider !== "" && (
        <VendorModelField
          provider={provider}
          lane={lane}
          value={modelId}
          onChange={onModel}
          bound={boundModels}
        />
      )}
    </>
  );
}

// Not a fragment: in the provider sheet's gap column the footer would gain that
// gap on top of its own margin.
function inlineFrame(fields: ReactNode, actions: ReactNode) {
  return (
    <div>
      {fields}
      <div className="actions rates-manual-actions">{actions}</div>
    </div>
  );
}

export function isPricedProvider(provider: string): boolean {
  return provider !== "fake";
}

const PRICED_PROVIDERS: readonly string[] = [
  ...PROVIDERS.filter(isPricedProvider),
  ...DECISION_PROVIDERS,
];

/** What each lane is called where a price is filed or removed. */
export const LANE_LABEL = {
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
  bound,
}: Readonly<{
  provider: string;
  lane: ModelLane;
  value: string;
  onChange: (next: string) => void;
  bound?: readonly BoundModel[];
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
          suggestions={withBound(
            offeredModels(available.data, sheet.data, provider, lane, locale),
            bound,
            t("aiProviders.inUse"),
          )}
          onChange={onChange}
        />
      )}
    </Field>
  );
}

// The models routing already runs on this vendor go first: they are the ones a
// missing price is most likely to be missing for. Once each, because a model
// bound on two lanes is still one id to offer.
function withBound(
  offered: ReturnType<typeof offeredModels>,
  bound: readonly BoundModel[] | undefined,
  hint: string,
): ReturnType<typeof offeredModels> {
  const inUse = [...new Set((bound ?? []).map((b) => b.model))];
  return [
    ...inUse.map((value) => ({ value, hint })),
    ...offered.filter((s) => !inUse.includes(s.value)),
  ];
}
