// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The contact's network, ordered as the decision a rep is making.
//
// Answer first, evidence under it: the three readings, then the one move this
// page recommends, then the alternatives, then where the handoff stands, then
// the picture, then what moved lately. A reader who trusts the first two
// sections never scrolls; one who does not can check every claim.
//
// The ego ring this replaced drew structure without showing the handoff, and
// its labels sat outside the drawing. The shared RelationshipMap draws the same
// routes in the language the account page already uses.

import { useMemo, useState } from "react";

import type { components } from "../../api/schema";
import { RelationshipMap } from "../../design-system/relationshipmap";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf } from "../common";
import { mapLabels } from "../companypeople/summary";
import { IntroAsksCard } from "../introasks";
import { IntroDrawer } from "../introdrawer";
import { type IntroRequest, useIntroRequests } from "../introrequests";
import { usePersonGraph } from "../persongraph";
import { availabilityLabel, RoutesCard } from "../personroutes";
import { DecisionStrip } from "./decision";
import { EdgeDetail } from "./edgedetail";
import { LeadPanel } from "./leadpanel";
import { completenessText, mapModelFromPersonGraph } from "./mapmodel";
import { MomentsCard, momentWhyNow } from "./moments";
import { RelayPanel } from "./relay";
import "../personnetwork.css";

type RelationshipMoments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;
type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

/**
 * PersonNetworkTab answers "who reaches this contact, what should I do, and
 * where did the last attempt get to".
 *
 * `view` is optional because the moments come from the 360 and the older
 * contacts screen holds none. Without it the tab is everything else, one card
 * shorter — never a card claiming nothing moved.
 */
export function PersonNetworkTab({
  personId,
  view,
}: Readonly<{ personId: string; view?: RelationshipMoments }>) {
  const t = useT();
  const { locale } = useLocale();
  const graph = usePersonGraph(personId);
  const asks = useIntroRequests(personId);
  const [focus, setFocus] = useState<string | null>(null);
  const [asking, setAsking] = useState<RouteCandidate | undefined>();

  const copy = useMemo(
    () => ({
      ourTeam: t("person.intro.laneOurs"),
      theirCompany: t("person.intro.laneTheirs"),
      peers: t("person.intro.lanePeers"),
      target: t("person.intro.laneTarget"),
      useThisRoute: t("person.intro.useThisRoute"),
      withheldDirect: t("person.graph.withheldDirect"),
      withheldAccount: t("person.graph.withheldAccount"),
      edgeDirect: (name: string) => t("person.intro.edgeDirect", { name }),
      edgeAccount: (name: string) => t("person.intro.edgeAccount", { name }),
    }),
    [t],
  );

  if (graph.isPending) {
    return (
      <SurfaceState
        state="loading"
        emptyLabel={t("person.graph.noDirect")}
        loadingLabel={t("person.graph.loading")}
        loadingLines={3}
      >
        {null}
      </SurfaceState>
    );
  }
  if (graph.isError) {
    // Not SurfaceState's `failed`: WHICH failure this was is what a reader
    // acts on — a refusal is answered by asking for the grant, a timeout by
    // retrying. problemMessageOf keeps the internal cause off the screen.
    return (
      <p role="alert" className="pn-failed">
        {problemMessageOf(graph.error, t)}
      </p>
    );
  }
  const data = graph.data;
  if (!data?.nodes) {
    return null;
  }

  const read = readGraph(data, asks.data ?? [], focus);

  const model = mapModelFromPersonGraph(data, copy);
  const complete = completenessText(data, copy, (n) =>
    t("person.graph.droppedNote", { count: formatNumber(n, locale) }),
  );

  return (
    <div className="pn-stack">
      <DecisionStrip
        lead={read.lead}
        legacyVia={read.legacy?.via_display_name}
        whyNow={momentWhyNow(view, t)}
        open={read.open}
      />

      <div className="pn-work-grid">
        <div className="pn-main-column">
          {/* The lead, or nothing. With no way in the strip above has already
              said so as this page's Best Path — its whole job is to answer
              first — and a paragraph here repeating the same sentence made the
              page say it twice in two different weights, which reads as two
              findings rather than one. */}
          {read.lead ? (
            <LeadPanel
              route={read.lead}
              targetName={read.targetName}
              blocked={availabilityLabel(read.lead.availability, t)}
              onAsk={setAsking}
            />
          ) : null}

          {/* The alternatives, and only those: the lead is drawn above, so
              listing it again would ask the reader which of the two identical
              lines is the recommendation. One route means no alternatives, and
              a card headed "other ways in" with nothing under it is worse than
              no card. */}
          {read.alternatives ? (
            <RoutesCard
              graph={data}
              onAsk={setAsking}
              skipLead={read.skipLead}
            />
          ) : null}

          {/* A group withheld for lack of a grant says so in the product's
              ONE spelling of that fact, rather than reading as a company
              nobody here knows. The map's completeness line repeats it for a
              reader who scrolled past this. */}
          {read.withheld ? (
            <SurfaceState
              state="withheld"
              emptyLabel={t("person.graph.noDirect")}
            >
              {null}
            </SurfaceState>
          ) : null}
        </div>

        <aside className="pn-side-column">
          <RelayPanel ask={read.open} />
          <IntroAsksCard personId={personId} personName={read.targetName} />
          {view && <MomentsCard view={view} />}
        </aside>
      </div>

      {/* Full width, below the grid. The drawing is 744px wide and scrolls
          rather than shrinking, so inside a half-width column it showed its
          left third and nothing else — one lane, and eleven nodes painted off
          the visible canvas. */}
      <RelationshipMap
        model={model}
        focusId={read.focused}
        onFocus={setFocus}
        onAction={(nodeId) => {
          const picked = read.routes.find((r) => r.via_user_id === nodeId);
          if (picked) {
            setAsking(picked);
          }
        }}
        completenessText={complete}
        // The account page's word set, with the one label that names the
        // subject swapped: this drawing is about a person, not an account.
        labels={mapLabels(t, locale, t("person.intro.mapRegion"))}
        // The selected person's own detail, including the one write this
        // picture offers: recording an observed acquaintance the graph
        // spotted but nothing has written down yet.
        panelSlot={
          read.focused && read.anchor ? (
            <EdgeDetail
              graph={data}
              nodeId={read.focused}
              anchorId={read.anchor.id}
            />
          ) : null
        }
      />

      {asking ? (
        <IntroDrawer
          personId={personId}
          personName={read.targetName}
          route={asking}
          open
          onClose={() => setAsking(undefined)}
        />
      ) : null}
    </div>
  );
}

