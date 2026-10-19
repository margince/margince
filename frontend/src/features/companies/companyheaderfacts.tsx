// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Popover } from "../../design-system/popover";
import { provenanceOf } from "../../design-system/provenanceof";
import { Fact, RecordFacts } from "../../design-system/recordfacts";
import { ProvenanceTag } from "../../design-system/trust";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { useViewerId } from "../../lib/session";
import {
  CompanyOwnerControl,
  companyWebsite,
  displayHost,
} from "./companyheader";

// The account's facts: the one line under its name (what it is, who holds it,
// the way in), and its origin (when its row was written and by whom), which
// stands in the fields column beside the record.

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

/**
 * CompanyOriginFacts is when the account's row was written and by whom: a
 * fact about the record rather than about the account, so it stands with the
 * fields beside the record and not in the line under its name.
 */
export function CompanyOriginFacts({
  company,
}: Readonly<{ company: Company }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const viewerId = useViewerId();
  const provenance = provenanceOf(
    company.captured_by,
    viewerId,
    company.author,
    company.source,
    company.imported_from,
  );
  const authorName = useMemberName(
    provenance.kind === "human" ? provenance.userId : undefined,
  );
  return (
    <RecordFacts>
      <Fact label={t("list.created")}>
        {formatDateAbbrev(company.created_at, locale, zone)}
      </Fact>
      {/* The provenance pill names the KIND of source; the popover behind it
          answers WHICH one, on the same rule the contact page's Source fact
          keeps for a mailbox capture's message id. */}
      <Fact label={t("history.field.source")}>
        <Popover
          label={
            <ProvenanceTag
              provenance={provenance}
              renderUser={() => authorName.data ?? undefined}
            />
          }
        >
          <p className="t-body">{company.source || t("trust.sourceUnknown")}</p>
        </Popover>
      </Fact>
    </RecordFacts>
  );
}
