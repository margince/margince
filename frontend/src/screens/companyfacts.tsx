// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three facts a rep checks before anything else on an account: what the
// open pipeline is worth, how much work is in flight, and whose account it is.
//
// They were not readable together. The pipeline figure lived only inside the
// commercial panel four cards down, the in-flight counts existed nowhere, and
// the owner sat mid-sentence in the identity meta line. This is RecordView's
// `controls` slot, the same one the deal record puts its standing in, so the
// two records read the same way.
//
// The owner is EDITABLE here, and this is its only mount. The deal box is
// read-only because a control there that looked editable but only navigated
// would be worse than a label; the company's owner control already writes, so
// moving it rather than copying it is what keeps one implementation.
//
// Lifecycle is deliberately absent. It stays in the record's name badge, where
// it is edited; a read-only copy of a value beside its live editor is two
// answers to one question, and the two disagree the moment somebody types.

import type { components } from "../api/schema";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { CompanyOwnerControl } from "./companyheader";
import "./companyfacts.css";

type Organization = components["schemas"]["Organization"];
type Organization360 = components["schemas"]["Organization360"];
type Commercial = NonNullable<
  NonNullable<Organization360["state_strip"]>["commercial"]
>;

/**
 * CompanyFacts is the account's standing: open pipeline, work in flight, owner.
 */
export function CompanyFacts({
  org,
  view,
}: Readonly<{
  org: Organization;
  // The 360 the page already holds. Absent while it loads, and each half of
  // it independently absent when a grant withheld it.
  view?: Organization360;
}>) {
  const t = useT();
  return (
    <dl className="co-facts">
      <div className="co-facts-item">
        <dt className="t-caption">{t("co.facts.pipeline")}</dt>
        <dd>
          <Pipeline view={view} />
        </dd>
      </div>
      <div className="co-facts-item">
        <dt className="t-caption">{t("co.facts.inFlight")}</dt>
        <dd>
          <InFlight view={view} />
        </dd>
      </div>
      <div className="co-facts-item">
        <dt className="t-caption">{t("co.pulse.owner")}</dt>
        <dd>
          <CompanyOwnerControl org={org} hideLabel />
        </dd>
      </div>
    </dl>
  );
}

/**
 * Pipeline is what the account's open deals are worth, in three states that a
 * reader must be able to tell apart.
 *
 * `commercial` absent is the READER lacking the deal grant — the state strip
 * itself is still there, so its absence is about permission and not about the
 * account. No open deals is a fact about the account. Open deals with no total
 * is a third thing again: the deals exist and nobody has priced them, and
 * printing a dash there would read as "we do not know", which is the one
 * reading it is not.
 */
function Pipeline({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!view) {
    return <span className="co-facts-quiet">{t("co.facts.reading")}</span>;
  }
  const commercial = view.state_strip?.commercial;
  if (!commercial) {
    return <span className="co-facts-quiet">{t("state.withheld")}</span>;
  }
  if (commercial.open_count === 0) {
    return <span className="co-facts-quiet">{t("co.facts.noDeals")}</span>;
  }
  return <span>{pricedTotal(commercial, locale, t)}</span>;
}

function pricedTotal(
  commercial: Commercial,
  locale: ReturnType<typeof useLocale>["locale"],
  t: ReturnType<typeof useT>,
): string {
  const total = commercial.open_pipeline_minor_base;
  if (total == null || !commercial.base_currency) {
    return t("co.facts.unpriced");
  }
  return formatMoney(total, commercial.base_currency, locale);
}

/**
 * InFlight counts the same two sections the work card lists, under the same
 * rule: a withheld half means no count at all, because a number that folds an
 * unreadable half into it is a false statement rather than a partial one.
 */
function InFlight({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (!view) {
    return <span className="co-facts-quiet">{t("co.facts.reading")}</span>;
  }
  if (!view.deals || !view.projects) {
    return <span className="co-facts-quiet">{t("state.withheld")}</span>;
  }
  const deals = view.deals.data.length;
  const projects = view.projects.filter(
    (project) => project.phase !== "closed",
  ).length;
  if (deals === 0 && projects === 0) {
    return <span className="co-facts-quiet">{t("co.facts.nothing")}</span>;
  }
  // Each half carries its own plural. One shared "{deals} deals · {projects}
  // projects" printed "1 projects" on any account with a single project, and
  // the two counts are independent — one can be singular while the other is
  // not, so one template cannot serve both.
  return (
    <span>
      {plural("co.facts.deals", deals, {
        count: formatNumber(deals, locale),
      })}
      {" · "}
      {plural("co.facts.projects", projects, {
        count: formatNumber(projects, locale),
      })}
      {(view.deals.page.has_more || view.projects_page?.has_more) &&
        ` ${t("co.facts.atLeast")}`}
    </span>
  );
}
