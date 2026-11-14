// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The contact's network, ordered as the decision a rep is making.
//
// Answer first, evidence under it: the one move this page recommends with
// the counts behind it, then the three readings, then the alternatives, then
// where the handoff stands, then the picture, then what moved lately. A
// reader who trusts the first section never scrolls; one who does not can
// check every claim.
//
// The ego ring this replaced drew structure without showing the handoff, and
// its labels sat outside the drawing. The shared RelationshipMap draws the same
// routes in the language the account page already uses.

import { useId, useMemo, useState } from "react";

import type { components } from "../../../api/schema";
import { Chapter, ChapterJump, Chapters } from "../../../design-system/chapter";
import { ErrorLine } from "../../../design-system/errorline";
import { Panel, PanelBody } from "../../../design-system/panel";
import { RelationshipMap } from "../../../design-system/relationshipmap";
import { SurfaceState } from "../../../design-system/surfacestate";
import { formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useViewerId } from "../../../lib/session";
import { mapLabels } from "../../companies/companycontacts/summary";
import { useContactGraph } from "../contactgraph";
import { availabilityLabel, RoutesPanel, useOwnRoute } from "../contactroutes";
import { IntroAsksPanel } from "../introasks";
import { IntroDrawer } from "../introdrawer";
import { type IntroRequest, useIntroRequests } from "../introrequests";
import { RecordIntroductionButton } from "../recordintroduction";
import { CaptureGapsNotice } from "./capturegaps";
import { useNetworkReadings } from "./decision";
import { EdgeDetail } from "./edgedetail";
import { LeadPanel } from "./leadpanel";
import {
  completenessText,
  mapModelFromContactGraph,
  WRITE_EMAIL_ACTION,
} from "./mapmodel";
import { latestChange, MomentsPanel } from "./moments";
import { RelayPanel } from "./relay";
import "../contactnetwork.css";

type RelationshipMoments = Pick<
  components["schemas"]["Contact360"],
  "relationship_changes" | "sections_omitted"
>;
type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];

/**
 * ContactNetworkTab answers "who reaches this contact, what should I do, and
 * where did the last attempt get to".
 *
 * `view` is optional because the moments come from the 360 and the older
 * contacts screen holds none. Without it the tab is everything else, one card
 * shorter — never a card claiming nothing moved.
 */
