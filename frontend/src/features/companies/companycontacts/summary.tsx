import { useQuery } from "@tanstack/react-query";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { StatCard } from "../../../design-system/atoms";
import { ReadingsGrid } from "../../../design-system/readingsgrid";
import { formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { ENTITY } from "../../../lib/entity";
import { throwProblem } from "../../../lib/problem";
import { routeHash } from "../../../lib/router";
import { ENGAGEMENT_LABELS } from "./contacts";
import "./companycontacts.css";

import { CommitteeBoard } from "./summary.board";
import { ROLE_LABELS } from "./summary.roles";

export { mapLabels } from "./summary.board";
export { buyingRoleLabel } from "./summary.roles";

export type Coverage = components["schemas"]["CompanyCoverage"];

export type Seat = components["schemas"]["CompanyCoverageSeat"];

export function useCompanyCoverage(companyId: string) {
  return useQuery({
    queryKey: ["company-coverage", companyId],
    queryFn: async () => {
      const { data, error } = await api.GET("/companies/{id}/coverage", {
        params: { path: { id: companyId } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
  });
}

// What the account looks like before the reader picks anybody.
//
// The list below answers "who, in what order". This band answers the three
// questions a rep opens the tab with. Is anybody here talking to us? Who is
// the way in? Which buying role does nobody hold?
export function CoverageBand({
  companyId,
  accountName,
  onNarrow,
}: Readonly<{
  companyId: string;
  accountName: string;
  /** The Coverage reading's door: it narrows the list below to the untried. */
  onNarrow: (status: "untried") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useCompanyCoverage(companyId);
  // A band that could not load is absent, not an error banner: the list below
  // it still answers, and a reading nobody can act on is not worth a shout.
  // `summary` is required by the contract, so its absence means the response
  // was not the one this reads — a stub, a proxy, a version skew. Half a band
  // off a payload this does not recognise is worse than none, and reading
  // through it crashes the tab.
  if (query.isPending) {
    return null;
  }
  // A read that FAILED says so. Rendering nothing makes a server error look
  // like a band this account does not offer, and the reader then works from a
  // list with no reading above it and no reason to doubt it. One slot, and it
  // folds like the row it stands in for.
  if (query.isError) {
    return (
      <ReadingsGrid label={t("co.contacts.band.label")} testId="coverage-band">
        <StatCard
          label={t("co.contacts.band.coverage")}
          value={t("co.contacts.band.unavailable")}
          detail={t("co.contacts.band.unavailableWhy")}
        />
      </ReadingsGrid>
    );
  }
  if (!query.data?.summary) {
    return null;
  }
  const coverage = query.data;
  return (
    <>
      {/* The roster's size first, then who answers, which role is open and
          the way in: four cards a reader uses one at a time. */}
      <ReadingsGrid label={t("co.contacts.band.label")} testId="coverage-band">
        <StatCard
          label={t("co.contacts.band.contacts")}
          value={formatNumber(coverage.summary.contacts_total, locale)}
          detail={t("co.contacts.band.onFile")}
        />
        <StatCard
          label={t("co.contacts.band.inTouch")}
          // Everyone who has written to us in the window, whichever side wrote
          // last, against the ROSTER: a bare "6" says nothing until the reader
          // knows whether the account holds seven contacts or seventy.
          value={t("co.contacts.band.reachable", {
            count: formatNumber(
              coverage.summary.answered + coverage.summary.waiting,
              locale,
            ),
            total: formatNumber(coverage.summary.contacts_total, locale),
          })}
          // Two gaps, and different work: nobody has approached the first at
          // all, and the second wrote to us and is still owed an answer.
          detail={t("co.contacts.band.untried", {
            count: formatNumber(coverage.summary.untried, locale),
            waiting: formatNumber(coverage.summary.waiting, locale),
          })}
          onOpen={() => onNarrow("untried")}
        />
        <CommitteeReading coverage={coverage} />
        <WayIn coverage={coverage} />
      </ReadingsGrid>
      <CommitteeBoard
        coverage={coverage}
        accountName={accountName}
        companyId={companyId}
      />
    </>
  );
}

/**
 * WayIn names the contact worth writing to and why. An account where everyone
 * was written to and nobody answered has no way in, and the card says so
 * rather than naming the least cold contact: a fourth follow-up dressed as an
 * opening is what this reading must not do.
 */
function WayIn({ coverage }: Readonly<{ coverage: Coverage }>) {
  const t = useT();
  const way = coverage.best_way_in;
  const slot = { label: t("co.contacts.band.wayIn") };
  if (!way) {
    return (
      <StatCard
        {...slot}
        value={t("co.contacts.band.noWayIn")}
        detail={t("co.contacts.band.noWayInWhy")}
      />
    );
  }
  // The card names one contact, so its door is that contact's page.
  // The name sits under the door's stretched target, so it is the same link.
  return (
    <StatCard
      {...slot}
      value={way.full_name}
      detail={
        <span>
          {way.title ? `${way.title} · ` : ""}
          {t(ENGAGEMENT_LABELS[way.engagement])}
        </span>
      }
      href={routeHash(ENTITY.contact.route(way.contact_id))}
    />
  );
}

/**
 * CommitteeReading says which critical role nobody holds. Three readings, not
 * two: a committee that could not be read says so, one read whole with a hole
 * names the hole, one read whole with none says it is complete. Collapsing the
 * first into either tells a reader a deal has no champion when it has one they
 * cannot see.
 */
function CommitteeReading({ coverage }: Readonly<{ coverage: Coverage }>) {
  const t = useT();
  const { locale } = useLocale();
  // A committee can be absent for two OPPOSITE reasons, and only the flag
  // tells them apart: the reader may not look, or there is nothing to look at.
  // Absence alone read as "hidden" accuses the reader's own role on every
  // account that simply has no open deal.
  const slot = { label: t("co.contacts.band.committee") };
  if (!coverage.committee) {
    const withheld = !coverage.completeness.committee_read;
    return (
      <StatCard
        {...slot}
        value={t(
          withheld
            ? "co.contacts.band.committeeUnread"
            : "co.contacts.band.noOpenDeal",
        )}
        detail={t(
          withheld
            ? "co.contacts.band.committeeUnreadWhy"
            : "co.contacts.band.noOpenDealWhy",
        )}
      />
    );
  }
  const gaps = coverage.committee.gaps;
  const unlisted = coverage.committee.unlisted_seats;
  return (
    <StatCard
      {...slot}
      value={
        gaps.length > 0
          ? t("co.contacts.band.missing", {
              role: t(ROLE_LABELS[gaps[0] as keyof typeof ROLE_LABELS]),
            })
          : // No gaps means one of two things, and only one is good news: the
            // server empties `gaps` whenever a seat is hidden, because it
            // cannot tell whether the role is held, so reading the empty list
            // as "both roles named" turns a suppressed answer into a sure one.
            t(
              unlisted > 0
                ? "co.contacts.band.committeePartial"
                : "co.contacts.band.committeeComplete",
            )
      }
      detail={
        unlisted > 0
          ? t("co.contacts.band.someHidden", {
              count: formatNumber(unlisted, locale),
            })
          : t("co.contacts.band.seatsHeld", {
              count: formatNumber(coverage.committee.seats.length, locale),
            })
      }
    />
  );
}
