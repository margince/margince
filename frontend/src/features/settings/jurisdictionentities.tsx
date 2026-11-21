// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { AppLink } from "../../design-system/applink";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { jurisdictionName } from "../../format/jurisdiction";
import { type Locale, useLocale, useT } from "../../i18n";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { throwProblem } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";

// The default company's jurisdiction is the installation's, so one value
// answers for it everywhere. Further companies each state their own.

export const JURISDICTION_QUERY_KEY = ["installation-jurisdiction"] as const;

/** The installation's selected pack code, or null when none is set. */
export function useInstallationJurisdiction() {
  return useQuery({
    queryKey: JURISDICTION_QUERY_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/jurisdiction",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/** The packs this build offers, as Select options named in the reader's language. */
export function jurisdictionOptions(
  available: readonly Readonly<{ code: string }>[],
  locale: Locale,
): { value: string; label: string }[] {
  return available.map((unit) => ({
    value: unit.code,
    label: jurisdictionName(unit.code, locale),
  }));
}

/** The default company's jurisdiction on its editor, read-only, with the way to change it. */
export function DefaultEntityJurisdiction() {
  const t = useT();
  const { locale } = useLocale();
  const query = useInstallationJurisdiction();
  return (
    <QueryGate query={query} pendingLabel={t("legalEntities.jurisdiction")}>
      {(jurisdiction) => (
        <p className="t-caption">
          {jurisdiction.selected
            ? t("legalEntities.defaultJurisdiction", {
                jurisdiction: jurisdictionName(jurisdiction.selected, locale),
              })
            : t("legalEntities.defaultJurisdictionNone")}{" "}
          <AppLink href={routeHash(settingsHref("jurisdiction"))}>
            {t("legalEntities.defaultJurisdictionLink")}
          </AppLink>
        </p>
      )}
    </QueryGate>
  );
}

/** Every live company and the jurisdiction whose law it answers under. */
export function JurisdictionEntitiesPanel({
  selected,
}: Readonly<{ selected: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const entities = useLegalEntities();
  const live = entities.data ?? [];
  if (live.length < 2) return null;
  const lawOf = (code: string | null | undefined) =>
    code ? jurisdictionName(code, locale) : t("jurisdiction.entities.none");
  return (
    <Panel title={t("jurisdiction.entities.title")}>
      <PanelBody>
        <FactList
          facts={live.map((entity) => ({
            key: entity.id,
            term: entity.display_name || t("legalEntities.defaultUnnamed"),
            value: entity.is_default
              ? t("jurisdiction.entities.default", {
                  jurisdiction: lawOf(selected),
                })
              : lawOf(entity.jurisdiction),
          }))}
        />
        <AppLink href={routeHash(settingsHref("company", "entities"))}>
          {t("jurisdiction.entities.edit")}
        </AppLink>
      </PanelBody>
    </Panel>
  );
}