export function ContactNetworkTab({
  contactId,
  view,
  onOpenEmail,
  onWrite,
}: Readonly<{
  contactId: string;
  view?: RelationshipMoments;
  // Opens the page's composer to this contact. The reader's own node on the
  // map offers it, because their way in is to write.
  onWrite?: () => void;
  // Opens a cited message in the contact page's own email drawer. The graph
  // names the messages a count was read from, and the reader checks the count
  // by reading one.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const graph = useContactGraph(contactId);

  if (graph.isPending) {
    return (
      <SurfaceState
        state="loading"
        emptyLabel={t("contact.graph.noDirect")}
        loadingLabel={t("contact.graph.loading")}
        loadingLines={3}
      >
        {null}
      </SurfaceState>
    );
  }
  if (graph.isError) {
    // Not SurfaceState's `failed`: WHICH failure this was is what a reader
    // acts on — a refusal is answered by asking for the grant, a timeout by
    // retrying. ErrorLine keeps the internal cause off the screen.
    return <ErrorLine error={graph.error} />;
  }
  const data = graph.data;
  if (!data?.nodes) {
    return null;
  }
  return (
    <NetworkChapters
      contactId={contactId}
      data={data}
      view={view}
      onOpenEmail={onOpenEmail}
      onWrite={onWrite}
    />
  );
}

type Graph = NonNullable<ReturnType<typeof useContactGraph>["data"]>;

// The tab's three chapters over one read graph: the way in, the people behind
// it, and where the introduction and the relationship stand.
function NetworkChapters({
  contactId,
  data,
  view,
  onOpenEmail,
  onWrite,
}: Readonly<{
  contactId: string;
  data: Graph;
  view?: RelationshipMoments;
  onOpenEmail?: (activityId: string) => void;
  onWrite?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const own = useOwnRoute();
  const viewerId = useViewerId();
  const asks = useIntroRequests(contactId);
  const [focus, setFocus] = useState<string | null>(null);
  const [asking, setAsking] = useState<RouteCandidate | undefined>();
  const baseId = useId();
  const ids = {
    wayIn: `${baseId}-way`,
    who: `${baseId}-who`,
    progress: `${baseId}-progress`,
  };
  const copy = useMemo(
    () => ({
      ourTeam: t("contact.intro.laneOurs"),
      theirCompany: t("contact.intro.laneTheirs"),
      peers: t("contact.intro.lanePeers"),
      target: t("contact.intro.laneTarget"),
      useThisRoute: t("contact.intro.useThisRoute"),
      you: t("contact.intro.heroYou"),
      writeEmail: t("compose.writeEmail"),
      withheldDirect: t("contact.graph.withheldDirect"),
      withheldAccount: t("contact.graph.withheldAccount"),
      edgeDirect: (name: string) => t("contact.intro.edgeDirect", { name }),
      edgeDirectYou: t("contact.intro.edgeDirectYou"),
      edgeAccount: (name: string) => t("contact.intro.edgeAccount", { name }),
    }),
    [t],
  );

  const read = readGraph(data, asks.data ?? [], focus);
  // The newest change and whether the section was refused: a refused section
  // reading as "nothing new" is a claim made out of a permission.
  const moved = latestChange(view);
  const readings = useNetworkReadings({
    routes: read.routes,
    routesWithheld: (data.groups_omitted ?? []).includes("direct"),
    legacyVia: read.legacy?.via_display_name,
    change: moved.change,
    changeWithheld: moved.withheld,
    open: read.open,
    openPending: asks.isPending,
    openUnread: asks.isError,
  });

  const model = mapModelFromContactGraph(data, copy, {
    userId: viewerId,
    mayWrite: onWrite !== undefined,
  });
  const complete = completenessText(data, copy, (n) =>
    t("contact.graph.droppedNote", { count: formatNumber(n, locale) }),
  );

  const wayIn =
    read.lead || read.hasMain ? (
      <>
        {/* The lead, or nothing. With no way in the chapter's answer says so,
            and a panel repeating it would make the page say it twice. */}
        {read.lead ? (
          <LeadPanel
            route={read.lead}
            targetName={read.targetName}
            targetId={contactId}
            blocked={availabilityLabel(read.lead.availability, t)}
            onAsk={setAsking}
            onOpenEmail={onOpenEmail}
          />
        ) : null}
        {/* The alternatives, and only those: the lead is drawn above, and one
            route means no alternatives. */}
        {read.alternatives ? (
          <RoutesPanel
            graph={data}
            onAsk={setAsking}
            skipLead={read.skipLead}
          />
        ) : null}
        {/* A group withheld for lack of a grant says so in the product's one
            spelling of that fact, rather than reading as nobody knowing them. */}
        {read.withheld ? (
          <SurfaceState
            loadingLabel={t("tab.network")}
            state="withheld"
            emptyLabel={t("contact.graph.noDirect")}
          >
            {null}
          </SurfaceState>
        ) : null}
      </>
    ) : null;
  const whoTitle = t("contact.intro.mapTitle", { name: read.targetName });

  return (
    // The tab is a size container: the rail and the sidebar change its width
    // without changing the viewport, so its folds are container queries.
    <div className="pn-stack">
      <Chapters>
        <ChapterJump
          label={t("contact.chapter.jump")}
          chapters={[
            {
              id: ids.wayIn,
              title: t("contact.chapter.wayIn"),
              reading: readings.wayIn.chip,
            },
            { id: ids.who, title: whoTitle },
            {
              id: ids.progress,
              title: t("contact.chapter.progress"),
              reading: readings.progress.chip,
            },
          ]}
        />

        <Chapter
          id={ids.wayIn}
          title={t("contact.chapter.wayIn")}
          answer={readings.wayIn.answer}
        >
          {wayIn}
        </Chapter>

        {/* The picture behind the answer, full width: the drawing scrolls rather
          than shrinking. How complete it is closes the card. */}
        <Chapter
          id={ids.who}
          title={whoTitle}
          answer={t("contact.chapter.whoAnswer", { name: read.targetName })}
        >
          <Panel>
            <PanelBody>
              <RelationshipMap
                model={model}
                focusId={read.focused}
                onFocus={setFocus}
                onAction={(nodeId, actionId) => {
                  if (actionId === WRITE_EMAIL_ACTION) {
                    onWrite?.();
                    return;
                  }
                  const picked = read.routeAt(nodeId);
                  // The reader's own route, or one spoken for, opens no drawer.
                  // The server refuses an ask whose introducer is the contact
                  // making it.
                  if (
                    picked &&
                    !own(picked) &&
                    picked.availability === "available"
                  ) {
                    setAsking(picked);
                  }
                }}
                completenessText={complete}
                // The account page's word set, with the one label that names the
                // subject swapped: this drawing is about a contact.
                labels={mapLabels(t, locale, t("contact.intro.mapRegion"))}
                // The selected contact's own detail, including the one write this
                // picture offers: recording an observed acquaintance.
                panelSlot={
                  read.focused && read.anchor ? (
                    <EdgeDetail
                      graph={data}
                      nodeId={read.focused}
                      anchorId={read.anchor.id}
                      viewerId={viewerId}
                      onOpenEmail={onOpenEmail}
                    />
                  ) : null
                }
              />
            </PanelBody>
          </Panel>
          <CaptureGapsNotice contactId={contactId} />
        </Chapter>

        <Chapter
          id={ids.progress}
          title={t("contact.chapter.progress")}
          answer={readings.progress.answer}
          action={
            <RecordIntroductionButton
              contactId={contactId}
              contactName={read.targetName}
            />
          }
        >
          <RelayPanel ask={read.open} />
          <IntroAsksPanel contactId={contactId} contactName={read.targetName} />
          {/* What moved lately closes the tab. */}
          {view ? <MomentsPanel view={view} /> : null}
        </Chapter>

        {asking ? (
          <IntroDrawer
            contactId={contactId}
            contactName={read.targetName}
            route={asking}
            open
            onClose={() => setAsking(undefined)}
          />
        ) : null}
      </Chapters>
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
  data: Graph,
  asks: readonly IntroRequest[],
  focus: string | null,
) {
  const nodes = data.nodes ?? [];
  const routes = data.routes ?? [];
  const alternatives =
    routes.length > 1 || (routes.length === 0 && !!data.route);
  const withheld = (data.groups_omitted ?? []).length > 0;
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
    alternatives,
    skipLead: routes.length > 1,
    legacy: routes.length === 0 ? data.route : undefined,
    withheld,
    // Whether the working grid has a left column at all.
    hasMain: alternatives || withheld,
    // The drawing's verb, joined back to the route it means.
    //
    // A node is keyed by the graph's own id and a route names a bare user id,
    // so the join is the node's `user_id` — read out of the same payload rather
    // than re-derived from the id's shape. Comparing the two raw (which is what
    // this did) matched nothing, and every "use this route" on the picture
    // silently did nothing at all.
    routeAt: (nodeId: string) => {
      const node = nodes.find((n) => n.id === nodeId);
      return node?.user_id === undefined
        ? undefined
        : routes.find((r) => r.via_user_id === node.user_id);
    },
    // A selection only means something against the graph ON SCREEN. This tab
    // stays mounted as a reader moves between contacts, so a raw id would open
    // the detail on a node this graph does not have — describing a record the
    // reader has already left. Validating beats resetting on contactId: it also
    // covers a graph that reloads without the node for any other reason.
    focused: nodes.some((n) => n.id === focus) ? focus : null,
  };
}

const OPEN = new Set(["requested", "accepted", "name_drop_approved"]);
