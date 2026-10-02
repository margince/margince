// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil, Trash2 } from "lucide-react";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import type { components } from "../api/schema";
import { useCan, useCanUpsert } from "../app/capability";
import { Badge, Button, Modal } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { today } from "../format/calendarday";
import { stable } from "../format/collate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { borrowedRows, useAiModelCatalogue } from "./ai-models";
import { usePriceSync } from "./ai-price-sync";
import { pricingPageFor } from "./ai-provider-links";
import { providerName } from "./ai-provider-names";
import type { ProviderUse } from "./ai-routing-query";
import { ProviderRefreshLine } from "./rate-catalogue-refresh";
import { type BoundModel, PriceForm } from "./rate-manual";
import { RemovePriceDialog } from "./rate-remove";
import "./ai-settings.css";

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];
type SheetRow = components["schemas"]["AiModelRate"];

export type ProviderUsage = ProviderUse;

// Whether a vendor can be called (`usable`, the server's answer) crossed with
// whether routing binds it. Four readings, and the two worth a reader's attention are the off-diagonal
// ones: a binding with nothing to call it with fails closed, and a keyed vendor
// nothing is bound to is ready to take a binding.
export type ProviderState = "active" | "ready" | "needs_key" | "inactive";

export function providerState(
  status: ProviderStatus,
  usage: ProviderUsage | undefined,
): ProviderState {
  const usable = status.usable;
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
  connection,
  onClose,
}: Readonly<{
  status: ProviderStatus;
  usage: ProviderUsage | undefined;
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
          {providerName(status.provider, t)}
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
          pricedBy={status.priced_by}
          usage={usage}
        />
      </div>
    </Modal>
  );
}

// Where focus lands when the prices section takes it back: the verb that opens
// the form when a writer holds one, else the first control in the section.
// Two lookups rather than one selector list, which answers in document order
// and would hand focus to whichever button comes first.
function firstVerb(section: HTMLElement | null): HTMLElement | null {
  return (
    section?.querySelector<HTMLElement>("button.btn-primary") ??
    section?.querySelector<HTMLElement>("button") ??
    null
  );
}

// What is billed per model for this vendor, and the way to change it. Absent
// for a reader without the sheet's read grant: an empty table there would say
// the vendor has no prices, which is a claim about the data.
function ProviderPrices({
  provider,
  pricedBy,
  usage,
}: Readonly<{
  provider: string;
  pricedBy?: string;
  usage: ProviderUsage | undefined;
}>) {
  const t = useT();
  const canRead = useCan("ai_model_rate", "read");
  const canWrite = useCanUpsert("ai_model_rate");
  const sheet = useAiModelCatalogue(canRead);
  const sync = usePriceSync(canRead);
  const lastLine = sync.data?.last_run?.report.providers.find(
    (p) => p.provider === provider,
  );
  // The row being edited, `{}` for a new price, nothing while the table shows.
  const [form, setForm] = useState<{
    initial?: SheetRow;
    draft?: BoundModel;
  } | null>(null);
  // The row whose entry is about to leave the sheet, while the question is open.
  const [removing, setRemoving] = useState<SheetRow | null>(null);
  // Swapping the table for the form unmounts the button that was pressed, which
  // drops focus onto the page. Focus follows the swap: into the form's first
  // field, and back to the verb that opens it.
  const section = useRef<HTMLElement>(null);
  const swapped = useRef(false);
  useEffect(() => {
    if (!swapped.current) {
      swapped.current = true;
      return;
    }
    const target = form
      ? section.current?.querySelector<HTMLElement>('input, [role="combobox"]')
      : firstVerb(section.current);
    target?.focus();
  }, [form]);
  if (!canRead) return null;
  const rows = pricedRows(sheet.data ?? [], provider, pricedBy);
  const page = pricingPageFor(provider, usage?.baseUrls ?? []);
  // Models routing runs on this vendor that the sheet cannot price: the reason
  // a lane elsewhere on the page says "no price".
  const unpriced = (usage?.models ?? []).filter(
    (m) => !rows.some((r) => r.model_id === m.model && r.lane === m.lane),
  );
  return (
    <section className="ai-sheet-section" ref={section}>
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
            draft={form.draft}
            boundModels={usage?.models}
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
                  <span className="sr-only">
                    {" "}
                    {t("aiProviders.opensNewTab")}
                  </span>
                </a>
              </>
            ) : null}
          </p>
          <ProviderRefreshLine line={lastLine} />
          {canWrite &&
            unpriced.map((m) => (
              <p key={`${m.lane}/${m.model}`} className="t-sub">
                {t("aiProviders.unpriced", { model: m.model })}{" "}
                <Button variant="link" onClick={() => setForm({ draft: m })}>
                  {t("aiProviders.setPrice")}
                </Button>
              </p>
            ))}
          {rows.length === 0 ? (
            <p className="t-sub">{t("aiProviders.noPrices")}</p>
          ) : (
            <PriceTable
              rows={rows}
              provider={provider}
              verbs={canWrite}
              // A borrowed row is corrected by writing this provider's own
              // price for the model, which then overrides it.
              onEdit={(r) => setForm({ initial: { ...r, provider } })}
              onRemove={setRemoving}
            />
          )}
          {removing ? (
            <RemovePriceDialog
              row={removing}
              onClose={() => setRemoving(null)}
              returnFocusTo={() => firstVerb(section.current)}
            />
          ) : null}
        </>
      )}
    </section>
  );
}

