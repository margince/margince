import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { navigate } from "../../app/router";
import { useUrlParams } from "../../app/urlstate";
import {
  Badge,
  Button,
  SegmentedControl,
  StatCard,
} from "../../design-system/atoms";
import { PipelineBoard } from "../../design-system/composed";
import { RelationshipMap } from "../../design-system/relationshipmap";
import { StatStrip } from "../../design-system/statstrip";
import { ProvenanceTag } from "../../design-system/trust";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import {
  isVersionSkewOf,
  problemCodeOf,
  problemMessageOf,
  throwProblem,
} from "../common";
import { EntityRef } from "../entityref";
import { IntroRequestModal, type IntroTarget } from "./introrequest";
import { ASK_INTRO, introTargetFor, mapModelFromCoverage } from "./mapmodel";
import "./companypeople.css";

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

function isCatalogRole(role: string): role is keyof typeof ROLE_LABELS {
  return Object.hasOwn(ROLE_LABELS, role);
}

/**
 * The reader's word for a recorded buying role. A role is written by
 * convention rather than drawn from an enum, so a value the catalog does not
 * name is a fact to show as written, not an error — every record page that
 * prints one reads it here, so the account and the person agree on the word.
 */
export function buyingRoleLabel(
  role: string,
  t: ReturnType<typeof useT>,
): string {
  return isCatalogRole(role) ? t(ROLE_LABELS[role]) : role.replace(/_/g, " ");
}

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
      <CommitteeBoard
        coverage={coverage}
        accountName={accountName}
        orgId={orgId}
      />
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
  orgId,
}: Readonly<{ coverage: Coverage; accountName: string; orgId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const [view, setView] = useState<"board" | "map">("board");
  // The selection lives in the ADDRESS, so the two views agree about who is
  // chosen and a reload or a pasted link restores it. Held locally it survived
  // only until the component unmounted, which made the board and the map two
  // pictures of the same account that could disagree.
  const [params, setParams] = useUrlParams();
  const focusId = params.get("focus") ?? null;
  const setFocusId = (next: string | null) => {
    const out = new Map(params);
    if (next) {
      out.set("focus", next);
    } else {
      out.delete("focus");
    }
    setParams(out);
  };
  const writes = useCommitteeWrites(orgId, coverage.selected_deal_id);
  const renderSeat = (seat: Seat) => (
    <SeatCard
      seat={seat}
      onConfirm={writes.confirm}
      onChange={writes.change}
      confirming={writes.confirming}
    />
  );
  // Who the reader is asking about, or null when the dialog is closed. Held
  // here rather than in the map: the map is data-only by design, and a dialog
  // that fetched would make it a screen.
  const [asking, setAsking] = useState<IntroTarget | null>(null);
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
  // The switcher and the reading sit in one row, so the verb is where the
  // committee is in BOTH views: a button that appears only on the board would
  // make the map a place where the gap can be seen and not filled.
  const toolbar = (
    <div className="cp-board-tools">
      {switcher}
      <SuggestRoles writes={writes} dealId={coverage.selected_deal_id} />
    </div>
  );

  if (view === "map") {
    return (
      <>
        {toolbar}
        {writes.note}
        <RelationshipMap
          model={model}
          focusId={focusId}
          onFocus={setFocusId}
          // What the map DRAWS is the committee, not the account. Comparing
          // seats against every contact read as a truncated contact map —
          // "showing 3 of 100" — when the picture is complete for what it is
          // about. It says the committee's own size, and names the seats it
          // could not list.
          completenessText={
            committee.unlisted_seats > 0
              ? t("co.people.map.scopePartial", {
                  count: formatNumber(committee.seats.length, locale),
                  hidden: formatNumber(committee.unlisted_seats, locale),
                })
              : t("co.people.map.scope", {
                  count: formatNumber(committee.seats.length, locale),
                })
          }
          labels={mapLabels(t, locale)}
          onAction={(nodeId, actionId) => {
            if (actionId === ASK_INTRO) {
              setAsking(introTargetFor(model, nodeId));
            }
          }}
        />
        {/* KEYED ON WHO IS BEING ASKED ABOUT. The dialog holds a draft and the
         * reader's edits, and React reuses a component across prop changes —
         * so without this, drafting for one contact and then opening another
         * showed the FIRST contact's message under the second one's name, and
         * a slow reply could land in a dialog about somebody else. */}
        {asking && (
          <IntroRequestModal
            key={`${asking.personId}:${asking.viaUserId}`}
            orgId={orgId}
            target={asking}
            dealId={coverage.selected_deal_id}
            onClose={() => setAsking(null)}
          />
        )}
      </>
    );
  }

  return (
    <>
      {toolbar}
      {writes.note}
      <PipelineBoard
        variant="plain"
        columns={columns}
        renderCard={(record) => renderSeat(record.seat)}
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
/**
 * mapLabels is the shared word set for the relationship map.
 *
 * Exported because the person page draws the SAME picture and every label but
 * one reads identically there. `region` is the exception — it names what the
 * drawing is OF, and "at this account" is wrong on a person's page — so it is
 * the one a caller passes in.
 */
export function mapLabels(
  t: ReturnType<typeof useT>,
  locale: Locale,
  region = t("co.people.map.region"),
) {
  return {
    region,
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
    routesWithheld: t("co.people.map.routesWithheld"),
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
    assign: t("co.people.map.assignHint"),
    engagement: {
      answered: t("co.reach.answered"),
      no_reply: t("co.reach.silent"),
      untried: t("co.reach.untried"),
    },
    awaitingReply: t("co.people.map.awaiting"),
    theyReplied: t("co.people.map.replied"),
    neverWritten: t("co.people.map.never"),
    onDeal: t("co.people.map.onDeal"),
    askIntro: t("co.map.askIntro"),
  };
}

function SeatCard({
  seat,
  onConfirm,
  onChange,
  confirming,
}: Readonly<{
  seat: Seat;
  onConfirm: (seat: Seat) => void;
  onChange: (seat: Seat) => void;
  confirming: ReadonlySet<string>;
}>) {
  const t = useT();
  // Indigo, dashed, and only here: the treatment means "a machine decided this
  // and nobody has confirmed it". A seat a colleague typed carries none of it,
  // which is the whole point — a reader can see which half of the committee is
  // the product's reading of the evidence and disagree with that half.
  const suggested = seat.ai_suggested === true;
  return (
    <div
      className={suggested ? "cp-seat cp-seat-suggested" : "cp-seat"}
      data-suggested={suggested ? "true" : undefined}
    >
      {suggested && (
        <p className="cp-seat-mark">
          <ProvenanceTag
            provenance={{ kind: "agent", agent: "propose_roles" }}
          />{" "}
          {t("co.people.board.readFromMessages")}
        </p>
      )}
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
      {/* Only an UNCONFIRMED seat offers the two verbs, and only when the
       * caller can address the row. Confirming a seat a colleague typed
       * would be agreeing with them on their behalf; the row is already
       * theirs, and nothing about it is waiting for an answer. */}
      {/* BOTH halves, because the write needs both. Rendering on the id alone
       * gave an enabled button whose click was silently dropped when the
       * version was missing — a control that reports nothing and does
       * nothing, which is worse than one that is not offered. */}
      {suggested && seat.relationship_id && seat.relationship_version && (
        <div className="cp-seat-verbs">
          <Button
            small
            onClick={() => onConfirm(seat)}
            pending={confirming.has(seat.relationship_id)}
            busyLabel={t("co.people.board.confirming")}
          >
            {t("co.people.board.confirm")}
          </Button>
          <Button small variant="ghost" onClick={() => onChange(seat)}>
            {t("co.people.board.change")}
          </Button>
        </div>
      )}
    </div>
  );
}

/**
 * useCommitteeWrites is the three writes this board performs.
 *
 * The reading POSTs to the deal and then invalidates the ACCOUNT's coverage,
 * which is the key this whole band reads under: a seat written to a deal is a
 * change to the committee the account shows, and refreshing the deal alone
 * would leave the board displaying the state before the write.
 *
 * Confirm and Change are the same patch on the same row. The store reassigns
 * `captured_by` to whoever edits, so a human touching the row is what clears
 * the mark — there is no separate "confirmed" column, and there does not need
 * to be: the question the mark asks is "did a person answer this", and an edit
 * IS that answer.
 */
function useCommitteeWrites(orgId: string, dealId: string | null | undefined) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const refresh = () => {
    queryClient.invalidateQueries({
      queryKey: ["organization-coverage", orgId],
    });
  };

  const suggest = useMutation({
    mutationFn: async (dealId: string) => {
      const { data, error } = await api.POST("/deals/{id}/role-proposals", {
        params: { path: { id: dealId } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: refresh,
  });

  // WHICH row is being confirmed, held here rather than read off the mutation.
  // One observer serves every card, and TanStack switches it to the newest
  // call — so a second confirm took the first's spinner away and swallowed its
  // outcome. The set is what lets two cards be busy at once and each keep its
  // own.
  const [confirming, setConfirming] = useState<ReadonlySet<string>>(new Set());
  const patch = useMutation({
    onMutate: (seat: SeatPatch) => {
      setConfirming((busy) => new Set(busy).add(seat.relationshipId));
    },
    onSettled: (_data, _error, seat) => {
      setConfirming((busy) => {
        const next = new Set(busy);
        next.delete(seat.relationshipId);
        return next;
      });
    },
    mutationFn: async (seat: SeatPatch) => {
      const { error } = await api.PATCH("/relationships/{id}", {
        // The conditional write. A colleague may have changed this seat while
        // the page was open, and without the version the confirmation would
        // overwrite them — the losing write leaving no trace, which is the one
        // outcome a concurrent edit must not have.
        params: { path: { id: seat.relationshipId }, ...ifMatch(seat.version) },
        body: { role: seat.role },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: refresh,
  });

  return {
    suggest,
    confirming,
    // Confirming writes the role back UNCHANGED. That looks like a no-op and
    // is not: the store reassigns captured_by to the editing human, so the
    // same role written by a person is what turns the machine's reading into
    // theirs.
    confirm: (seat: Seat) => {
      if (seat.relationship_id && seat.relationship_version !== undefined) {
        patch.mutate({
          relationshipId: seat.relationship_id,
          role: seat.role,
          version: seat.relationship_version,
        });
      }
    },
    // The DEAL's stakeholder panel, which is where a role is actually edited.
    // The contact's own page shows the role as a read-only badge, so sending a
    // reader there was a verb that promised an edit and delivered a label.
    change: () => {
      if (dealId) {
        navigate({ screen: "deals", id: dealId });
      }
    },
    note: <WriteNote suggest={suggest} patch={patch} locale={locale} t={t} />,
  };
}

/**
 * WriteNote says what the last write did, in words rather than a count alone.
 *
 * The two empty answers are DIFFERENT facts and get different sentences.
 * Nothing proposed means their messages do not say who buys; everything
 * refused means the product read something and would not stand behind it. A
 * single "no roles found" would report the second as the first, and a reader
 * would go looking for evidence that was in fact considered and dropped.
 */
function WriteNote({
  suggest,
  patch,
  locale,
  t,
}: Readonly<{
  suggest: ReturnType<
    typeof useMutation<DealRoleProposalResult, Error, string>
  >;
  patch: ReturnType<typeof useMutation<void, Error, SeatPatch>>;
  locale: Locale;
  t: ReturnType<typeof useT>;
}>) {
  // The PATCH's failure first. A suggestion that failed minutes ago would
  // otherwise mask the confirm the reader just pressed, and the older message
  // is the less relevant one exactly when a newer write has gone wrong.
  if (patch.isError) {
    return (
      <p className="t-caption cp-write-note">
        {isVersionSkewOf(patch.error)
          ? t("edit.versionSkew")
          : problemMessageOf(patch.error, t)}
      </p>
    );
  }
  if (suggest.isError) {
    return (
      <p className="t-caption cp-write-note">
        {problemCodeOf(suggest.error) === "not_implemented"
          ? t("co.people.board.suggestUnavailable")
          : problemMessageOf(suggest.error, t)}
      </p>
    );
  }
  const result = suggest.data;
  if (!result) {
    return null;
  }
  if (result.written.length > 0) {
    return (
      <p className="t-caption cp-write-note">
        {t("co.people.board.suggestWrote", {
          count: formatNumber(result.written.length, locale),
        })}
      </p>
    );
  }
  return (
    <p className="t-caption cp-write-note">
      {result.skipped > 0
        ? t("co.people.board.suggestRefused", {
            count: formatNumber(result.skipped, locale),
          })
        : t("co.people.board.suggestNothing")}
    </p>
  );
}

/**
 * SuggestRoles is the verb that reads the committee out of the correspondence.
 *
 * Disabled with a REASON rather than hidden when the account has no open deal:
 * a role is recorded on a deal, and a reader who cannot see why the button is
 * inert learns nothing about what to do instead.
 */
function SuggestRoles({
  writes,
  dealId,
}: Readonly<{
  writes: {
    suggest: ReturnType<
      typeof useMutation<DealRoleProposalResult, Error, string>
    >;
  };
  dealId: string | null | undefined;
}>) {
  const t = useT();
  return (
    <Button
      small
      variant="ai"
      onClick={() => dealId && writes.suggest.mutate(dealId)}
      reason={dealId ? undefined : t("co.people.board.suggestNoDeal")}
      pending={writes.suggest.isPending}
      busyLabel={t("co.people.board.suggesting")}
    >
      <Sparkles aria-hidden="true" />
      {t("co.people.board.suggest")}
    </Button>
  );
}

type DealRoleProposalResult = components["schemas"]["DealRoleProposalResult"];
type SeatPatch = { relationshipId: string; role: string; version: number };
