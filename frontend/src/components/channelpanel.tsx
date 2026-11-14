// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { AppLink } from "../design-system/applink";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CopyAction } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { OffsiteLink } from "../design-system/offsitelink";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../design-system/overviewcard";
import { QueryStates } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import {
  isVerbosity,
  LinkExistingDialog,
  type Verbosity,
  verbosityOptions,
} from "./channellinkdialog";

type LinkableChannel = components["schemas"]["LinkableChannel"];
type ChannelLinkState = components["schemas"]["ChannelLinkState"];
type ChannelLink = components["schemas"]["ChannelLink"];
type ChannelInvites = components["schemas"]["ChannelInvites"];
type LinkChannelRequest = components["schemas"]["LinkChannelRequest"];
type RecordType = components["schemas"]["ChannelLinkRecordType"];

// A deal and its project share one channel, so every record's read goes stale
// when any link changes.
const CHANNEL_LINK_KEY = "channel-link";

type RecordRef = Readonly<{ recordType: RecordType; recordId: string }>;

function linkPath({ recordType, recordId }: RecordRef) {
  return { params: { path: { record_type: recordType, id: recordId } } };
}

/**
 * The team chat channel of a deal or project. It shows what it is linked to
 * and how much Margince posts there, and it links, tunes and unlinks it. One read
 * decides which of the four states the card draws.
 */
export function ChannelPanel({
  recordType,
  recordId,
  canEdit,
}: Readonly<RecordRef & { canEdit: boolean }>) {
  const t = useT();
  const ref = { recordType, recordId };
  const query = useQuery({
    queryKey: [CHANNEL_LINK_KEY, recordType, recordId],
    queryFn: async (): Promise<ChannelLinkState> =>
      unwrap(
        await api.GET("/channel-links/{record_type}/{id}", linkPath(ref)),
        t,
      ),
  });
  const state = query.data;
  // Held here, not in the unlinked body. A create moves the card to linked,
  // and the notice must outlive the body that asked for the channel.
  const [invites, setInvites] = useState<ChannelInvites | null>(null);
  return (
    <OverviewCard
      title={t("channelLink.title")}
      action={
        canEdit && state?.link ? (
          <UnlinkVerb
            record={ref}
            channel={state.link}
            // A paused channel has nothing left to tune, so Unlink is the
            // card's one verb and stands in view.
            inView={state.state === "paused"}
          />
        ) : undefined
      }
      empty={state?.state === "off" ? t("channelLink.off") : undefined}
      emptyAction={
        state?.state === "off" ? (
          <AppLink
            className="link-button"
            href={routeHash(settingsHref("integrations", "telegram"))}
          >
            {t("channelLink.openSettings")}
          </AppLink>
        ) : undefined
      }
    >
      {state?.state === "off" ? undefined : (
        <QueryStates query={query} pendingLabel={t("channelLink.title")}>
          {state?.state === "none" && (
            <UnlinkedBody
              record={ref}
              canEdit={canEdit}
              onCreated={setInvites}
            />
          )}
          {state?.link && (
            <LinkedBody
              record={ref}
              channel={state.link}
              paused={state.state === "paused"}
              canEdit={canEdit}
            />
          )}
          {invites && <InvitesNotice invites={invites} />}
        </QueryStates>
      )}
    </OverviewCard>
  );
}

// The link verb both ways in: create posts no channel name, existing posts the
// one the reader typed. Each caller passes the whole request, so a click never
// sends a form state older than the one it was pressed on.
function useLinkChannel(
  record: RecordRef,
  onCreated: (invites: ChannelInvites | null) => void,
) {
  const t = useT();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: LinkChannelRequest) =>
      unwrap(
        await api.POST("/channel-links/{record_type}/{id}", {
          ...linkPath(record),
          body,
        }),
        t,
      ),
    onSuccess: (answer, body) => {
      if (body.mode === "create") onCreated(answer.invites ?? null);
      void qc.invalidateQueries({ queryKey: [CHANNEL_LINK_KEY] });
    },
  });
}

// The channels the record can be linked to, read only while the dialog is open:
// the list is the bot's, and it changes in the chat, not here.
function useLinkableChannels(record: RecordRef, open: boolean) {
  const t = useT();
  return useQuery({
    queryKey: [
      CHANNEL_LINK_KEY,
      "linkable",
      record.recordType,
      record.recordId,
    ],
    enabled: open,
    queryFn: async (): Promise<LinkableChannel[]> =>
      unwrap(
        await api.GET(
          "/channel-links/{record_type}/{id}/channels",
          linkPath(record),
        ),
        t,
      ).channels,
  });
}

