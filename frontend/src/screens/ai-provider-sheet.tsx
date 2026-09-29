// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { useCan, useCanUpsert } from "../app/capability";
import { Badge, Button, Modal } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useAiModelCatalogue } from "./ai-models";
import { pricingPageFor } from "./ai-provider-links";
import {
  type ModelPriceRefresh,
  ProviderRefreshLine,
} from "./rate-catalogue-refresh";
import { PriceForm } from "./rate-manual";
import "./ai-settings.css";

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];
type SheetRow = components["schemas"]["AiModelRate"];

export type ProviderUsage = { for: string[]; baseUrls: string[] };

// Whether a vendor can be called (`usable`) crossed with whether routing binds
// it. Four readings, and the two worth a reader's attention are the off-diagonal
// ones: a binding with nothing to call it with fails closed, and a keyed vendor
// nothing is bound to is ready to take a binding.
export type ProviderState = "active" | "ready" | "needs_key" | "inactive";

export function providerState(
  status: ProviderStatus,
  usage: ProviderUsage | undefined,
): ProviderState {
  const usable = status.configured || status.optional || status.env_var === "";
  if (usage) return usable ? "active" : "needs_key";
  return usable ? "ready" : "inactive";
}

export const STATE_LABEL = {
  active: "aiProviders.state.active",
  ready: "aiProviders.state.ready",
  needs_key: "aiProviders.state.needsKey",
  inactive: "aiProviders.state.inactive",
} as const satisfies Record<ProviderState, MessageKey>;

export const STATE_TONE = {
  active: "success",
  ready: "info",
  needs_key: "warning",
  inactive: "default",
} as const satisfies Record<
  ProviderState,
  "success" | "info" | "warning" | "default"
>;

/**
 * Everything about one vendor in one right-hand sheet: its credential (the
 * `connection` the card hands in, so the key logic stays with the key hooks),
 * and the prices its models are billed at. Adding or editing a price swaps the
 * table for the form inside this sheet rather than opening a second dialog.
 */
export function ProviderSheet({
  status,
  usage,
  refresh,
  connection,
  onClose,
}: Readonly<{
  status: ProviderStatus;
  usage: ProviderUsage | undefined;
  refresh: ModelPriceRefresh;
  connection: ReactNode;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const state = providerState(status, usage);
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
      size="wide"
    >
      <div className="drawer-head">
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {status.provider}
        </Heading>
        <p className="t-caption ai-sheet-status">
          <Badge tone={STATE_TONE[state]}>{t(STATE_LABEL[state])}</Badge>
          <span>
            {usage
              ? t("aiProviders.usedBy", { roles: usage.for.join(", ") })
              : t("aiProviders.notUsed")}
          </span>
        </p>
      </div>
      <div className="drawer-body">
        <section className="ai-sheet-section">
          <Heading size="small" className="t-h3">
            {t("aiProviders.connection")}
          </Heading>
          {connection}
        </section>
        <ProviderPrices
          provider={status.provider}
          usage={usage}
          refresh={refresh}
        />
      </div>
    </Modal>
  );
}

// What is billed per model for this vendor, and the way to change it. Absent
// for a reader without the sheet's read grant: an empty table there would say
// the vendor has no prices, which is a claim about the data.
function ProviderPrices({
  provider,
  usage,
  refresh,
}: Readonly<{
  provider: string;
  usage: ProviderUsage | undefined;
  refresh: ModelPriceRefresh;
}>) {
  const t = useT();
  const canRead = useCan("ai_model_rate", "read");
  const canWrite = useCanUpsert("ai_model_rate");
  const sheet = useAiModelCatalogue();
  // The row being edited, `{}` for a new price, nothing while the table shows.
  const [form, setForm] = useState<{ initial?: SheetRow } | null>(null);
  if (!canRead) return null;
  const rows = (sheet.data ?? []).filter((r) => r.provider === provider);
  const page = pricingPageFor(provider, usage?.baseUrls ?? []);
  return (
    <section className="ai-sheet-section">
      <div className="ai-sheet-heading">
        <Heading size="small" className="t-h3">
          {t("aiProviders.prices")}
        </Heading>
        {canWrite && form === null ? (
          <Button variant="primary" onClick={() => setForm({})}>
            {t("aiProviders.addPrice")}
          </Button>
        ) : null}
      </div>
      {form !== null ? (
        <>
          <div>
            <Button variant="link" onClick={() => setForm(null)}>
              {t("aiProviders.backToPrices")}
            </Button>
          </div>
          <PriceForm
            provider={provider}
            initial={form.initial}
            onDone={() => setForm(null)}
            onCancel={() => setForm(null)}
          />
        </>
      ) : (
        <>
          <p className="t-caption">
            {t("aiProviders.priceUnit")}
            {page ? (
              <>
                {" · "}
                <a href={page} target="_blank" rel="noreferrer noopener">
                  {t("aiProviders.priceSource")}
                </a>
              </>
            ) : null}
          </p>
          <ProviderRefreshLine refresh={refresh} provider={provider} />
          {rows.length === 0 ? (
            <p className="t-sub">{t("aiProviders.noPrices")}</p>
          ) : (
            <div className="ai-sheet-table">
              <DataTable<SheetRow>
                label={t("aiProviders.prices")}
                rows={rows}
                rowKey={(r) => `${r.lane}/${r.model_id}`}
                columns={[
                  {
                    key: "model",
                    header: t("settings.rates.colModel"),
                    grow: true,
                    render: (r) => r.model_id,
                  },
                  {
                    key: "in",
                    header: t("aiProviders.colInput"),
                    align: "end",
                    render: (r) => r.input_per_mtok,
                  },
                  {
                    key: "out",
                    header: t("aiProviders.colOutput"),
                    align: "end",
                    render: (r) => r.output_per_mtok,
                  },
                  {
                    key: "cr",
                    header: t("aiProviders.colCacheRead"),
                    align: "end",
                    render: (r) => r.cache_read_per_mtok,
                  },
                  {
                    key: "cw",
                    header: t("aiProviders.colCacheWrite"),
                    align: "end",
                    render: (r) => r.cache_write_per_mtok,
                  },
                  ...(canWrite
                    ? [
                        {
                          key: "edit",
                          header: t("aiRates.manual.edit"),
                          align: "end" as const,
                          render: (r: SheetRow) => (
                            <Button
                              variant="ghost"
                              onClick={() => setForm({ initial: r })}
                            >
                              {t("aiRates.manual.edit")}
                              <span className="sr-only"> {r.model_id}</span>
                            </Button>
                          ),
                        },
                      ]
                    : []),
                ]}
              />
            </div>
          )}
        </>
      )}
    </section>
  );
}