/**
 * readGraph is the page's whole reading of one payload, in one place.
 *
 * Pulled out of the component because assembling six sections and deriving
 * seven values from two reads is two jobs, and only the second is worth
 * testing on its own.
 */
function readGraph(
  data: NonNullable<ReturnType<typeof usePersonGraph>["data"]>,
  asks: readonly IntroRequest[],
  focus: string | null,
) {
  const nodes = data.nodes ?? [];
  const routes = data.routes ?? [];
  return {
    anchor: nodes.find((n) => n.group === "anchor"),
    targetName: nodes.find((n) => n.group === "anchor")?.label ?? "",
    routes,
    lead: routes[0],
    // The ask in flight. Only one route per colleague can be open at a time,
    // and the newest is the one this page is about.
    open: asks.find((a) => OPEN.has(a.status)),
    // The card shows what the lead panel does NOT: the other routes when there
    // are some, and the legacy singular `route` when a server that predates the
    // candidate list is answering — which the panel cannot draw at all.
    //
    // An empty `routes` is NOT the legacy case on its own. A current server
    // answering "nobody reaches this contact" sends an empty list and no
    // `route`, and drawing the card then put a heading reading "Best first,
    // pick the one you can actually use" above nothing — beside the paragraph
    // that had already said nobody reaches them, so the page said it twice and
    // offered a choice of none. The legacy payload is the one that has a
    // singular `route` to draw.
    alternatives: routes.length > 1 || (routes.length === 0 && !!data.route),
    skipLead: routes.length > 1,
    legacy: routes.length === 0 ? data.route : undefined,
    withheld: (data.groups_omitted ?? []).length > 0,
    // A selection only means something against the graph ON SCREEN. This tab
    // stays mounted as a reader moves between contacts, so a raw id would open
    // the detail on a node this graph does not have — describing a record the
    // reader has already left. Validating beats resetting on personId: it also
    // covers a graph that reloads without the node for any other reason.
    focused: nodes.some((n) => n.id === focus) ? focus : null,
  };
}

const OPEN = new Set(["requested", "accepted", "name_drop_approved"]);
