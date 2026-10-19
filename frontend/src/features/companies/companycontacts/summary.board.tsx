import { useMemo, useState } from "react";
import { EntityRef } from "../../../components/entityref";
import { Badge, Button, SegmentedControl } from "../../../design-system/atoms";
import { PipelineBoard } from "../../../design-system/pipelineboard";
import { RelationshipMap } from "../../../design-system/relationshipmap";
import { ProvenanceTag } from "../../../design-system/trust";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, useT } from "../../../i18n";
import { useViewerId } from "../../../lib/session";
import { useUrlParams } from "../../../lib/urlstate";
import { ENGAGEMENT_LABELS, ENGAGEMENT_TONES } from "./contacts";
import { IntroRequestModal, type IntroTarget } from "./introrequest";
import { ASK_INTRO, introTargetFor, mapModelFromCoverage } from "./mapmodel";
import type { Coverage, Seat } from "./summary";
import { ROLE_LABELS, ROLES } from "./summary.roles";
import { SuggestRoles, useCommitteeWrites } from "./summary.writes";

/**
 * CommitteeBoard draws the buying team as a structure, holes included.
 *
 * A missing champion is an empty column with the words in it, because the
 * strategy is the shape. A sentence under twenty-five rows cannot make anybody
 * see a gap.
 */
