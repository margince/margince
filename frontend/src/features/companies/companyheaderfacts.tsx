// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Fact, RecordFacts } from "../../design-system/recordfacts";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  CompanyOwnerControl,
  companyWebsite,
  displayHost,
} from "./companyheader";

// The account's facts. The one line under its name says what it is, who holds
// it and the way in. Its origin, when its row was written and by whom, stands
// in the fields column beside the record.

type Company = components["schemas"]["Company"];
type Company360 = components["schemas"]["Company360"];

/**
 * CompanyIdentityFacts is the head's facts line: what the account is, who
 * holds it and the way in.
 */
export function CompanyIdentityFacts({
  company,
  view,
  loading,
}: Readonly<{
  company: Company;
  // The composite read, and whether it is still in flight. The way-in fact
  // reads from it, so it waits for the read rather than guessing.
  view?: Company360;
  loading?: boolean;
}>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const website = companyWebsite(company);
  const wayIn = loading ? undefined : view?.strength;
  return (
    <RecordFacts>
      {website && (
        <Fact label={t("field.domain")}>
          <a className="co-meta-link" href={website}>
            {displayHost(website)}
          </a>
        </Fact>
      )}
      {company.industry && (
        <Fact label={t("history.field.industry")}>{company.industry}</Fact>
      )}
      {company.size_band && (
        <Fact label={t("history.field.size_band")}>
          {t("co.pulse.sizeBand", { band: company.size_band })}
        </Fact>
      )}
      {/* The owner control, not a read of the owner: on an unowned account it
          is the claim door, and a label here left that account with no way to
          become anybody's. */}
      <Fact label={t("co.pulse.owner")} named>
        <CompanyOwnerControl company={company} hideLabel />
      </Fact>
      {wayIn?.contributor_contact_id && (
        <Fact label={t("co.pulse.strongestLead")} named>
          {/* No space before the tail: it opens with its own comma, so the
              name and the clause read as one sentence. */}
          <EntityRef kind="contact" id={wayIn.contributor_contact_id} />
          {plural("co.pulse.strengthTail", wayIn.contact_count, {
            count: formatNumber(wayIn.contact_count, locale),
          })}
        </Fact>
      )}
    </RecordFacts>
  );
}