function UnlinkedBody({
  record,
  canEdit,
  onCreated,
}: Readonly<{
  record: RecordRef;
  canEdit: boolean;
  onCreated: (invites: ChannelInvites | null) => void;
}>) {
  const t = useT();
  const link = useLinkChannel(record, onCreated);
  const [existingOpen, setExistingOpen] = useState(false);
  const linkable = useLinkableChannels(record, existingOpen);
  const createFailed = link.isError && link.variables?.mode === "create";
  return (
    <>
      <OverviewList>
        <OverviewRow
          title={t("channelLink.none")}
          sub={t("channelLink.noneSub")}
        />
      </OverviewList>
      {canEdit && (
        <div className="card-actions">
          <Button
            variant="primary"
            pending={link.isPending && link.variables?.mode === "create"}
            disabled={link.isPending}
            onClick={() => link.mutate({ mode: "create" })}
          >
            {t("channelLink.create")}
          </Button>
          <Button onClick={() => setExistingOpen(true)} aria-haspopup="dialog">
            {t("channelLink.linkExisting")}
          </Button>
        </div>
      )}
      {createFailed && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("channelLink.linkFailed")}
        >
          {problemMessageOf(link.error, t)}
        </Callout>
      )}
      <LinkExistingDialog
        open={existingOpen}
        channels={linkable.data}
        onClose={() => {
          setExistingOpen(false);
          link.reset();
        }}
        link={link}
      />
    </>
  );
}

// Shown once, right after a create. A member left out cannot read the channel
// until they have an account, or until an admin lets the app add them.
function InvitesNotice({ invites }: Readonly<{ invites: ChannelInvites }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const lines = [
    invites.not_in_chat > 0 &&
      plural("channelLink.notInChat", invites.not_in_chat, {
        count: formatNumber(invites.not_in_chat, locale),
      }),
    invites.not_added > 0 &&
      plural("channelLink.notAdded", invites.not_added, {
        count: formatNumber(invites.not_added, locale),
      }),
  ].filter((line): line is string => typeof line === "string");
  if (lines.length === 0) return null;
  return <Callout tone="info" kind="outcome" title={lines.join(" ")} />;
}

function LinkedBody({
  record,
  channel,
  paused,
  canEdit,
}: Readonly<{
  record: RecordRef;
  channel: ChannelLink;
  paused: boolean;
  canEdit: boolean;
}>) {
  const t = useT();
  return (
    <>
      <OverviewList>
        <OverviewRow
          title={
            <>
              <OffsiteLink
                href={channel.channel_url}
                label={t("channelLink.open", { name: channel.channel_name })}
              >
                {channel.channel_name}
              </OffsiteLink>
              <CopyAction
                text={channel.channel_url}
                labels={{
                  copy: t("channelLink.copy"),
                  copied: t("channelLink.copied"),
                  remedy: t("channelLink.copyRemedy"),
                }}
              />
            </>
          }
          right={
            paused ? (
              <Badge tone="warning">{t("channelLink.pausedBadge")}</Badge>
            ) : undefined
          }
        />
      </OverviewList>
      {paused ? (
        <Callout tone="warning" title={t("channelLink.paused")}>
          {t("channelLink.pausedBody")}
        </Callout>
      ) : (
        <VerbositySetting
          record={record}
          verbosity={channel.verbosity}
          canEdit={canEdit}
        />
      )}
    </>
  );
}

function VerbositySetting({
  record,
  verbosity,
  canEdit,
}: Readonly<{ record: RecordRef; verbosity: Verbosity; canEdit: boolean }>) {
  const t = useT();
  const qc = useQueryClient();
  const change = useMutation({
    mutationFn: async (next: Verbosity) =>
      unwrap(
        await api.PATCH("/channel-links/{record_type}/{id}", {
          ...linkPath(record),
          body: { verbosity: next },
        }),
        t,
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: [CHANNEL_LINK_KEY] });
    },
  });
  return (
    <>
      <SettingList>
        <SettingRow
          label={t("channelLink.verbosity")}
          description={t("channelLink.verbosityHint")}
          control={(control) => (
            <Select
              {...control}
              options={verbosityOptions(t)}
              value={change.isPending ? change.variables : verbosity}
              disabled={!canEdit}
              aria-busy={change.isPending}
              onChange={(value) => {
                if (isVerbosity(value)) change.mutate(value);
              }}
            />
          )}
        />
      </SettingList>
      {change.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("channelLink.verbosityFailed")}
        >
          {problemMessageOf(change.error, t)}
        </Callout>
      )}
    </>
  );
}

function UnlinkVerb({
  record,
  channel,
  inView,
}: Readonly<{ record: RecordRef; channel: ChannelLink; inView: boolean }>) {
  const t = useT();
  const qc = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const unlink = useMutation({
    mutationFn: async (target: RecordRef) => {
      unwrap(
        await api.DELETE("/channel-links/{record_type}/{id}", linkPath(target)),
        t,
      );
    },
    onSuccess: () => {
      setConfirming(false);
      void qc.invalidateQueries({ queryKey: [CHANNEL_LINK_KEY] });
    },
  });
  const verb = (
    <Button
      variant={inView ? undefined : "danger"}
      aria-haspopup="dialog"
      aria-label={t("channelLink.unlinkNamed", { name: channel.channel_name })}
      onClick={() => setConfirming(true)}
    >
      {t("channelLink.unlink")}
    </Button>
  );
  return (
    <>
      {inView ? (
        verb
      ) : (
        <OverflowMenu label={t("record.moreActions")}>{verb}</OverflowMenu>
      )}
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("channelLink.unlinkTitle")}
        confirmLabel={t("channelLink.unlink")}
        confirmVariant="danger"
        pending={unlink.isPending}
        error={unlink.isError ? problemMessageOf(unlink.error, t) : null}
        onConfirm={() => unlink.mutate(record)}
      >
        <p>{t("channelLink.unlinkBody")}</p>
      </ConfirmModal>
    </>
  );
}
