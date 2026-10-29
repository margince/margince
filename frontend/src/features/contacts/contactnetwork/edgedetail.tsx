// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What one selected contact on the map is to this contact, and the one write
// the picture offers.
//
// It rides the map's own panel slot rather than sitting beside it: the detail
// is ABOUT the selection, and a panel that stayed on screen after the
// selection cleared would describe somebody the reader is no longer looking at.

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { Badge, Button, Card } from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import { relayMemberFor } from "../../../design-system/provenanceof";
import { formatNumber } from "../../../format/format";
import { type Translator, useLocale, useT } from "../../../i18n";
import { useCanWrite } from "../../../lib/capability";
import { BAND_BADGE } from "../companygraph";
import { ReceiptList } from "./receipts";

type Graph = components["schemas"]["ContactGraph"];
type GraphNode = components["schemas"]["ContactGraphNode"];

/**
 * EdgeDetail shows what the selected node's relationship is made of.
 *
 * The receipts are the point: counts say a route exists, the messages say the
 * reader is not being asked to trust a number.
 */
export function EdgeDetail({
  graph,
  nodeId,
  anchorId,
  viewerId,
  onOpenEmail,
}: Readonly<{
  graph: Graph;
  nodeId: string;
  anchorId: string;
  // The signed-in user, whose own node is titled "You" like the rest of the tab.
  viewerId: string | undefined;
  // Opens one of the cited messages in the contact page's email drawer.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const node = graph.nodes?.find((n) => n.id === nodeId);
  const edges = (graph.edges ?? []).filter(
    (e) => e.from === nodeId || e.to === nodeId,
  );
  if (!node || edges.length === 0) {
    return (
      <p className="pn-counts t-sub">
        {t("contact.graph.noEdge", { name: node?.label ?? "" })}
      </p>
    );
  }
  return (
    <Card title={nameFor(node, viewerId, t)} level={3}>
      {node.suggest_edge && node.contact_id && (
        <RecordWorksWith graph={graph} peer={node} />
      )}
      {edges.map((edge) => {
        // Whom this edge joins the SELECTED node to. Both ends are read
        // because an account-arm edge need not touch the anchor at all.
        const otherEnd = edge.from === nodeId ? edge.to : edge.from;
        const otherNode = graph.nodes?.find((n) => n.id === otherEnd);
        const withWhom =
          otherEnd === anchorId
            ? t("contact.graph.withContact")
            : otherNode
              ? t("contact.graph.withColleague", {
                  name: nameFor(otherNode, viewerId, t),
                })
              : undefined;
        const receipts = edge.receipts ?? [];
        const memberId = [node, otherNode].find((end) => end?.user_id)?.user_id;
        return (
          <div key={`${edge.from}->${edge.to}`}>
            <p className="pn-band">
              <Badge>{t(BAND_BADGE[edge.strength_bucket])}</Badge>
              {withWhom && <span>{withWhom}</span>}
            </p>
            <p className="pn-counts t-sub">
              {t("contact.graph.counts", {
                total: formatNumber(edge.interactions_90d, locale),
                inbound: formatNumber(edge.inbound_90d ?? 0, locale),
                outbound: formatNumber(edge.outbound_90d ?? 0, locale),
              })}
            </p>
            {receipts.length > 0 ? (
              <ReceiptList
                receipts={receipts}
                relayMember={relayMemberFor(memberId, viewerId)}
                onOpenEmail={onOpenEmail}
              />
            ) : (
              <p className="pn-counts t-sub">{t("contact.graph.countsOnly")}</p>
            )}
          </div>
        );
      })}
    </Card>
  );
}

/**
 * RecordWorksWith is the one-click acceptance of an observed acquaintance:
 * the server flagged the pair as strong and unrecorded, and the click is the
 * rep's OWN attributed write of a works_with relationship — nothing was
 * staged, nothing happens until they press it. The flag vanishes on the next
 * read because the edge now exists.
 */
function RecordWorksWith({
  graph,
  peer,
}: Readonly<{ graph: Graph; peer: GraphNode }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const mayRecord = useCanWrite("relationship", "create");
  const record = useMutation({
    mutationFn: async (pair: { anchor: string; peer: string }) => {
      const { error } = await api.POST("/relationships", {
        body: {
          kind: "works_with",
          contact_id: pair.anchor,
          counterparty_contact_id: pair.peer,
          source: "manual",
        },
      });
      if (error) {
        throw error;
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: ["contact-graph", graph.contact_id],
      }),
  });
  if (!mayRecord) {
    return null;
  }
  return (
    <p className="pn-suggest">
      {record.isError ? (
        <ErrorLine inline error={record.error} />
      ) : (
        <Button
          disabled={record.isPending}
          onClick={() => {
            if (peer.contact_id) {
              record.mutate({
                anchor: graph.contact_id,
                peer: peer.contact_id,
              });
            }
          }}
        >
          <span className="pn-suggest-label">
            {t("contact.graph.recordWorksWith", { name: peer.label })}
          </span>
        </Button>
      )}
    </p>
  );
}

function nameFor(
  node: GraphNode,
  viewerId: string | undefined,
  t: Translator,
): string {
  return viewerId !== undefined && node.user_id === viewerId
    ? t("contact.intro.heroYou")
    : node.label;
}
