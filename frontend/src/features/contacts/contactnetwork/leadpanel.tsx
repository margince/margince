// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one move this page recommends, the evidence behind it, and the other
// ways in, as one card.
//
// The move reads by its place at the top, its rank and its one filled button,
// not by a tinted band. Nothing here is machine-authored, so the indigo AI
// treatment never appears.

import type { components } from "../../../api/schema";
import { SeatAvatar } from "../../../components/seatavatar";
import { Avatar, Badge, Button } from "../../../design-system/atoms";
import { Eyebrow } from "../../../design-system/eyebrow";
import { Panel, PanelBody } from "../../../design-system/panel";
import { relayMemberFor } from "../../../design-system/provenanceof";
import { StrengthMeter } from "../../../design-system/strengthmeter";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../../i18n";
import { labelFor } from "../../../lib/labelfor";
import { useMe, useViewerId } from "../../../lib/session";
import { BAND_BADGE } from "../companygraph";
import { lastContactBucket, RouteList, useOwnRoute } from "../contactroutes";
import { ReceiptList } from "./receipts";

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];
type RouteEvidence = components["schemas"]["ContactGraphRouteEvidence"];
type Translate = ReturnType<typeof useT>;

/** The job titles the graph knows for each name on the chain. */
export type ChainRoles = Readonly<{
  via?: string;
  through?: string;
  target?: string;
}>;

/**
 * LeadPanel names the recommended route, offers the ask, and ranks the rest.
 *
 * The head of the server's ranked list, not a second opinion about it. This
 * card draws `routes[0]` and lists the others under it. So the recommendation
 * and its alternatives cannot disagree about the order.
 */
export function LeadPanel({
  route,
  alternatives = [],
  roles = {},
  targetName,
  targetId,
  blocked,
  onAsk,
  onOpenEmail,
}: Readonly<{
  route: RouteCandidate;
  // The routes ranked after this one, in the server's order.
  alternatives?: readonly RouteCandidate[];
  roles?: ChainRoles;
  targetName: string;
  // The contact this page is about, which keys their chip.
  targetId: string;
  // Opens one of the cited messages in the contact page's email drawer.
  onOpenEmail?: (activityId: string) => void;
  // Why the ask cannot be made, when it cannot. A card that offered a button
  // answering 409 would be a control that exists to fail.
  blocked: string | null;
  onAsk: (route: RouteCandidate) => void;
}>) {
  const t = useT();
  // The reader is one of the colleagues the server ranks. The warmest way in
  // is often their own relationship, and "ask X" is then false.
  const mine = useOwnRoute()(route);
  return (
    <Panel className="pn-way">
      <PanelBody>
        <div className="pn-lead">
          <div className="pn-lead-head">
            <span className="pn-route-rank pn-rank-best" aria-hidden="true">
              1
            </span>
            <Eyebrow>{t("contact.intro.bestWayIn")}</Eyebrow>
            {/* The route's own band, in the map's words. The lead is the
                best-ranked route, which need not be a strong one. */}
            <Badge
              tone={
                blocked || route.strength_bucket !== "strong"
                  ? undefined
                  : "success"
              }
            >
              {blocked ??
                t(labelFor(BAND_BADGE, route.strength_bucket ?? "none"))}
            </Badge>
          </div>

          <Chain
            route={route}
            mine={mine}
            roles={roles}
            targetName={targetName}
            targetId={targetId}
          />

          <Figures
            route={route}
            targetName={targetName}
            mine={mine}
            onOpenEmail={onOpenEmail}
          />

          {/* The reader's own route gets a sentence instead of the button: the
              server refuses an ask whose introducer is the one asking. */}
          {blocked ? null : mine ? (
            <p className="pn-counts t-sub">
              {t("contact.intro.ownRouteNoAsk")}
            </p>
          ) : (
            <p className="pn-lead-foot">
              <Button variant="primary" onClick={() => onAsk(route)}>
                {t("contact.intro.askFirstName", {
                  name: route.via_display_name,
                })}
              </Button>
            </p>
          )}
        </div>
      </PanelBody>
      {alternatives.length > 0 ? (
        <PanelBody>
          <RouteList routes={alternatives} firstRank={2} onAsk={onAsk} />
        </PanelBody>
      ) : null}
    </Panel>
  );
}

