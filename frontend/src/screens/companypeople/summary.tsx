import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, SegmentedControl, StatCard } from "../../design-system/atoms";
import { PipelineBoard } from "../../design-system/composed";
import { RelationshipMap } from "../../design-system/relationshipmap";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import { EntityRef } from "../entityref";
import { mapModelFromCoverage } from "./mapmodel";

// What the account looks like before the reader picks anybody.
//
// The list below answers "who, in what order". This band answers the three
// questions a rep opens the tab with and would otherwise work out by reading
// the list themselves: is anybody here talking to us, who is the way in, and
// which buying role nobody holds.

type Coverage = components["schemas"]["OrganizationCoverage"];
type Seat = components["schemas"]["OrganizationCoverageSeat"];

/** The roles a committee reads in, and the order it reads them. */
const ROLES = [
  "champion",
  "economic_buyer",
  "influencer",
  "blocker",
  "user",
] as const;

const ROLE_LABELS = {
  champion: "co.role.champion",
  economic_buyer: "co.role.economic_buyer",
  influencer: "co.role.influencer",
  blocker: "co.role.blocker",
  user: "co.role.user",
} as const;

export function useOrganizationCoverage(orgId: string) {
  return useQuery({
    queryKey: ["organization-coverage", orgId],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}/coverage", {
        params: { path: { id: orgId } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
  });
}

