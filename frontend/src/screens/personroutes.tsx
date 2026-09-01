// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every way in to this contact, best first.
//
// The server ranks; this file renders. It states each route's evidence in the
// reader's own language rather than printing the server's `why`, which is
// English whatever locale the app is in — the counts cross the wire as facts
// for exactly that reason.

import type { components } from "../api/schema";
import { Badge, Button, Card } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import "./personnetwork.css";

type Graph = components["schemas"]["PersonGraph"];
type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];
type RouteEvidence = components["schemas"]["PersonGraphRouteEvidence"];
type Translate = ReturnType<typeof useT>;
type Pluralize = ReturnType<typeof usePlural>;

/**
 * RoutesCard lists the ways in, the recommendation first.
 *
 * One list, not a lead card plus a list: the server's recommendation IS the
 * head of this list, so drawing it twice would invite the two to disagree on
 * screen the moment one of them was rendered from stale data.
 */
export function RoutesCard({
  graph,
  onAsk,
  skipLead,
}: Readonly<{
  graph: Graph;
  onAsk?: (route: RouteCandidate) => void;
  // Drop the head of the list, for a caller that already drew it as the lead.
  // Showing it twice would ask the reader which of the two is the
  // recommendation.
  skipLead?: boolean;
}>) {
  const t = useT();
  const all = graph.routes ?? [];
  const routes = skipLead ? all.slice(1) : all;
  // `routes` is optional in the contract and `route` is not, so a response from
  // a server that predates the list still carries the recommendation. Reading
  // only the list would render that payload as "nobody can reach them" — the
  // one sentence on this card a reader must never see wrongly.
  const legacy = all.length === 0 ? graph.route : undefined;
  return (
    <Card
      title={t("person.intro.routesTitle")}
      sub={t("person.intro.routesSub")}
    >
      {routes.length === 0 && !legacy ? (
        <p className="pn-route">{t("person.graph.noRoute")}</p>
      ) : (
        <ol className="pn-routes">
          {legacy ? (
            <li className="pn-route-row">
              <LegacyRouteRow route={legacy} />
            </li>
          ) : (
            routes.map((route, index) => (
              <li className="pn-route-row" key={route.route_id}>
                <RouteRow route={route} lead={index === 0} onAsk={onAsk} />
              </li>
            ))
          )}
        </ol>
      )}
    </Card>
  );
}

/**
 * LegacyRouteRow renders the singular `route` from a server that does not send
 * the list yet.
 *
 * It prints the server's own English `why`, because that payload carries no
 * structured counts to write a translated sentence from. Stating the sentence
 * the server sent is honest; inventing one from fields that are not there is
 * not.
 */
function LegacyRouteRow({
  route,
}: Readonly<{ route: NonNullable<Graph["route"]> }>) {
  const t = useT();
  return (
    <>
      <p className="pn-route">
        {route.through_display_name
          ? t("person.graph.routeVia", {
              name: route.via_display_name,
              through: route.through_display_name,
            })
          : t("person.graph.routeDirect", {
              name: route.via_display_name,
            })}
      </p>
      <p className="pn-counts">{route.why}</p>
    </>
  );
}

function RouteRow({
  route,
  lead,
  onAsk,
}: Readonly<{
  route: RouteCandidate;
  lead: boolean;
  onAsk?: (route: RouteCandidate) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const blocked = availabilityLabel(route.availability, t);
  return (
    <>
      <p className="pn-route">
        <RouteLine route={route} />{" "}
        <Badge tone={lead ? "accent" : undefined} quiet={!lead}>
          {lead ? t("person.intro.best") : t("person.intro.alternative")}
        </Badge>
        {/* A route that cannot be used says so beside itself. Rendering it
            identically to an open one would send a rep to ask a colleague who
            has already declined. */}
        {blocked ? <Badge quiet>{blocked}</Badge> : null}
      </p>
      <p className="pn-counts">
        {evidenceSentence(route.evidence, t, plural, locale)}
      </p>
      {/* A route that cannot be asked for offers no button. Rendering one that
          answers 409 would be a control that exists to fail. */}
      {onAsk && route.availability === "available" ? (
        <Button
          variant={lead ? undefined : "ghost"}
          onClick={() => onAsk(route)}
        >
          {t("person.intro.askAction")}
        </Button>
      ) : null}
    </>
  );
}

/**
 * RouteLine names the colleague and how they reach the contact.
 *
 * One sentence, exported, because the routes list and the ask drawer both say
 * it. A second wording for one fact is how two surfaces start disagreeing about
 * what a route is — and the drawer is where the reader confirms the route they
 * picked on the list, so the two must read alike.
 */
export function RouteLine({ route }: Readonly<{ route: RouteCandidate }>) {
  const t = useT();
  return (
    <>
      {route.through_display_name
        ? t("person.graph.routeVia", {
            name: route.via_display_name,
            through: route.through_display_name,
          })
        : t("person.graph.routeDirect", { name: route.via_display_name })}
    </>
  );
}

/**
 * evidenceSentence writes the proof line the reader acts on.
 *
 * Two-way and one-sided are different claims, and the difference is the whole
 * point: thirty unanswered sends are not a relationship, and a line that
 * printed one number for both would say they were.
 */
export function evidenceSentence(
  ev: RouteEvidence,
  t: Translate,
  plural: Pluralize,
  locale: Locale,
): string {
  const when = lastContactPhrase(ev, t, locale);
  const base = ev.two_way
    ? "person.intro.evidenceTwoWay"
    : "person.intro.evidenceOneSided";
  // Through the plural translator, not a count comparison: "1 interactions"
  // undermines the very claim the line is making, and which counts are
  // singular is a fact about the reader's language rather than about one.
  return plural(base, ev.interactions_90d, {
    total: formatNumber(ev.interactions_90d, locale),
    when,
  });
}

// The server counts the days, so the client never re-derives today from a
// timestamp — two clocks disagreeing is how "yesterday" becomes "2 days ago"
// for a reader in another timezone.
function lastContactPhrase(
  ev: RouteEvidence,
  t: Translate,
  locale: Locale,
): string {
  const days = ev.days_since_last;
  if (days === null || days === undefined) return t("person.intro.whenNever");
  if (days === 0) return t("person.intro.whenToday");
  if (days === 1) return t("person.intro.whenYesterday");
  return t("person.intro.whenDays", { days: formatNumber(days, locale) });
}

/**
 * availabilityLabel names why a route cannot be asked for, or null when it can.
 *
 * An available route needs no label; the other three each say something the
 * reader would otherwise learn by asking and being turned down.
 *
 * The lead panel and this list draw the SAME route, so both take their wording
 * here: one state must not have two spellings, or the reader has to decide
 * which card to believe.
 */
export function availabilityLabel(
  availability: RouteCandidate["availability"],
  t: Translate,
): string | null {
  switch (availability) {
    case "already_requested":
      return t("person.intro.alreadyRequested");
    case "declined":
      return t("person.intro.declined");
    case "unavailable":
      return t("person.intro.unavailable");
    default:
      return null;
  }
}
