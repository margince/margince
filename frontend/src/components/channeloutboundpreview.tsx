// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Disclosure } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { NoteMarkdown } from "../design-system/markdown";
import {
  ProviderMark,
  providerBrandName,
} from "../design-system/provider-mark";
import { QueryStates } from "../design-system/querystates";
import { useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { verbosityOptions } from "./channellinkdialog";
import type { MessengerScope } from "./messenger/messenger.queries";
import { MessengerTeamComparison } from "./messenger/messenger.teamcomparison";

type Link = components["schemas"]["ChannelLink"];
type Kind = components["schemas"]["ChannelLinkRecordType"];
type Preview = components["schemas"]["ChannelDeliveryPreview"];

export function ChannelOutboundPreview({
  channel,
  recordType,
  recordId,
  busy,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  channel: Link;
  recordType: Kind;
  recordId: string;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: (preview: Preview) => void;
}>) {
  const t = useT();
  const query = useQuery({
    queryKey: [
      "channel-link",
      "delivery-preview",
      recordType,
      recordId,
      channel.id,
      channel.message_detail,
      channel.verbosity,
    ],
    queryFn: async () =>
      unwrap(
        await api.GET("/channel-links/{record_type}/{id}/delivery-preview", {
          params: { path: { record_type: recordType, id: recordId } },
        }),
      ),
    retry: false,
    refetchOnMount: "always",
  });
  const preview = query.data;
  const current =
    preview?.link_id === channel.id &&
    preview.verbosity === channel.verbosity &&
    preview.message_detail === (channel.message_detail ?? "brief");
  return (
    <ConfirmModal
      open
      error={error}
      title={t("channelPreview.title")}
      confirmLabel={t("channelPreview.enable")}
      onClose={onClose}
      pending={busy}
      confirmDisabled={!current || query.isFetching || query.isError || busy}
      onConfirm={() => {
        if (preview && current && !query.isFetching && !query.isError)
          onConfirm(preview);
      }}
    >
      <p>{t("channelPreview.help")}</p>
      <QueryStates query={query} pendingLabel={t("channelPreview.loading")}>
        {preview && !current && (
          <ErrorLine>{t("channelDelivery.linkChanged")}</ErrorLine>
        )}
        {preview && current && (
          <ChannelDeliveryPreviewContent
            preview={preview}
            scope={{
              provider: preview.provider,
              record_type: recordType,
              id: recordId,
            }}
          />
        )}
      </QueryStates>
    </ConfirmModal>
  );
}

export function ChannelDeliveryPreviewContent({
  preview,
  scope,
}: Readonly<{ preview: Preview; scope?: MessengerScope }>) {
  const t = useT();
  return (
    <>
      <p>
        <ProviderMark providerKey={preview.provider} />{" "}
        {providerBrandName(preview.provider)} · {preview.workspace} ·{" "}
        {preview.channel_name}
      </p>
      <p>
        {preview.root_id
          ? t("channelPreview.thread")
          : t("channelPreview.channel")}
      </p>
      <p>{t("channelPreview.audience")}</p>
      <ul aria-label={t("channelPreview.audience")}>
        {preview.audience.map((member) => (
          <li key={member.id}>{member.name}</li>
        ))}
      </ul>
      {scope && (
        <Disclosure summary={t("messenger.teamComparison")}>
          <MessengerTeamComparison members={preview.audience} scope={scope} />
        </Disclosure>
      )}
      <p>
        {t("channelPreview.settings", {
          frequency:
            verbosityOptions(t).find(
              (option) => option.value === preview.verbosity,
            )?.label ?? preview.verbosity,
          detail: t(
            preview.message_detail === "full"
              ? "channelLink.detailFull"
              : "channelLink.detailBrief",
          ),
        })}
      </p>
      <p>{t("channelPreview.scope")}</p>
      <p>{t("channelPreview.overview")}</p>
      <NoteMarkdown source={readablePreview(preview.overview_text)} />
      <p>{t("channelPreview.updates")}</p>
      {preview.update_text ? (
        <NoteMarkdown source={readablePreview(preview.update_text)} />
      ) : (
        <p>{t("channelPreview.noUpdates")}</p>
      )}
      {preview.excluded_updates > 0 && <p>{t("channelPreview.excluded")}</p>}
      <p>{t("channelPreview.recheck")}</p>
    </>
  );
}

function readablePreview(text: string): string {
  return text
    .replace(/<[^<>\n|]+\|([^<>\n]*)>/g, "$1")
    .replace(/\*([^*\n]+)\*/g, "**$1**");
}
