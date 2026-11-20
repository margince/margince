// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, useMemo } from "react";
import type { components } from "../../../api/schema";
import { Chapter } from "../../../design-system/chapter";
import { Panel } from "../../../design-system/panel";
import { RelationshipMap } from "../../../design-system/relationshipmap";
import { formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useViewerId } from "../../../lib/session";
import { useOwnRoute } from "../contactroutes";
import { CaptureGapsNotice } from "./capturegaps";
import { EdgeDetail } from "./edgedetail";
import {
  completenessText,
  mapModelFromContactGraph,
  WRITE_EMAIL_ACTION,
} from "./mapmodel";
import type { Graph, GraphReading } from "./readgraph";

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];

// The picture behind the answer, full width: the drawing scrolls rather than
// shrinking. How complete it is closes the card.
export function WhoKnowsChapter({
  id,
  title,
  contactId,
  data,
  read,
  onFocus,
  onAsk,
  onWrite,
  onOpenEmail,
  labels,
}: Readonly<{
  id: string;
  title: string;
  contactId: string;
  data: Graph;
  read: GraphReading;
  onFocus: (nodeId: string | null) => void;
  onAsk: (route: RouteCandidate) => void;
  onWrite?: () => void;
  onOpenEmail?: (activityId: string) => void;
  labels: ComponentProps<typeof RelationshipMap>["labels"];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const own = useOwnRoute();
  const viewerId = useViewerId();
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

  const model = mapModelFromContactGraph(data, copy, {
    userId: viewerId,
    mayWrite: onWrite !== undefined,
  });
  const complete = completenessText(data, copy, (n) =>
    t("contact.graph.droppedNote", { count: formatNumber(n, locale) }),
  );
  if (read.lonely) {
    return null;
  }
  return (
    <Chapter
      id={id}
      title={title}
      answer={t("contact.chapter.whoAnswer", { name: read.targetName })}
    >
      <Panel className="pn-who">
        <RelationshipMap
          model={model}
          focusId={read.focused}
          onFocus={onFocus}
          onAction={(nodeId, actionId) => {
            if (actionId === WRITE_EMAIL_ACTION) {
              onWrite?.();
              return;
            }
            const picked = read.routeAt(nodeId);
            // The reader's own route, or one spoken for, opens no drawer.
            // The server refuses an ask whose introducer is the contact
            // making it.
            if (picked && !own(picked) && picked.availability === "available") {
              onAsk(picked);
            }
          }}
          completenessText={complete}
          labels={labels}
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
      </Panel>
      <CaptureGapsNotice contactId={contactId} />
    </Chapter>
  );
}
