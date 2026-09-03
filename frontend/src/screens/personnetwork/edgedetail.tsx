// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What one selected person on the map is to this contact, and the one write
// the picture offers.
//
// It rides the map's own panel slot rather than sitting beside it: the detail
// is ABOUT the selection, and a panel that stayed on screen after the
// selection cleared would describe somebody the reader is no longer looking at.

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useCanWrite } from "../../app/capability";
import { useRecordZone } from "../../app/recordzone";
import { Badge, Button, Card } from "../../design-system/atoms";
import { EmailReference } from "../../design-system/emailreference";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf } from "../common";

type Graph = components["schemas"]["PersonGraph"];
type GraphNode = components["schemas"]["PersonGraphNode"];

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
}: Readonly<{ graph: Graph; nodeId: string; anchorId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const node = graph.nodes?.find((n) => n.id === nodeId);
  const edges = (graph.edges ?? []).filter(
    (e) => e.from === nodeId || e.to === nodeId,
  );
  if (!node || edges.length === 0) {
    return (
      <p className="pn-counts">
        {t("person.graph.noEdge", { name: node?.label ?? "" })}
      </p>
    );
  }
  return (
    <Card title={node.label} level={3}>
      {node.suggest_edge && node.person_id && (
        <RecordWorksWith graph={graph} peer={node} />
      )}
      {edges.map((edge) => {
        // Whom this edge joins the SELECTED node to. Both ends are read
        // because an account-arm edge need not touch the anchor at all.
        const otherEnd = edge.from === nodeId ? edge.to : edge.from;
        const otherNode = graph.nodes?.find((n) => n.id === otherEnd);
        const withWhom =
          otherEnd === anchorId
            ? t("person.graph.withContact")
            : otherNode
              ? t("person.graph.withColleague", { name: otherNode.label })
              : undefined;
        const receipts = edge.receipts ?? [];
        return (
          <div key={`${edge.from}->${edge.to}`} className="pn-edge-facts">
            <p className="pn-band">
              <Badge>{t(`person.band.${edge.strength_bucket}`)}</Badge>
              {withWhom && <span>{withWhom}</span>}
            </p>
            <p className="pn-counts">
              {t("person.graph.counts", {
                total: formatNumber(edge.interactions_90d, locale),
                inbound: formatNumber(edge.inbound_90d ?? 0, locale),
                outbound: formatNumber(edge.outbound_90d ?? 0, locale),
              })}
            </p>
            {receipts.length > 0 ? (
              <ul className="pn-receipts">
                {receipts.map((r) => (
                  <li key={r.activity_id}>
                    {/* The canonical citation for a MAIL. A receipt names the
                        message it is evidence of — it is not a place to read
                        one, which is why this is EmailReference and not
                        EmailEntry — and it carries the record's zone, so every
                        reader names the same day.

                        The graph counts attendees and organizers as well as
                        correspondents, so a receipt can be a meeting or a
                        call. Those keep the plain line: an email's icon and an
                        email's "No subject" on a meeting would tell a reader
                        it was a mail. */}
                    {r.kind === "email" ? (
                      <EmailReference
                        subject={r.subject}
                        occurredAt={formatDate(
                          r.occurred_at,
                          locale,
                          recordZone,
                        )}
                      />
                    ) : (
                      <>
                        {r.subject ?? t("person.graph.untitledMessage")} ·{" "}
                        {formatDate(r.occurred_at, locale, recordZone)}
                      </>
                    )}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="pn-counts">{t("person.graph.countsOnly")}</p>
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
          person_id: pair.anchor,
          counterparty_person_id: pair.peer,
          source: "manual",
        },
      });
      if (error) {
        throw error;
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: ["person-graph", graph.person_id],
      }),
  });
  if (!mayRecord) {
    return null;
  }
  return (
    <p className="pn-suggest">
      {record.isError ? (
        <span role="alert">{problemMessageOf(record.error, t)}</span>
      ) : (
        <Button
          small
          disabled={record.isPending}
          onClick={() => {
            if (peer.person_id) {
              record.mutate({ anchor: graph.person_id, peer: peer.person_id });
            }
          }}
        >
          {t("person.graph.recordWorksWith", { name: peer.label })}
        </Button>
      )}
    </p>
  );
}
