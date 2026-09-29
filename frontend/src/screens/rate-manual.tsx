// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ComboBox } from "../design-system/combobox";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { calendarDay } from "../format/calendarday";
import { formatUsdPerMTok } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  offeredModels,
  unreadablePrice,
  useAiModelCatalogue,
  useAvailableModels,
} from "./ai-models";
import { problemMessageOf, throwProblem, WriteRefused } from "./common";

// What the server's pattern for a per-MTok price accepts: a plain non-negative
// decimal, up to twelve whole digits and six fractional. Zero is a price (a
// local model costs nothing to call), so it passes.
const PRICE = /^[0-9]{1,12}(\.[0-9]{1,6})?$/;

// The reader's own today, for the same reason the sheets read effective dates
// against their calendar rather than UTC's.
function today(): string {
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
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const sheet = useAiModelCatalogue();
  const available = useAvailableModels(
    fixedProvider ?? "",
    "",
    fixedProvider !== undefined,
  );

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
    (v) => v.trim() !== "" && !PRICE.test(v.trim()),
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
    setInput(row.input_per_mtok);
    setOutput(row.output_per_mtok);
    setCacheRead(row.cache_read_per_mtok);
    setCacheWrite(row.cache_write_per_mtok);
  };

  return (
    <Modal open onClose={onClose} labelledBy={labelId}>
      <Heading size="large" id={labelId} className="t-h2 modal-title">
        {fixedProvider === undefined
          ? t("settings.rates.modelModalTitle")
          : t("aiRates.manual.title", { provider: fixedProvider })}
      </Heading>
      <div className="form-stack">
        {existing.length > 0 ? (
          <ul
            className="rates-manual-list"
            aria-label={t("aiRates.manual.priced")}
          >
            {existing.map((row) => (
              <li key={row.model_id}>
                <span>{row.model_id}</span>
                <span className="t-caption">
                  {unreadablePrice(row.input_per_mtok) ||
                  unreadablePrice(row.output_per_mtok)
                    ? ""
                    : t("aiAdmin.rates", {
                        input: formatUsdPerMTok(row.input_per_mtok, locale),
                        output: formatUsdPerMTok(row.output_per_mtok, locale),
                      })}
                </span>
                <Button variant="ghost" onClick={() => edit(row)}>
                  {t("aiRates.manual.edit")}
                  <span className="sr-only"> {row.model_id}</span>
                </Button>
              </li>
            ))}
          </ul>
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
          <Field label={t("settings.rates.colModel")}>
            {(control) => (
              <ComboBox
                {...control}
                value={modelId}
                suggestions={offeredModels(
                  available.data,
                  sheet.data,
                  fixedProvider,
                  "chat",
                  locale,
                )}
                onChange={(next) => {
                  setSaved(null);
                  setModelId(next);
                }}
              />
            )}
          </Field>
        )}
        {field(t("settings.rates.colInput"), input, setInput, {
          placeholder: "5.00",
        })}
        {field(t("settings.rates.colOutput"), output, setOutput, {
          placeholder: "25.00",
        })}
        {field(t("settings.rates.colCacheRead"), cacheRead, setCacheRead)}
        {field(t("settings.rates.colCacheWrite"), cacheWrite, setCacheWrite)}
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
        {malformed ? (
          <ErrorLine>{t("aiRates.manual.malformed")}</ErrorLine>
        ) : null}
        {saved ? (
          <p className="t-caption" role="status">
            {saved}
          </p>
        ) : null}
        <WriteRefused titleKey="settings.rates.notSaved" message={error} />
        <div className="form-actions">
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
      </div>
    </Modal>
  );
}
