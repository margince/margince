import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, StatCard } from "../../design-system/atoms";
import { PipelineBoard } from "../../design-system/composed";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import { EntityRef } from "../entityref";

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
  onNarrow,
}: Readonly<{
  orgId: string;
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
  if (query.isPending || query.isError || !query.data?.summary) {
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
      <CommitteeBoard coverage={coverage} />
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
      openLabel={t("co.people.band.showAnswered")}
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
  if (!coverage.committee) {
    return (
      <StatCard
        label={t("co.people.band.committee")}
        value={t("co.people.band.committeeUnread")}
        detail={t("co.people.band.committeeUnreadWhy")}
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
          : t("co.people.band.committeeComplete")
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
function CommitteeBoard({ coverage }: Readonly<{ coverage: Coverage }>) {
  const t = useT();
  const committee = coverage.committee;
  if (!committee) {
    return null;
  }
  const columns = ROLES.map((role) => ({
    stage: role,
    label: t(ROLE_LABELS[role]),
    deals: committee.seats
      .filter((seat) => seat.role === role)
      .map((seat) => ({ id: seat.person_id, name: seat.full_name, seat })),
  }));
  return (
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
  );
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
