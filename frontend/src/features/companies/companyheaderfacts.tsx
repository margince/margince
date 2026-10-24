// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { formatNumber } from "../../format/format";
import { industryWords } from "../../format/industry";
import { useLocale, usePlural, useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import { companyWebsite, displayHost } from "./companyheader";

// The lines under the account's name.

type Company = components["schemas"]["Company"];
type Company360 = components["schemas"]["Company360"];

/**
 * CompanyHeadLines are the lines under the account's name, one fact each:
 * what it is, its site, and the best way in. Nothing is drawn for a fact the
 * account does not have, so the head never opens on an empty line.
 */
export function CompanyHeadLines({
  company,
  view,
  loading,
}: Readonly<{
  company: Company;
  // The 360 the page already holds; its strength names the way in.
  view?: Company360;
  loading?: boolean;
}>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const website = companyWebsite(company);
  const wayIn = loading ? undefined : view?.strength;
  // The 360 already carries the way-in contact's name, so the line reads it
  // from there; the reference only resolves a contact the page did not list.
  const named = view?.contacts?.data.find(
    (row) => row.contact_id === wayIn?.contributor_contact_id,
  );
  return (
    <>
      {company.industry && <span>{industryWords(t, company.industry)}</span>}
      {website && (
        <a className="entity-link" href={website}>
          {displayHost(website)}
        </a>
      )}
      {wayIn?.contributor_contact_id && (
        <span>
          {t("co.pulse.strongestLead")}{" "}
          {/* No space before the tail: it opens with its own comma. */}
          {named ? (
            <a
              className="entity-link"
              href={routeHash({ screen: "contacts", id: named.contact_id })}
            >
              {named.full_name}
            </a>
          ) : (
            <EntityRef kind="contact" id={wayIn.contributor_contact_id} />
          )}
          {plural("co.pulse.strengthTail", wayIn.contact_count, {
            count: formatNumber(wayIn.contact_count, locale),
          })}
        </span>
      )}
    </>
  );
}
