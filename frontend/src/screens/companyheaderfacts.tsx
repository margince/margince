// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Popover } from "../design-system/popover";
import { Fact, RecordFacts } from "../design-system/recordfacts";
import { ProvenanceTag } from "../design-system/trust";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { provenanceOf, useViewerId } from "./common";
import { companyWebsite, displayHost } from "./companyheader";
import {
  EntityRef,
  rosterOwnerName,
  useRoster,
  useRosterPartial,
} from "./entityref";

// The account's name-line subtitle and its facts strip: what CompanyIdentityLine
// used to draw as one running sentence, in the contact record page's own
// shapes, an inline subtitle beside the name, and named cells under it.

type Company = components["schemas"]["Company"];
type Company360 = components["schemas"]["Company360"];

/**
 * CompanySubtitle is the name line's own subtitle, beside the name rather
 * than under everything else the header carries: what the account is, and
 * the one way in every reader already knows, the same inline shape
 * contactpage.tsx's ContactSubtitle draws for a contact's title and employer.
 */
export function CompanySubtitle({
  company,
}: Readonly<{ company: Company }>): ReactNode {
  const website = companyWebsite(company);
  return (
    <div className="record-sub record-sub-inline">
      {company.industry}
      {website && (
        <>
          {company.industry ? " · " : ""}
          <a className="co-meta-link" href={website}>
            {displayHost(website)}
          </a>
        </>
      )}
    </div>
  );
}

/**
 * CompanyIdentityFacts is the head's facts strip: the way in, who holds the
 * account, and when its own row was written and by whom. Re-houses what
 * CompanyIdentityLine drew as one sentence, as the same named cells every
 * other record page's facts strip carries (RecordFacts).
 */
export function CompanyIdentityFacts({
  company,
  view,
  loading,
}: Readonly<{
  company: Company;
  // The 360 the page already holds. Absent while it loads, and the strength
  // reading absent again once it lands, for a company nobody has a way in on.
  view?: Company360;
  loading?: boolean;
}>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const viewerId = useViewerId();
  const roster = useRoster("user", true);
  const rosterPartial = useRosterPartial("user", true);
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
      <Fact label={t("co.pulse.owner")}>
        {rosterOwnerName(
          company.owner_id,
          roster,
          rosterPartial,
          t,
          t("co.pulse.unowned"),
        )}
      </Fact>
      {wayIn?.contributor_contact_id && (
        <Fact label={t("co.pulse.strongestLead")}>
          {/* No space before the tail: it opens with its own comma, so the
              name and the clause read as one sentence. */}
          <EntityRef kind="contact" id={wayIn.contributor_contact_id} />
          {plural("co.pulse.strengthTail", wayIn.contact_count, {
            count: formatNumber(wayIn.contact_count, locale),
          })}
        </Fact>
      )}
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
              provenance={provenanceOf(
                company.captured_by,
                viewerId,
                company.author,
              )}
              renderUser={companyAuthorName(roster.data)}
            />
          }
        >
          <p className="t-body">{company.source || t("trust.sourceUnknown")}</p>
        </Popover>
      </Fact>
    </RecordFacts>
  );
}

// Resolves a `captured_by` human id to the name the owner control already
// reads off the same roster, rather than the generic "typed by a person" the
// tag falls back to without one: the header has always had the roster in
// hand, so a record every colleague can see is named for who wrote it.
function companyAuthorName(
  roster: ReturnType<typeof useRoster>["data"],
): (userId: string) => ReactNode {
  return (userId: string) => {
    const entry = roster?.find((candidate) => candidate.id === userId);
    return entry && "display_name" in entry ? entry.display_name : undefined;
  };
}
