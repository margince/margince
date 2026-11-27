// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { MessagesSquare } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { DockedWindow, type DockState } from "../../design-system/dockedwindow";
import { VerbWord } from "../../design-system/iconaction";
import { ProviderMark } from "../../design-system/provider-mark";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { isMessengerProvider } from "../../lib/messagingproviders";
import { unwrap } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { CHANNEL_LINK_KEY } from "../channelpanel";
import { MessengerAccountActions } from "./messenger.account";
import { MessengerMessages } from "./messenger.messages";
import { MessengerPicker } from "./messenger.picker";
import {
  MESSENGER_PROVIDERS,
  type MessengerProvider,
  type MessengerScope,
  useMessengerConversation,
  useMessengerIdentity,
} from "./messenger.queries";
import "./messenger.css";

type RecordType = components["schemas"]["ChannelLinkRecordType"];

export function RecordMessenger({
  recordType,
  recordId,
  name,
  disabled,
  reasonId,
}: Readonly<{
  recordType: RecordType;
  recordId: string;
  name: string;
  disabled?: boolean;
  reasonId?: string;
}>) {
  const t = useT();
  const [params, replaceParams] = useUrlParams();
  const [open, Open] = useState(() => params.has("messenger") && !disabled);
  const returnedProvider = params.get("provider") ?? "";
  return (
    <>
      <Button
        disabled={disabled}
        reasonId={reasonId}
        onClick={() => Open(true)}
      >
        <MessagesSquare aria-hidden="true" />
        <VerbWord word={t("messenger.verb")} label={t("messenger.verb")} />
      </Button>
      {open && (
        <RecordMessengerWindow
          recordType={recordType}
          recordId={recordId}
          name={name}
          initialProvider={
            isMessengerProvider(returnedProvider) ? returnedProvider : undefined
          }
          onClose={() => {
            Open(false);
            const next = new Map(params);
            next.delete("messenger");
            next.delete("provider");
            replaceParams(next);
          }}
        />
      )}
    </>
  );
}

function RecordMessengerWindow({
  recordType,
  recordId,
  name,
  onClose,
  initialProvider,
}: Readonly<{
  recordType: RecordType;
  recordId: string;
  name: string;
  onClose: () => void;
  initialProvider?: MessengerProvider;
}>) {
  const t = useT();
  const [dock, Dock] = useState<DockState>("open");
  const [chosen, Chosen] = useState<MessengerProvider | null>(
    initialProvider ?? null,
  );
  const [destination, Destination] = useState<string | null>(null);
  const [picking, Picking] = useState(false);
  const seat = useViewerId();
  const link = useQuery({
    queryKey: [CHANNEL_LINK_KEY, recordType, recordId],
    queryFn: async () =>
      unwrap(
        await api.GET("/channel-links/{record_type}/{id}", {
          params: { path: { record_type: recordType, id: recordId } },
        }),
      ),
    retry: false,
  });
  const provider = chosen ?? link.data?.link?.provider ?? "dispact";
  const identity = useMessengerIdentity(provider, Boolean(seat), seat);
  const linked = link.data?.link;
  const conversation =
    destination ??
    (!picking &&
    linked?.provider === provider &&
    (provider !== "dispact" || linked.native_identity === true)
      ? linked.channel_slug
      : "");
  const scope: MessengerScope = {
    provider,
    account: JSON.stringify([
      seat,
      identity.data?.workspace,
      identity.data?.grant_version,
    ]),
    record_type: recordType,
    id: recordId,
  };
  const context = useMessengerConversation(
    scope,
    conversation,
    identity.data?.state === "connected" && conversation !== "" && !picking,
  );
  const ready = !identity.isError && identity.data?.state === "connected";
  const current =
    ready && !context.isError && !picking ? context.data : undefined;
  const title = conversationTitle(current, t("messenger.verb"));
  return (
    <DockedWindow
      title={title || t("messenger.verb")}
      state={dock}
      onStateChange={Dock}
      onClose={onClose}
    >
      <div className="record-messenger">
        <p className="t-caption">{t("messenger.openedFrom", { name })}</p>
        <MessengerProviderToolbar
          provider={provider}
          account={
            <MessengerAccountActions
              provider={provider}
              returnRecord={scope}
              state={identity.data?.state}
              compact
            />
          }
          onProvider={(next) => {
            Chosen(next);
            Destination(null);
            Picking(true);
          }}
          showDestination={Boolean(conversation) && !picking}
          destinationDisabled={!ready}
          onDestination={() => Picking(true)}
        />
        <QueryStates query={identity} pendingLabel={t("messenger.loading")}>
          {identity.data?.state !== "connected" && (
            <MessengerAccountActions
              provider={provider}
              returnRecord={scope}
              state={identity.data?.state}
            />
          )}
          {ready && (
            <MessengerDestination
              scope={scope}
              conversation={conversation}
              picking={picking}
              onChoose={(id) => {
                Destination(id);
                Picking(false);
              }}
              query={context}
              context={current}
              seat={seat}
            />
          )}
        </QueryStates>
      </div>
    </DockedWindow>
  );
}

function conversationTitle(
  context: ReturnType<typeof useMessengerConversation>["data"],
  fallback: string,
) {
  if (!context || context.conversation.kind === "channel") return fallback;
  return (
    context.conversation.members
      .filter((person) => person.id !== context.sender.id)
      .map((person) => person.name)
      .join(", ") || fallback
  );
}

function MessengerProviderToolbar({
  provider,
  onProvider,
  showDestination,
  destinationDisabled,
  account,
  onDestination,
}: Readonly<{
  provider: MessengerProvider;
  onProvider: (value: MessengerProvider) => void;
  showDestination: boolean;
  destinationDisabled: boolean;
  account: ReactNode;
  onDestination: () => void;
}>) {
  const t = useT();
  return (
    <div className="messenger-toolbar">
      <Select
        aria-label={t("messenger.provider")}
        value={provider}
        options={MESSENGER_PROVIDERS.map((item) => ({
          ...item,
          adornment: <ProviderMark providerKey={item.value} />,
        }))}
        onChange={(value) => {
          const next = MESSENGER_PROVIDERS.find((item) => item.value === value);
          if (next) onProvider(next.value);
        }}
      />
      {account}
      {showDestination && (
        <Button
          variant="ghost"
          disabled={destinationDisabled}
          onClick={onDestination}
        >
          {t("messenger.change")}
        </Button>
      )}
    </div>
  );
}

function MessengerDestination({
  scope,
  conversation,
  picking,
  onChoose,
  query,
  context,
  seat,
}: Readonly<{
  scope: MessengerScope;
  conversation: string;
  picking: boolean;
  onChoose: (id: string) => void;
  query: ReturnType<typeof useMessengerConversation>;
  context: ReturnType<typeof useMessengerConversation>["data"];
  seat: string | undefined;
}>) {
  const t = useT();
  if (!conversation || picking)
    return (
      <MessengerPicker key={scope.provider} scope={scope} onChoose={onChoose} />
    );
  return (
    <QueryStates query={query} pendingLabel={t("messenger.loading")}>
      {context && seat && (
        <>
          {context.conversation.kind !== "channel" && (
            <p className="t-caption">{t("messenger.private")}</p>
          )}
          <MessengerMessages
            key={JSON.stringify([
              scope.provider,
              conversation,
              context.grant_version,
            ])}
            scope={scope}
            context={context}
            seat={seat}
          />
        </>
      )}
    </QueryStates>
  );
}
