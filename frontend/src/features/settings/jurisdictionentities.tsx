// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { AppLink } from "../../design-system/applink";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { jurisdictionName } from "../../format/jurisdiction";
import { useLocale, useT } from "../../i18n";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { throwProblem } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";

// The default company's country is the installation's jurisdiction, so one
// value answers for it everywhere. Further companies each state their own.

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

/** The default company's country on its editor, read-only, with the way to change it. */
export function DefaultEntityCountry() {
  const t = useT();
  const { locale } = useLocale();
  const query = useInstallationJurisdiction();
  return (
    <QueryGate query={query} pendingLabel={t("legalEntities.country")}>
      {(jurisdiction) => (
        <p className="t-caption">
          {jurisdiction.selected
            ? t("legalEntities.defaultCountry", {
                country: jurisdictionName(jurisdiction.selected, locale),
              })
            : t("legalEntities.defaultCountryNone")}{" "}
          <AppLink href={routeHash(settingsHref("jurisdiction"))}>
            {t("legalEntities.defaultCountryLink")}
          </AppLink>
        </p>
      )}
    </QueryGate>
  );
}

/** Every live company and the country whose law it answers under. */
export function JurisdictionEntitiesPanel({
  selected,
}: Readonly<{ selected: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const entities = useLegalEntities();
  const live = entities.data ?? [];
  if (live.length < 2) return null;
  const countryOf = (code: string | null | undefined) =>
    code ? jurisdictionName(code, locale) : t("jurisdiction.entities.none");
  return (
    <Panel title={t("jurisdiction.entities.title")}>
      <PanelBody>
        <PanelIntro>{t("jurisdiction.entities.lead")}</PanelIntro>
        <FactList
          facts={live.map((entity) => ({
            key: entity.id,
            term: entity.display_name || t("legalEntities.defaultUnnamed"),
            value: entity.is_default
              ? t("jurisdiction.entities.default", {
                  country: countryOf(selected),
                })
              : countryOf(entity.country),
          }))}
        />
        <AppLink href={routeHash(settingsHref("company", "entities"))}>
          {t("jurisdiction.entities.edit")}
        </AppLink>
      </PanelBody>
    </Panel>
  );
}
