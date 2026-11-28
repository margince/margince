// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { QueryStates } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { useViewerId } from "../lib/session";
import { MessengerAccountActions } from "./messenger/messenger.account";
import {
  type MessengerPage,
  type MessengerScope,
  useMessengerConversation,
  useMessengerIdentity,
  useMessengerMessages,
  useMessengerPage,
} from "./messenger/messenger.queries";

export function ChannelConversationPicker({
  provider,
  recordType,
  recordId,
  channel,
  value,
  onChange,
}: Readonly<{
  provider: components["schemas"]["MessengerProvider"];
  recordType: components["schemas"]["ChannelLinkRecordType"];
  recordId: string;
  channel: string;
  value: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const seat = useViewerId();
  const identity = useMessengerIdentity(provider, Boolean(seat), seat);
  const scope: MessengerScope = {
    provider,
    record_type: recordType,
    id: recordId,
    account: JSON.stringify([
      seat,
      identity.data?.workspace,
      identity.data?.grant_version,
    ]),
  };
  const context = useMessengerConversation(
    scope,
    channel,
    identity.data?.state === "connected",
  );
  return (
    <QueryStates query={identity} pendingLabel={t("messenger.loading")}>
      {identity.data?.state !== "connected" ? (
        <MessengerAccountActions
          provider={provider}
          state={identity.data?.state}
        />
      ) : (
        <QueryStates query={context} pendingLabel={t("messenger.loading")}>
          {context.data && (
            <ConversationChoices
              key={JSON.stringify([scope.account, channel])}
              scope={scope}
              channel={channel}
              value={value}
              onChange={onChange}
            />
          )}
        </QueryStates>
      )}
    </QueryStates>
  );
}

function ConversationChoices({
  scope,
  channel,
  value,
  onChange,
}: Readonly<{
  scope: MessengerScope;
  channel: string;
  value: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const query = useMessengerMessages(scope, channel, "", true);
  const more = useMessengerPage();
  const [pages, Pages] = useState<MessengerPage[]>([]);
  const rows = [
    ...(query.data?.messages ?? []),
    ...pages.flatMap((page) => page.messages),
  ].filter(
    (message, index, all) =>
      !message.deleted &&
      (!message.root_id || message.root_id === message.id) &&
      all.findIndex((item) => item.id === message.id) === index,
  );
  const cursor = pages.length
    ? pages[pages.length - 1]?.next_cursor
    : query.data?.next_cursor;
  const selected = rows.find((message) => message.id === value);
  return (
    <QueryStates query={query} pendingLabel={t("messenger.loading")}>
      <div className="form-stack">
        <Field label={t("channelLink.chooseConversation")}>
          {(control) => (
            <Select
              {...control}
              value={value}
              options={rows.map((message) => ({
                value: message.id,
                label: message.text.slice(0, 120) || t("messenger.empty"),
              }))}
              onChange={onChange}
            />
          )}
        </Field>
        {selected && <p className="messenger-message-text">{selected.text}</p>}
        {cursor && (
          <Button
            pending={more.isPending}
            onClick={() =>
              more.mutate(
                { scope, conversation: channel, root: "", cursor },
                { onSuccess: (page) => Pages((current) => [...current, page]) },
              )
            }
          >
            {t("messenger.loadEarlier")}
          </Button>
        )}
        {more.error && (
          <Callout tone="warning" title={t("messenger.loadProblem")}>
            {problemMessageOf(more.error, t)}
          </Callout>
        )}
      </div>
    </QueryStates>
  );
}