export function CommitteeBoard({
  coverage,
  accountName,
  companyId,
}: Readonly<{ coverage: Coverage; accountName: string; companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const [view, setView] = useState<"board" | "map">("board");
  // The selection lives in the address, so the two views agree about who is
  // chosen and a reload or a pasted link restores it. Local state would die on
  // unmount, and the board and the map could then disagree.
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
  const writes = useCommitteeWrites(companyId, coverage.selected_deal_id);
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
  // Who is reading, so the picture never offers them an introduction from
  // themselves: the endpoint refuses one whose introducer is the caller.
  const viewerId = useViewerId();
  const model = useMemo(
    () => mapModelFromCoverage(coverage, accountName, mapCopy(t), viewerId),
    [coverage, accountName, t, viewerId],
  );
  const committee = coverage.committee;
  if (!committee) {
    return null;
  }
  // `role` is a free string on the wire, so a seat can carry one this board
  // has no column for. It goes in a column of its own rather than being
  // dropped. Otherwise the summary counts a contact the board does not draw.
  const known = new Set<string>(ROLES);
  const other = committee.seats.filter((seat) => !known.has(seat.role));
  const columns = [
    ...ROLES.map((role) => ({
      stage: role,
      label: t(ROLE_LABELS[role]),
      deals: committee.seats
        .filter((seat) => seat.role === role)
        .map((seat) => ({ id: seat.contact_id, name: seat.full_name, seat })),
    })),
    ...(other.length > 0
      ? [
          {
            stage: "other",
            label: t("co.contacts.board.otherRoles"),
            deals: other.map((seat) => ({
              id: seat.contact_id,
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
      label={t("co.contacts.view")}
      labels={{
        board: t("co.contacts.view.board"),
        map: t("co.contacts.view.map"),
      }}
    />
  );
  // The switcher and the reading sit in one row, so the verb is beside the
  // committee in both views. Otherwise the map shows a gap it cannot fill.
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
          // The map draws the committee, not the account. A count against
          // every contact ("showing 3 of 100") reads as a truncated map. So it
          // says the committee's own size, and names the seats it could not list.
          completenessText={
            committee.unlisted_seats > 0
              ? t("co.contacts.map.scopePartial", {
                  count: formatNumber(committee.seats.length, locale),
                  hidden: formatNumber(committee.unlisted_seats, locale),
                })
              : t("co.contacts.map.scope", {
                  count: formatNumber(committee.seats.length, locale),
                })
          }
          labels={mapLabels(t, locale)}
          onAction={(nodeId, actionId) => {
            if (actionId === ASK_INTRO) {
              setAsking(introTargetFor(model, nodeId, viewerId));
            }
          }}
        />
        {/* Keyed on who is being asked about. The dialog holds a draft, and
         * React reuses a component across prop changes. Without the key, a
         * second contact's dialog shows the first one's message, and a slow
         * reply can land in a dialog about somebody else. */}
        {asking && (
          <IntroRequestModal
            key={`${asking.contactId}:${asking.viaUserId}`}
            companyId={companyId}
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
          // A critical role nobody holds says so in the column that would hold
          // them, not in a line underneath the board.
          column.deals.length === 0 && committee.gaps.includes(column.stage) ? (
            <p data-testid={`gap-${column.stage}`}>
              {t("co.contacts.board.nobodyHolds")}
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
 * Exported because the contact page draws the same picture with the same labels
 * but one. `region` names what the drawing is of, and "at this account" is
 * wrong on a contact's page, so a caller passes it in.
 */
export function mapLabels(
  t: ReturnType<typeof useT>,
  locale: Locale,
  region = t("co.contacts.map.region"),
) {
  return {
    region,
    band: {
      strong: t("co.routeIn.band.strong"),
      developing: t("co.routeIn.band.some"),
      cold: t("co.routeIn.band.faint"),
    },
    bestRoute: t("co.contacts.map.bestRoute"),
    alternatives: t("co.contacts.map.alternatives"),
    noRoute: t("co.contacts.map.noRoute"),
    laneMore: (hidden: number) =>
      t("co.contacts.map.more", { count: formatNumber(hidden, locale) }),
    clearFocus: t("co.contacts.map.clear"),
    emptyTitle: t("co.contacts.map.emptyTitle"),
    emptyBody: t("co.contacts.map.emptyBody"),
    nothingSelected: t("co.contacts.map.nothingSelected"),
  };
}

function mapCopy(t: ReturnType<typeof useT>) {
  return {
    routesWithheld: t("co.contacts.map.routesWithheld"),
    ourSide: t("co.contacts.map.ourSide"),
    account: t("co.contacts.map.account"),
    roles: {
      champion: t("co.role.champion"),
      economic_buyer: t("co.role.economic_buyer"),
      influencer: t("co.role.influencer"),
      blocker: t("co.role.blocker"),
      user: t("co.role.user"),
    },
    otherRoles: t("co.contacts.board.otherRoles"),
    missing: (role: string) => t("co.contacts.map.missing", { role }),
    assign: t("co.contacts.map.assignHint"),
    engagement: {
      waiting: t("co.reach.waiting"),
      answered: t("co.reach.answered"),
      no_reply: t("co.reach.silent"),
      untried: t("co.reach.untried"),
    },
    awaitingReply: t("co.contacts.map.awaiting"),
    replyOwed: t("co.contacts.map.owed"),
    theyReplied: t("co.contacts.map.replied"),
    neverWritten: t("co.contacts.map.never"),
    onDeal: t("co.contacts.map.onDeal"),
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
  // `staging-card` is the product's one drawing of "a machine decided this and
  // nobody has confirmed it". A seat a colleague typed carries none of it, so a
  // reader can see which half of the committee is our reading.
  const suggested = seat.ai_suggested === true;
  return (
    <div
      className={suggested ? "cp-seat staging-card" : "cp-seat"}
      data-suggested={suggested ? "true" : undefined}
    >
      {suggested && (
        <p className="cp-seat-mark">
          <ProvenanceTag provenance={{ kind: "agent", job: "propose_roles" }} />{" "}
          {t("co.contacts.board.readFromMessages")}
        </p>
      )}
      <EntityRef kind="contact" id={seat.contact_id} name={seat.full_name} />
      {seat.engagement && (
        <Badge tone={ENGAGEMENT_TONES[seat.engagement]}>
          {t(ENGAGEMENT_LABELS[seat.engagement])}
        </Badge>
      )}
      {/* Only an unconfirmed seat with an addressable row offers the two
       * verbs. Confirming a seat a colleague typed would agree on their behalf. */}
      {/* The write needs both the id and the version. Without the version,
       * the click would do nothing, which is worse than no button. */}
      {suggested && seat.relationship_id && seat.relationship_version && (
        <div className="cp-seat-verbs">
          <Button
            onClick={() => onConfirm(seat)}
            pending={confirming.has(seat.relationship_id)}
            busyLabel={t("co.contacts.board.confirming")}
          >
            {t("co.contacts.board.confirm")}
          </Button>
          <Button variant="ghost" onClick={() => onChange(seat)}>
            {t("co.contacts.board.change")}
          </Button>
        </div>
      )}
    </div>
  );
}
