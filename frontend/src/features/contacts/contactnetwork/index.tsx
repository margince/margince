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

import { type ReactNode, useId, useState } from "react";

import type { components } from "../../../api/schema";
import { Chapter, ChapterJump, Chapters } from "../../../design-system/chapter";
import { ErrorLine } from "../../../design-system/errorline";
import { Panel, PanelBody } from "../../../design-system/panel";
import { SurfaceState } from "../../../design-system/surfacestate";
import { useLocale, useT } from "../../../i18n";
import { mapLabels } from "../../companies/companycontacts/summary";
import { useContactGraph } from "../contactgraph";
import { availabilityLabel, RoutesPanel } from "../contactroutes";
import { IntroAsksPanel, isOpenAsk } from "../introasks";
import { IntroDrawer } from "../introdrawer";
import { type IntroRequest, useIntroRequests } from "../introrequests";
import { RecordIntroductionButton } from "../recordintroduction";
import { CaptureGapsNotice, useCaptureGapsShown } from "./capturegaps";
import { useNetworkReadings } from "./decision";
import { LeadPanel } from "./leadpanel";
import { latestChange, MomentsTimeline } from "./moments";
import { type Graph, type GraphReading, readGraph } from "./readgraph";
import { RelaySteps } from "./relay";
import { WhoKnowsChapter } from "./whoknows";
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

// The tab's three chapters over one read graph: the way in, who knows the
// contact, and where the introduction and the relationship stand.
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
  const asks = useIntroRequests(contactId);
  const [focus, setFocus] = useState<string | null>(null);
  const [asking, setAsking] = useState<RouteCandidate | undefined>();
  const baseId = useId();
  const ids = {
    wayIn: `${baseId}-way`,
    who: `${baseId}-who`,
    progress: `${baseId}-progress`,
  };

  const read = readGraph(data, asks.data ?? [], focus);
  const settledAsks = (asks.data ?? []).filter((ask) => !isOpenAsk(ask));
  const gapsShown = useCaptureGapsShown(contactId);
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

  const wayIn = wayInParts({
    read,
    data,
    contactId,
    t,
    onAsk: setAsking,
    onOpenEmail,
  });
  // With no map below, the mailbox notice sits under the way in. One that
  // draws nothing must not leave the chapter an empty card.
  const lonelyNotice =
    read.lonely && gapsShown ? (
      <CaptureGapsNotice contactId={contactId} />
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
            ...(read.lonely ? [] : [{ id: ids.who, title: whoTitle }]),
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
          {wayIn || lonelyNotice ? (
            <>
              {wayIn}
              {lonelyNotice}
            </>
          ) : null}
        </Chapter>

        <WhoKnowsChapter
          id={ids.who}
          title={whoTitle}
          contactId={contactId}
          data={data}
          read={read}
          onFocus={setFocus}
          onAsk={setAsking}
          onWrite={onWrite}
          onOpenEmail={onOpenEmail}
          // The account page's word set, with the one label that names the
          // subject swapped: this drawing is about a contact.
          labels={mapLabels(t, locale, t("contact.intro.mapRegion"))}
        />

        <ProgressChapter
          id={ids.progress}
          answer={readings.progress.answer}
          contactId={contactId}
          read={read}
          view={view}
          settledAsks={settledAsks}
          // Still true while a retry runs, so the card holding the Retry is
          // not taken away by the read its own mount starts.
          asksFailed={asks.errorUpdatedAt > asks.dataUpdatedAt}
        />

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

// The way in: the lead route, a lone route from an older server, and a group
// withheld from the reader. Null when there is none, so no card is framed.
function wayInParts({
  read,
  data,
  contactId,
  t,
  onAsk,
  onOpenEmail,
}: Readonly<{
  read: GraphReading;
  data: Graph;
  contactId: string;
  t: ReturnType<typeof useT>;
  onAsk: (route: RouteCandidate) => void;
  onOpenEmail?: (activityId: string) => void;
}>): ReactNode {
  return read.lead || read.hasMain ? (
    <>
      {/* The lead, or nothing. With no way in the chapter's answer says so,
          and a panel repeating it would make the page say it twice. */}
      {read.lead ? (
        <LeadPanel
          route={read.lead}
          alternatives={read.routes.slice(1)}
          roles={read.roles}
          targetName={read.targetName}
          targetId={contactId}
          blocked={availabilityLabel(read.lead.availability, t)}
          onAsk={onAsk}
          onOpenEmail={onOpenEmail}
        />
      ) : null}
      {/* Only a server that predates the ranked list sends a lone route,
          which the lead card cannot draw. */}
      {read.legacy ? <RoutesPanel graph={data} onAsk={onAsk} /> : null}
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
}

// Where the introduction stands and what moved lately. A chapter with nothing
// in it would still draw its card, as a bare rule.
function ProgressChapter({
  id,
  answer,
  contactId,
  read,
  view,
  settledAsks,
  asksFailed,
}: Readonly<{
  id: string;
  answer: ReactNode;
  contactId: string;
  read: GraphReading;
  view?: RelationshipMoments;
  settledAsks: readonly IntroRequest[];
  asksFailed: boolean;
}>) {
  const t = useT();
  const progressShown =
    read.showRelay || view !== undefined || settledAsks.length > 0;
  const asksPanel = (
    <IntroAsksPanel contactId={contactId} contactName={read.targetName} />
  );
  return (
    <Chapter
      id={id}
      title={t("contact.chapter.progress")}
      answer={answer}
      action={
        <RecordIntroductionButton
          contactId={contactId}
          contactName={read.targetName}
        />
      }
    >
      {progressShown ? (
        <>
          {/* One card: where the handoff stands, then what moved lately
              with earlier asks among it. */}
          <Panel>
            {read.showRelay ? (
              <PanelBody>
                <RelaySteps ask={read.open} />
              </PanelBody>
            ) : null}
            <MomentsTimeline view={view} asks={settledAsks} />
          </Panel>
          {/* An open ask keeps its verbs in a card of its own. */}
          {asksPanel}
        </>
      ) : asksFailed ? (
        // A failed read holds the only Retry for it, so its card stays.
        asksPanel
      ) : null}
    </Chapter>
  );
}
