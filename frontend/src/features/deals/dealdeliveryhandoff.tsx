// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ChannelDeliveryPreviewContent } from "../../components/channeloutboundpreview";
import { ChannelPanel } from "../../components/channelpanel";
import { Disclosure } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { QueryStates } from "../../design-system/querystates";
import { formatDate, formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { unwrap } from "../../lib/problem";
import { dueOnDay } from "../../lib/standingpeeks";
import { useViewerZone } from "../../lib/viewerzone";
import { DealCommitmentsCard } from "./deal360/dealcommitments";

type Kind = components["schemas"]["ChannelLinkRecordType"];
type Handoff = components["schemas"]["ProjectHandoff"];
type ChannelLink = components["schemas"]["ChannelLink"];

function needsOutgoingPreview(link: ChannelLink | undefined) {
  return (
    !!link &&
    link.send_crm_updates !== false &&
    link.can_send_crm_updates !== false
  );
}

function handoffDestination(
  projectId: string,
  projectLink: ChannelLink | undefined,
  dealId: string,
  dealLink: ChannelLink | undefined,
): { kind: Kind; id: string; link: ChannelLink } | null {
  if (projectLink) return { kind: "project", id: projectId, link: projectLink };
  if (dealLink) return { kind: "deal", id: dealId, link: dealLink };
  return null;
}

function useHandoffChannel(kind: Kind, id: string) {
  return useQuery({
    queryKey: ["channel-link", kind, id],
    queryFn: async () =>
      unwrap(
        await api.GET("/channel-links/{record_type}/{id}", {
          params: { path: { record_type: kind, id } },
        }),
      ),
  });
}

export function DealDeliveryHandoff({
  dealId,
  projectId,
  busy,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  dealId: string;
  projectId: string;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}>) {
  const t = useT();
  const current = useHandoffChannel("deal", dealId);
  const target = useHandoffChannel("project", projectId);
  const handoff = useQuery({
    queryKey: ["project-handoff", projectId],
    queryFn: async () =>
      unwrap(
        await api.GET("/projects/{id}/handoff", {
          params: { path: { id: projectId } },
        }),
      ),
  });
  const destination = handoffDestination(
    projectId,
    target.data?.link,
    dealId,
    current.data?.link,
  );
  const previewNeeded = needsOutgoingPreview(destination?.link);
  const preview = useQuery({
    queryKey: [
      "channel-link",
      "handoff-preview",
      destination?.kind,
      destination?.id,
      destination?.link.id,
    ],
    enabled: previewNeeded && current.isSuccess && target.isSuccess,
    retry: false,
    queryFn: async () => {
      if (!destination) throw new Error("A delivery destination is required.");
      return unwrap(
        await api.GET("/channel-links/{record_type}/{id}/delivery-preview", {
          params: {
            path: { record_type: destination.kind, id: destination.id },
          },
        }),
      );
    },
  });
  const ready =
    current.isSuccess &&
    target.isSuccess &&
    handoff.isSuccess &&
    !current.isFetching &&
    !target.isFetching &&
    (!previewNeeded ||
      (preview.isSuccess &&
        !preview.isFetching &&
        preview.data.link_id === destination?.link.id));
  return (
    <ConfirmModal
      open
      title={t("channelHandoff.title")}
      confirmLabel={t("deal.startDelivery")}
      pending={busy}
      error={error}
      confirmDisabled={!ready || busy}
      onClose={onClose}
      onConfirm={() => {
        if (ready) onConfirm();
      }}
    >
      <p>{t("channelHandoff.help")}</p>
      <QueryStates query={current} pendingLabel={t("channelLink.title")}>
        <p>
          {current.data?.link
            ? t("channelHandoff.current", {
                channel: current.data.link.channel_name,
              })
            : t("channelHandoff.noCurrent")}
        </p>
      </QueryStates>
      <QueryStates query={target} pendingLabel={t("channelLink.title")}>
        <p>
          {target.data?.link
            ? t("channelHandoff.preserve", {
                channel: target.data.link.channel_name,
              })
            : current.data?.link
              ? t("channelHandoff.continue", {
                  channel: current.data.link.channel_name,
                })
              : t("channelHandoff.noChannel")}
        </p>
      </QueryStates>
      <Disclosure summary={t("channelHandoff.choose")}>
        <ChannelPanel
          recordType="project"
          recordId={projectId}
          canEdit={!busy}
        />
      </Disclosure>
      {destination && !previewNeeded && <p>{t("channelHandoff.noOutgoing")}</p>}
      {destination && previewNeeded && (
        <QueryStates query={preview} pendingLabel={t("channelPreview.loading")}>
          {preview.data?.link_id === destination.link.id && (
            <ChannelDeliveryPreviewContent
              preview={preview.data}
              scope={{
                provider: preview.data.provider,
                record_type: destination.kind,
                id: destination.id,
              }}
            />
          )}
        </QueryStates>
      )}
      <QueryStates query={handoff} pendingLabel={t("channelHandoff.loading")}>
        {handoff.data && <HandoffCommitments handoff={handoff.data} />}
      </QueryStates>
      <DealCommitmentsCard dealId={dealId} />
    </ConfirmModal>
  );
}

function HandoffCommitments({ handoff }: Readonly<{ handoff: Handoff }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const words = { zone, companyZone: useCompanyZone() };
  return (
    <>
      <p>
        {t("channelHandoff.owner", {
          owner: handoff.owner_name ?? t("list.unowned"),
        })}
      </p>
      <p>
        {handoff.target_end_date
          ? t("channelHandoff.target", {
              at: formatDate(
                handoff.target_end_date.slice(0, 10),
                locale,
                zone,
              ),
            })
          : t("channelHandoff.noTarget")}
      </p>
      <p>{t("channelHandoff.promises")}</p>
      {handoff.open_commitments.length ? (
        <ul>
          {handoff.open_commitments.map((item, index) => {
            const due = item.due_at
              ? dueOnDay(words, item.due_at, item.due_has_time, item.due_intent)
              : undefined;
            return (
              <li
                key={
                  item.task_id ?? item.claim_id ?? `${item.subject}:${index}`
                }
              >
                {item.subject}
                {item.assignee_name && ` · ${item.assignee_name}`}
                {due &&
                  ` · ${item.due_has_time ? formatDateTime(due.at, locale, due.zone) : formatDate(due.at, locale, due.zone)}`}
              </li>
            );
          })}
        </ul>
      ) : (
        <p>{t("channelHandoff.noPromises")}</p>
      )}
      {handoff.sections_omitted.length > 0 && (
        <Callout tone="warning" title={t("channelHandoff.partial")}>
          {t("channelHandoff.omitted")}
        </Callout>
      )}
      {handoff.gaps.length > 0 && (
        <Callout tone="warning" title={t("channelHandoff.gaps")}>
          <ul>
            {handoff.gaps.map((gap) => (
              <li key={`${gap.code}:${gap.source}`}>{gap.message}</li>
            ))}
          </ul>
        </Callout>
      )}
    </>
  );
}