// The rows that price this provider's calls: its own, and for a provider
// priced by another (the server's priced_by), the other's rows for models its
// own sheet does not list — the same fallback the server prices a call with.
function pricedRows(
  sheet: readonly SheetRow[],
  provider: string,
  pricedBy: string | undefined,
): SheetRow[] {
  const own = sheet.filter((r) => r.provider === provider);
  return [...own, ...borrowedRows(sheet, provider, pricedBy)].sort((a, b) =>
    stable(a.model_id, b.model_id),
  );
}

// The prices themselves, one row per model, with the two verbs a writer holds
// on a row: correcting the price, and taking the whole entry off the sheet.
function PriceTable({
  rows,
  provider,
  verbs,
  onEdit,
  onRemove,
}: Readonly<{
  rows: SheetRow[];
  provider: string;
  verbs: boolean;
  onEdit: (row: SheetRow) => void;
  onRemove: (row: SheetRow) => void;
}>) {
  const t = useT();
  return (
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
            render: (r) => (
              <span className="ai-sheet-model">
                {r.model_id}
                {r.effective_date > today() ? (
                  <Badge tone="info">
                    {t("aiRates.manual.from", { date: r.effective_date })}
                  </Badge>
                ) : null}
                {r.source === "manual" ? (
                  <Badge>{t("aiProviders.setByHand")}</Badge>
                ) : null}
                {r.provider !== provider ? (
                  <Badge>
                    {t("aiProviders.borrowedFrom", {
                      provider: providerName(r.provider, t),
                    })}
                  </Badge>
                ) : null}
              </span>
            ),
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
          ...(verbs
            ? [
                {
                  key: "actions",
                  header: t("table.actions"),
                  align: "end" as const,
                  render: (r: SheetRow) => (
                    <div className="cell-actions">
                      <Button
                        iconOnly
                        variant="ghost"
                        aria-label={`${t("aiRates.manual.edit")} ${r.model_id}`}
                        onClick={() => onEdit(r)}
                      >
                        <Pencil aria-hidden />
                      </Button>
                      {/* A borrowed row is the other provider's to remove. */}
                      {r.provider === provider ? (
                        <Button
                          iconOnly
                          variant="ghost"
                          aria-label={t("aiRates.remove.verb", {
                            model: r.model_id,
                          })}
                          onClick={() => onRemove(r)}
                        >
                          <Trash2 aria-hidden />
                        </Button>
                      ) : null}
                    </div>
                  ),
                },
              ]
            : []),
        ]}
      />
    </div>
  );
}