/**
 * Chain draws who asks whom: the reader, the colleague, a contact in between
 * when there is one, then the contact this page is about.
 *
 * The tie to the contact carries the route's band twice: as the line's weight
 * and as a meter. So the strength is in words as well as in ink.
 */
function Chain({
  route,
  mine,
  roles,
  targetName,
  targetId,
}: Readonly<{
  route: RouteCandidate;
  mine: boolean;
  roles: ChainRoles;
  targetName: string;
  targetId: string;
}>) {
  const t = useT();
  const me = useMe();
  const viewerId = useViewerId();
  const band = route.strength_bucket ?? "none";
  const through = route.through_display_name;
  const tie = (
    <li className="pn-chain-link" data-strength={band}>
      <span className="pn-chain-link-line" />
      <StrengthMeter band={band} label={t(`contact.band.${band}`)} />
      <span className="pn-chain-link-words">{heroLine(mine, t)}</span>
    </li>
  );
  return (
    <ol className="pn-chain">
      {mine ? null : (
        <>
          <li className="pn-chain-node">
            <SeatAvatar
              id={viewerId}
              name={me.data?.user.display_name ?? t("contact.intro.heroYou")}
              size="md"
            />
            <b>{t("contact.intro.heroYou")}</b>
          </li>
          <li className="pn-chain-link" data-strength="team">
            <span className="pn-chain-link-line" />
            <span className="pn-chain-link-words">
              {t("contact.intro.sameTeam")}
            </span>
          </li>
        </>
      )}
      <li className="pn-chain-node">
        <SeatAvatar
          id={route.via_user_id}
          name={route.via_display_name}
          size="md"
        />
        <b>{mine ? t("contact.intro.heroYou") : route.via_display_name}</b>
        {roles.via ? <span className="t-caption">{roles.via}</span> : null}
      </li>
      {tie}
      {through ? (
        <>
          <li className="pn-chain-node">
            <Avatar
              name={through}
              identity={route.through_contact_id ?? through}
              size="md"
            />
            <b>{through}</b>
            {roles.through ? (
              <span className="t-caption">{roles.through}</span>
            ) : null}
          </li>
          <li className="pn-chain-link" data-strength="through">
            <span className="pn-chain-link-line" />
          </li>
        </>
      ) : null}
      <li className="pn-chain-node pn-chain-node-target">
        <Avatar name={targetName} identity={targetId} size="md" />
        <b>{targetName}</b>
        {roles.target ? (
          <span className="t-caption">{roles.target}</span>
        ) : null}
      </li>
    </ol>
  );
}

// The words on the tie, which continue the name before it. On a route through a
// contact the tie runs to that contact's node, so it is direct there too.
function heroLine(mine: boolean, t: Translate): string {
  return mine
    ? t("contact.intro.heroDirectYou")
    : t("contact.intro.heroDirect");
}

/**
 * Figures is what the recommendation was read from: how much traffic, how
 * recent, and the messages behind it.
 *
 * Receipts come only on a direct route: pooled counts are disclosable where the
 * correspondence itself is not, so a route through a colleague says so.
 */
