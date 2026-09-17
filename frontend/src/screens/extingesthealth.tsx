// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge, EmptyState } from "../design-system/atoms";
import { type Fact, FactList } from "../design-system/factlist";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
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

function unitFacts(
  units: readonly UnitHealth[],
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
  zone: string,
): Fact[] {
  return units.map((unit) => ({
    key: unit.unit,
    // The unit's own name, verbatim: it is what the operator greps the
    // connector's log with and what names the directory the mapping lives in.
    term: <span>{unit.unit}</span>,
    value: (
      <span>
        {unit.refusals.map((refusal) => (
          <Badge key={refusal.refusal} tone="warn">
            {t("extIngest.refusalCount", {
              count: formatNumber(refusal.refused, locale),
              refusal: t(REFUSAL_LABEL[refusal.refusal]),
            })}
          </Badge>
        ))}
      </span>
    ),
    note: (
      <>
        {plural("extIngest.refusedTotal", unit.refused, {
          count: formatNumber(unit.refused, locale),
        })}
        {/* Only when the report carried one. A unit is listed because it
            refused something, so this is present in practice — but a line
            reading "most recent" with nothing after it would state a value the
            report never sent. */}
        {unit.last_refused_at !== null &&
          unit.last_refused_at !== undefined && (
            <>
              {" · "}
              {t("extIngest.lastRefused", {
                when: formatDateTime(unit.last_refused_at, locale, zone),
              })}
            </>
          )}
      </>
    ),
  }));
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
      <EmptyState>
        {t("extIngest.empty", {
          days: formatNumber(health.window_days, locale),
        })}
      </EmptyState>
    );
  }

  return (
    <SettingList>
      <SettingRow
        label={t("settings.extIngest")}
        description={t("extIngest.noDetail")}
        layout="stack"
        control={
          <div className="settingrow-measure">
            <FactList
              numeric
              facts={unitFacts(health.units, t, plural, locale, zone)}
            />
          </div>
        }
      />
    </SettingList>
  );
}

export function ExtensionIngestHealthCard() {
  const t = useT();
  const { locale } = useLocale();
  // The reader's own resolved zone, for the same reason the job card resolves
  // one: "most recent at 03:14" is only useful against the clock on their wall.
  const zone = viewerZone();
  // `job_health:read`, which is what the endpoint asks for
  // (compose/extingesthealth.go). The same operational question as the card
  // above, for the same reader, so the same grant decides both.
  const canSee = useCan("job_health", "read");
  const query = useQuery({
    queryKey: ["extension-ingest-health"],
    enabled: canSee,
    queryFn: async () => {
      const { data, error } = await api.GET("/admin/extension-ingest-health");
      if (error) {
        throwProblem(error);
      }
      // Rejected rather than defaulted. `?? []` would draw the clean state,
      // which claims every record this installation's connectors sent was
      // representable — a statement about the installation that this response
      // never made, and exactly the reassurance an operator opens the card to
      // trust. A malformed payload is a condition to report, so it becomes the
      // card's error state.
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

  return (
    <HealthCard
      title={t("settings.extIngest")}
      sub={t("settings.extIngestSub")}
      withheld={t("extIngest.adminOnly")}
      canSee={canSee}
      query={query}
      footer={(report) =>
        t("extIngest.generatedAt", {
          time: formatDateTime(report.generated_at, locale, zone),
        })
      }
    >
      {(health) => <ExtensionIngestBody health={health} zone={zone} />}
    </HealthCard>
  );
}
