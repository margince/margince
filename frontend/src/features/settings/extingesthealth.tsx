// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyedName } from "../../design-system/keyedname";
import { PanelBody, PanelIntro } from "../../design-system/panel";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { extensionTitle } from "../../lib/extensions";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";

// GET /admin/extension-ingest-health — whether an installed connector is
// sending records this CRM cannot represent.
//
// A connector moves past a record it cannot get accepted, because stopping on
// one malformed message parks the whole feed. So the drop left the connector's
// own log and nothing else, and a provider that changed its format looked
// exactly like a provider with nothing to send. This card is the half that
// makes the difference visible — the endpoint beside it would otherwise be one
// nothing reads, which is what the job-health card already had to be built to
// fix once.

type Health = components["schemas"]["ExtensionIngestHealth"];
type UnitHealth = components["schemas"]["ExtensionUnitIngestHealth"];
type Refusal = components["schemas"]["ExtensionIngestRefusal"];

// The refusal vocabulary, in the reader's words rather than the wire's. Keyed
// on the union so a class added to the contract is a compile error here instead
// of a raw token on the screen — the operator is being told which mapping to
// look at, and "counterparty" is a word they can act on where a bare enum value
// is one they have to look up.
const REFUSAL_LABEL: Record<Refusal["refusal"], MessageKey> = {
  key: "extIngest.refusal.key",
  activity: "extIngest.refusal.activity",
  addresses: "extIngest.refusal.addresses",
  counterparty: "extIngest.refusal.counterparty",
  participants: "extIngest.refusal.participants",
  size: "extIngest.refusal.size",
};

function unitColumns(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  zone: string,
): DataTableColumn<UnitHealth>[] {
  return [
    {
      key: "unit",
      header: t("extIngest.col.connector"),
      // The key under the title is what the connector's log and directory carry.
      render: (unit) => (
        <KeyedName name={extensionTitle(unit.unit, locale)} code={unit.unit} />
      ),
    },
    {
      key: "refused",
      header: t("extIngest.col.refused"),
      align: "end",
      render: (unit) => formatNumber(unit.refused, locale),
      folded: (unit) =>
        plural("extIngest.refusedTotal", unit.refused, {
          count: formatNumber(unit.refused, locale),
        }),
    },
    {
      key: "reasons",
      header: t("extIngest.col.reasons"),
      grow: true,
      render: (unit) => (
        <CellStack>
          {unit.refusals.map((refusal) => (
            <span key={refusal.refusal}>
              {t("extIngest.refusalCount", {
                count: formatNumber(refusal.refused, locale),
                refusal: t(REFUSAL_LABEL[refusal.refusal]),
              })}
            </span>
          ))}
        </CellStack>
      ),
    },
    {
      key: "last",
      header: t("extIngest.col.lastRefused"),
      // A unit is listed because it refused something, so a time is present in
      // practice; a missing one stays blank rather than claiming a value.
      render: (unit) =>
        unit.last_refused_at
          ? formatDateTime(unit.last_refused_at, locale, zone)
          : null,
      folded: (unit) =>
        unit.last_refused_at
          ? t("extIngest.lastRefused", {
              when: formatDateTime(unit.last_refused_at, locale, zone),
            })
          : undefined,
    },
  ];
}

function ExtensionIngestBody({
  health,
  zone,
}: Readonly<{ health: Health; zone: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();

  if (health.units.length === 0) {
    // Its own words, naming the window. "Nothing here" would read as a card
    // with no answer, and the answer — every record was representable — is
    // precisely what the reader came for.
    return (
      <PanelBody>
        <EmptyState>
          {t("extIngest.empty", {
            days: formatNumber(health.window_days, locale),
          })}
        </EmptyState>
      </PanelBody>
    );
  }

  return (
    <DataTable
      bleed
      fold
      label={t("settings.extIngest")}
      columns={unitColumns(t, plural, locale, zone)}
      rows={[...health.units]}
      rowKey={(unit) => unit.unit}
    />
  );
}

// `job_health:read`, what the endpoint asks for (compose/extingesthealth.go):
// the job card's question for the same reader, so the same grant decides both.
export function useExtensionIngestHealth() {
  const canSee = useCan("job_health", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: ["extension-ingest-health", scope],
    enabled: canSee && visible && scope !== undefined,
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/admin/extension-ingest-health", { signal }),
      );
      // Rejected, not defaulted: `?? []` would draw the clean state, a claim about
      // the installation this response never made.
      if (
        !data ||
        typeof data.generated_at !== "string" ||
        typeof data.window_days !== "number" ||
        !Array.isArray(data.units)
      ) {
        throw new Error("malformed extension-ingest-health response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function ExtensionIngestHealthCard() {
  const t = useT();
  // The reader's own zone: "most recent at 03:14" is only useful against the
  // clock on their wall.
  const zone = viewerZone();
  const { canSee, query } = useExtensionIngestHealth();

  return (
    <HealthCard
      title={t("settings.extIngest")}
      sub={t("settings.extIngestSub")}
      withheld={t("extIngest.adminOnly")}
      canSee={canSee}
      query={query}
      lead={(health) =>
        health.units.length > 0 && (
          <PanelIntro>{t("extIngest.noDetail")}</PanelIntro>
        )
      }
    >
      {(health) => <ExtensionIngestBody health={health} zone={zone} />}
    </HealthCard>
  );
}