function Figures({
  route,
  targetName,
  mine,
  onOpenEmail,
}: Readonly<{
  route: RouteCandidate;
  targetName: string;
  mine: boolean;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const ev = route.evidence;
  const receipts = route.receipts ?? [];
  const reach = route.through_display_name
    ? t("contact.intro.factIndirect")
    : t("contact.intro.factDirect");
  return (
    <dl className="pn-figures">
      <div>
        <dt className="t-caption">{t("contact.intro.exchangesWindow")}</dt>
        <dd className="pn-figure">
          <b>{formatNumber(ev.interactions_90d, locale)}</b>
          <ExchangeSplit
            evidence={ev}
            viaName={route.via_display_name}
            targetName={targetName}
            mine={mine}
          />
        </dd>
      </div>
      <div>
        <dt className="t-caption">{t("contact.intro.evidenceLastContact")}</dt>
        <dd className="pn-figure">
          <b>{lastContactReading(ev, t, locale)}</b>
        </dd>
        <dd className="pn-figure-note">
          {/* One-sided is the fact a reader most needs before asking, so it
              keeps its warning; reciprocal reads as plain words. */}
          {ev.two_way ? (
            <span className="t-caption">
              {t("contact.intro.factReciprocal")} · {reach}
            </span>
          ) : (
            <>
              <Badge tone="warning">{t("contact.intro.factOneSided")}</Badge>
              <span className="t-caption">{reach}</span>
            </>
          )}
        </dd>
      </div>
      <div className="pn-figure-receipts">
        {receipts.length > 0 ? (
          <>
            <dt className="t-caption">
              {plural("contact.intro.factReceipts", receipts.length, {
                count: formatNumber(receipts.length, locale),
              })}
            </dt>
            <dd>
              <ReceiptList
                receipts={receipts}
                relayMember={relayMemberFor(route.via_user_id, viewerId)}
                onOpenEmail={onOpenEmail}
              />
            </dd>
          </>
        ) : (
          <dd className="t-caption">{t("contact.graph.countsOnly")}</dd>
        )}
      </div>
    </dl>
  );
}

/**
 * ExchangeSplit shows how much of the traffic came from each end.
 *
 * Drawn only when the server split the count: a bar guessed from a total
 * alone would show a share nothing measured. The figures are in words under
 * it, so the bar is never the only carrier.
 */
function ExchangeSplit({
  evidence,
  viaName,
  targetName,
  mine,
}: Readonly<{
  evidence: RouteEvidence;
  viaName: string;
  targetName: string;
  mine: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const inbound = evidence.inbound_90d;
  const outbound = evidence.outbound_90d;
  if (inbound === undefined || outbound === undefined) {
    return null;
  }
  const total = Math.max(inbound + outbound, 1);
  return (
    <span className="pn-split-wrap">
      <span className="pn-split" aria-hidden="true">
        <i className="pn-split-in" style={{ width: pct(inbound, total) }} />
        <i className="pn-split-out" style={{ width: pct(outbound, total) }} />
      </span>
      <span className="pn-split-legend t-caption">
        <span className="pn-split-key-in">
          {t("contact.intro.evidenceFrom", {
            count: formatNumber(inbound, locale),
            name: targetName,
          })}
        </span>
        <span className="pn-split-key-out">
          {mine
            ? t("contact.intro.evidenceFromYou", {
                count: formatNumber(outbound, locale),
              })
            : t("contact.intro.evidenceFrom", {
                count: formatNumber(outbound, locale),
                name: viaName,
              })}
        </span>
      </span>
    </span>
  );
}

function pct(part: number, total: number): string {
  return `${(part / total) * 100}%`;
}

// The figure-sized spelling of the fact the route rows put in a sentence.
// Both read `lastContactBucket`, so the two cannot disagree about yesterday.
function lastContactReading(
  ev: RouteEvidence,
  t: Translate,
  locale: Locale,
): string {
  const last = lastContactBucket(ev);
  switch (last.kind) {
    case "never":
      return t("contact.intro.lastNever");
    case "today":
      return t("contact.intro.lastToday");
    case "yesterday":
      return t("contact.intro.lastYesterday");
    case "days":
      return t("contact.intro.lastDays", {
        days: formatNumber(last.days, locale),
      });
  }
}
