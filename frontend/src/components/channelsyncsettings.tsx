// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Disclosure, Field, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import { ChannelDeliveryRecovery } from "./channeldeliveryreview";
import { ChannelHistoryProfile } from "./channelhistoryprofile";
import { ChannelOutboundPreview } from "./channeloutboundpreview";

type Link = components["schemas"]["ChannelLink"];
type Patch = components["schemas"]["SetChannelVerbosityRequest"];
type Kind = components["schemas"]["ChannelLinkRecordType"];

export function ChannelSyncSettings({
  channel,
  recordType,
  recordId,
  onClose,
  onSaved,
  children,
}: Readonly<{
  channel: Link;
  recordType: Kind;
  recordId: string;
  onClose: () => void;
  onSaved: () => void;
  children: ReactNode;
}>) {
  const t = useT();
  const [previewOpen, setPreviewOpen] = useState(false);
  const heading = useId();
  const toast = useToast();
  const mutation = useMutation({
    mutationFn: async ({
      recordType,
      recordId,
      body,
    }: Readonly<{ recordType: Kind; recordId: string; body: Patch }>) =>
      unwrap(
        await api.PATCH("/channel-links/{record_type}/{id}", {
          params: { path: { record_type: recordType, id: recordId } },
          body,
        }),
      ),
    onSuccess: () => {
      onSaved();
      toast.show(t("settings.saved"));
      setPreviewOpen(false);
    },
  });
  const patch = (body: Patch) =>
    mutation.mutate({
      recordType,
      recordId,
      body: { ...body, expected_link_id: channel.id },
    });
  return (
    <Modal
      open
      intent="form"
      labelledBy={heading}
      onClose={onClose}
      closeDisabled={mutation.isPending}
    >
      <Heading size="large" className="t-h2 modal-title" id={heading}>
        {t("channelLink.manage")}
      </Heading>
      <div className="form-stack">
        <p>{channel.channel_name}</p>
        <Switch
          label={t("channelLink.readDirection")}
          checked={channel.save_channel_updates ?? true}
          pending={mutation.isPending}
          onChange={(value) =>
            patch({
              save_channel_updates: value,
            })
          }
        />
        {channel.can_send_crm_updates === false && (
          <p>{t("channelLink.privateTeamsReadOnly")}</p>
        )}
        <Switch
          label={t("channelLink.postDirection")}
          disabled={channel.can_send_crm_updates === false}
          checked={channel.send_crm_updates ?? true}
          pending={mutation.isPending}
          onChange={(value) => {
            if (value) setPreviewOpen(true);
            else patch({ send_crm_updates: false });
          }}
        />
        {channel.can_send_crm_updates !== false && (
          <Button
            disabled={mutation.isPending}
            onClick={() => setPreviewOpen(true)}
          >
            {t("channelPreview.title")}
          </Button>
        )}
        <Field
          label={t("channelLink.messageDetail")}
          hint={t("channelLink.detailHelp")}
        >
          {(control) => (
            <Select
              {...control}
              value={channel.message_detail ?? "brief"}
              options={[
                { value: "brief", label: t("channelLink.detailBrief") },
                { value: "full", label: t("channelLink.detailFull") },
              ]}
              disabled={mutation.isPending}
              onChange={(value) => {
                if (value === "brief" || value === "full")
                  patch({ message_detail: value });
              }}
            />
          )}
        </Field>
        {previewOpen && (
          <ChannelOutboundPreview
            channel={channel}
            recordType={recordType}
            recordId={recordId}
            busy={mutation.isPending}
            error={mutation.error ? problemMessageOf(mutation.error, t) : null}
            onClose={() => setPreviewOpen(false)}
            onConfirm={(preview) =>
              patch({
                send_crm_updates: true,
                expected_link_id: preview.link_id,
                expected_version: preview.version,
              })
            }
          />
        )}
        <ChannelDeliveryRecovery
          channel={channel}
          recordType={recordType}
          recordId={recordId}
          busy={mutation.isPending}
        />
        {channel.provider === "slack" && (
          <Disclosure summary={t("channelProfile.details")}>
            <ChannelHistoryProfile profile={channel.history_profile} />
          </Disclosure>
        )}
        {children}
        {mutation.isError && (
          <Callout tone="danger" title={t("channelLink.verbosityFailed")}>
            {problemMessageOf(mutation.error, t)}
          </Callout>
        )}
      </div>
      <div className="actions">
        <Button onClick={onClose} disabled={mutation.isPending}>
          {t("common.close")}
        </Button>
      </div>
    </Modal>
  );
}