export function CoverageBand({
  orgId,
  accountName,
  onNarrow,
}: Readonly<{
  orgId: string;
  accountName: string;
  /** Each statement is a door: it narrows the list below to what it describes. */
  onNarrow: (status: "answered" | "untried" | null) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useOrganizationCoverage(orgId);
  // A band that could not load is absent, not an error banner: the list below
  // it still answers, and a reading nobody can act on is not worth a shout.
  //
  // `summary` is required by the contract, so its absence means the response
  // was not the one this reads — a stub, a proxy, a version skew. Rendering
  // half a band off a payload this does not recognise is worse than rendering
  // none, and reading through it crashes the whole tab.
  if (query.isPending) {
    return null;
  }
  // A read that FAILED says so. Rendering nothing makes a server error look
  // like a band this account does not offer, and the reader then works from a
  // list with no reading above it and no reason to doubt it.
  if (query.isError) {
    return (
      <StatStrip testId="coverage-band">
        <StatCard
          label={t("co.people.band.coverage")}
          value={t("co.people.band.unavailable")}
          detail={t("co.people.band.unavailableWhy")}
        />
      </StatStrip>
    );
  }
  if (!query.data?.summary) {
    return null;
  }
  const coverage = query.data;
  return (
    <>
      <StatStrip testId="coverage-band">
        <WayIn coverage={coverage} onNarrow={onNarrow} />
        <CommitteeReading coverage={coverage} />
        <StatCard
          label={t("co.people.band.coverage")}
          value={t("co.people.band.reachable", {
            count: formatNumber(coverage.summary.answered, locale),
          })}
          detail={t("co.people.band.untried", {
            count: formatNumber(coverage.summary.untried, locale),
          })}
          openLabel={t("co.people.band.showUntried")}
          onOpen={() => onNarrow("untried")}
        />
      </StatStrip>
      <CommitteeBoard coverage={coverage} accountName={accountName} />
    </>
  );
}

/**
 * WayIn names the person worth writing to and why.
 *
 * An account where everyone was written to and nobody answered has no way in,
 * and the card says so rather than naming the least cold contact: dressing a
 * fourth follow-up up as an opening is the one thing this reading must not do.
 */
function WayIn({
  coverage,
  onNarrow,
}: Readonly<{
  coverage: Coverage;
  onNarrow: (status: "answered" | "untried" | null) => void;
}>) {
  const t = useT();
  const way = coverage.best_way_in;
  if (!way) {
    return (
      <StatCard
        label={t("co.people.band.wayIn")}
        value={t("co.people.band.noWayIn")}
        detail={t("co.people.band.noWayInWhy")}
      />
    );
  }
  return (
    <StatCard
      label={t("co.people.band.wayIn")}
      value={way.full_name}
      detail={
        <span>
          {way.title ? `${way.title} · ` : ""}
          {t(
            way.engagement === "answered"
              ? "co.reach.answered"
              : "co.reach.untried",
          )}
        </span>
      }
      // The label names what the press DOES. A way in who has not answered
      // yet narrows to nothing in particular, so the door says "show everyone"
      // rather than promising a list of people who answered and clearing the
      // filter instead.
      openLabel={t(
        way.engagement === "answered"
          ? "co.people.band.showAnswered"
          : "co.people.band.showAll",
      )}
      onOpen={() => onNarrow(way.engagement === "answered" ? "answered" : null)}
    />
  );
}

/**
 * CommitteeReading says which critical role nobody holds.
 *
 * Three readings, not two. A committee that could not be read says so; one
 * read whole with a hole names the hole; one read whole with none says it is
 * complete. Collapsing the first into either of the others is how a page tells
 * a reader a deal has no champion when it has one they cannot see.
 */
function CommitteeReading({ coverage }: Readonly<{ coverage: Coverage }>) {
  const t = useT();
  const { locale } = useLocale();
  // A committee can be absent for two OPPOSITE reasons, and the flag is the
  // only thing that tells them apart: the reader may not look, or there is
  // nothing to look at. Reading absence alone as "hidden from you" accuses the
  // reader's own role on every account that simply has no open deal.
  if (!coverage.committee) {
    const withheld = !coverage.completeness.committee_read;
    return (
      <StatCard
        label={t("co.people.band.committee")}
        value={t(
          withheld
            ? "co.people.band.committeeUnread"
            : "co.people.band.noOpenDeal",
        )}
        detail={t(
          withheld
            ? "co.people.band.committeeUnreadWhy"
            : "co.people.band.noOpenDealWhy",
        )}
      />
    );
  }
  const gaps = coverage.committee.gaps;
  const unlisted = coverage.committee.unlisted_seats;
  return (
    <StatCard
      label={t("co.people.band.committee")}
      value={
        gaps.length > 0
          ? t("co.people.band.missing", {
              role: t(ROLE_LABELS[gaps[0] as keyof typeof ROLE_LABELS]),
            })
          : // No gaps means one of two things, and only one is good news. The
            // server empties `gaps` whenever a seat is hidden, precisely
            // because it cannot tell whether the role is held — so reading the
            // empty list as "both roles named" turns a suppressed answer into
            // a confident one.
            t(
              unlisted > 0
                ? "co.people.band.committeePartial"
                : "co.people.band.committeeComplete",
            )
      }
      detail={
        unlisted > 0
          ? t("co.people.band.someHidden", {
              count: formatNumber(unlisted, locale),
            })
          : t("co.people.band.seatsHeld", {
              count: formatNumber(coverage.committee.seats.length, locale),
            })
      }
    />
  );
}

/**
 * CommitteeBoard draws the buying team as a structure, holes included.
 *
 * A missing champion is an empty column with the words in it, because that is
 * the reading: the strategy is the shape, and a gap is a shape a sentence
 * under twenty-five rows cannot make anybody see.
 */
function CommitteeBoard({
  coverage,
  accountName,
}: Readonly<{ coverage: Coverage; accountName: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const [view, setView] = useState<"board" | "map">("board");
  // The map's own selection. Held here rather than inside the primitive so the
  // board and the map can agree about who is chosen once the board learns to
  // read it.
  const [focusId, setFocusId] = useState<string | null>(null);
  const model = useMemo(
    () => mapModelFromCoverage(coverage, accountName, mapCopy(t)),
    [coverage, accountName, t],
  );
  const committee = coverage.committee;
  if (!committee) {
    return null;
  }
  // `role` is a free string on the wire, so a seat can carry one this board
  // has no column for. It goes in a column of its own rather than being
  // dropped: a person the summary counted and the board does not draw is a
  // reader looking for somebody who is not there.
  const known = new Set<string>(ROLES);
  const other = committee.seats.filter((seat) => !known.has(seat.role));
  const columns = [
    ...ROLES.map((role) => ({
      stage: role,
      label: t(ROLE_LABELS[role]),
      deals: committee.seats
        .filter((seat) => seat.role === role)
        .map((seat) => ({ id: seat.person_id, name: seat.full_name, seat })),
    })),
    ...(other.length > 0
      ? [
          {
            stage: "other",
            label: t("co.people.board.otherRoles"),
            deals: other.map((seat) => ({
              id: seat.person_id,
              name: seat.full_name,
              seat,
            })),
          },
        ]
      : []),
  ];
  const switcher = (
    <SegmentedControl
      options={["board", "map"] as const}
      value={view}
      onChange={setView}
      label={t("co.people.view")}
      labels={{
        board: t("co.people.view.board"),
        map: t("co.people.view.map"),
      }}
    />
  );

  if (view === "map") {
    return (
      <>
        {switcher}
        <RelationshipMap
          model={model}
          focusId={focusId}
          onFocus={setFocusId}
          completenessText={t("co.people.map.scope", {
            shown: formatNumber(committee.seats.length, locale),
            total: formatNumber(coverage.summary.contacts_total, locale),
          })}
          labels={mapLabels(t, locale)}
        />
      </>
    );
  }

  return (
    <>
      {switcher}
      <PipelineBoard
        variant="plain"
        columns={columns}
        renderCard={(record) => <SeatCard seat={record.seat} />}
        columnExtras={(column) =>
          // The gap, where it is: a critical role nobody holds says so in the
          // column that would hold them, not in a line underneath the board.
          column.deals.length === 0 && committee.gaps.includes(column.stage) ? (
            <p className="t-caption" data-testid={`gap-${column.stage}`}>
              {t("co.people.board.nobodyHolds")}
            </p>
          ) : null
        }
      />
    </>
  );
}

/** The map's own words, all of them the caller's to supply. */
function mapLabels(t: ReturnType<typeof useT>, locale: Locale) {
  return {
    region: t("co.people.map.region"),
    band: {
      strong: t("co.routeIn.band.strong"),
      developing: t("co.routeIn.band.some"),
      cold: t("co.routeIn.band.faint"),
    },
    bestRoute: t("co.people.map.bestRoute"),
    alternatives: t("co.people.map.alternatives"),
    noRoute: t("co.people.map.noRoute"),
    laneMore: (hidden: number) =>
      t("co.people.map.more", { count: formatNumber(hidden, locale) }),
    clearFocus: t("co.people.map.clear"),
    emptyTitle: t("co.people.map.emptyTitle"),
    emptyBody: t("co.people.map.emptyBody"),
    nothingSelected: t("co.people.map.nothingSelected"),
  };
}

function mapCopy(t: ReturnType<typeof useT>) {
  return {
    ourSide: t("co.people.map.ourSide"),
    account: t("co.people.map.account"),
    roles: {
      champion: t("co.role.champion"),
      economic_buyer: t("co.role.economic_buyer"),
      influencer: t("co.role.influencer"),
      blocker: t("co.role.blocker"),
      user: t("co.role.user"),
    },
    otherRoles: t("co.people.board.otherRoles"),
    missing: (role: string) => t("co.people.map.missing", { role }),
    assign: t("co.people.map.assign"),
    engagement: {
      answered: t("co.reach.answered"),
      no_reply: t("co.reach.silent"),
      untried: t("co.reach.untried"),
    },
    awaitingReply: t("co.people.map.awaiting"),
    theyReplied: t("co.people.map.replied"),
    neverWritten: t("co.people.map.never"),
    onDeal: t("co.people.map.onDeal"),
  };
}

function SeatCard({ seat }: Readonly<{ seat: Seat }>) {
  const t = useT();
  return (
    <div>
      <EntityRef kind="person" id={seat.person_id} name={seat.full_name} />
      {seat.engagement && (
        <Badge
          tone={
            seat.engagement === "answered"
              ? "success"
              : seat.engagement === "no_reply"
                ? "warn"
                : undefined
          }
        >
          {t(
            seat.engagement === "answered"
              ? "co.reach.answered"
              : seat.engagement === "no_reply"
                ? "co.reach.silent"
                : "co.reach.untried",
          )}
        </Badge>
      )}
    </div>
  );
}
