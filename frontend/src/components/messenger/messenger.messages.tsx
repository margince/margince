// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLayoutEffect, useRef, useState } from "react";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryStates } from "../../design-system/querystates";
import { stable } from "../../format/collate";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { MessengerAudience } from "./messenger.audience";
import { MessengerComposer } from "./messenger.composer";
import { MessengerOutcome } from "./messenger.outcome";
import {
  type MessengerContext,
  type MessengerPage,
  type MessengerScope,
  useMessengerConversation,
  useMessengerMessages,
  useMessengerPage,
} from "./messenger.queries";

export function MessengerMessages({
  scope,
  context,
  seat,
}: Readonly<{
  scope: MessengerScope;
  context: MessengerContext;
  seat: string;
}>) {
  const [reviewed, Reviewed] = useState(context.audience_key);
  const [root, Root] = useState("");
  const current = useMessengerConversation(
    scope,
    context.conversation.id,
    false,
  );
  const query = useMessengerMessages(
    scope,
    context.conversation.id,
    root,
    true,
  );
  return (
    <div className="messenger-conversation">
      <MessengerAudience
        scope={scope}
        context={context}
        refreshing={query.isFetching || current.isFetching}
        onRefresh={() => {
          void query.refetch();
          void current.refetch();
        }}
        changed={reviewed !== context.audience_key}
        onReview={() => Reviewed(context.audience_key)}
      />
      <MessengerOutcome scope={scope} />
      <MessengerThread
        key={root}
        scope={scope}
        context={context}
        seat={seat}
        root={root}
        onRoot={Root}
        audienceReviewed={reviewed === context.audience_key}
        query={query}
      />
    </div>
  );
}

function MessengerThread({
  scope,
  context,
  seat,
  root,
  onRoot,
  audienceReviewed,
  query,
}: Readonly<{
  scope: MessengerScope;
  context: MessengerContext;
  seat: string;
  root: string;
  onRoot: (id: string) => void;
  audienceReviewed: boolean;
  query: ReturnType<typeof useMessengerMessages>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const conversation = context.conversation;
  const pagination = useMessengerPage();
  const [frozen, Frozen] = useState<MessengerPage | null>(null);
  const [pages, Pages] = useState<MessengerPage[]>([]);
  const box = useRef<HTMLDivElement>(null);
  const anchor = useRef<{ height: number; top: number } | null>(null);
  const snapshot = frozen ?? query.data;
  const messages = [
    ...pages.flatMap((page) => page.messages),
    ...(snapshot?.messages ?? []),
  ]
    .filter(
      (message, index, all) =>
        all.findIndex((item) => item.id === message.id) === index,
    )
    .sort((a, b) => Date.parse(a.at) - Date.parse(b.at) || stable(a.id, b.id));
  const cursor = pages.length
    ? pages[pages.length - 1]?.next_cursor
    : snapshot?.next_cursor;
  const later = snapshot?.cursor_direction === "newer";
  useLayoutEffect(() => {
    const node = box.current;
    if (!node) return;
    if (anchor.current) {
      node.scrollTop =
        anchor.current.top + node.scrollHeight - anchor.current.height;
      anchor.current = null;
      return;
    }
    if (!frozen && messages.length) node.scrollTop = node.scrollHeight;
  }, [frozen, messages]);
  const loadMore = () => {
    if (!cursor || !snapshot) return;
    Frozen(snapshot);
    pagination.mutate(
      { scope, conversation: conversation.id, root, cursor },
      {
        onSuccess: (page) => {
          const node = box.current;
          if (node && !later)
            anchor.current = { height: node.scrollHeight, top: node.scrollTop };
          Pages((pages) => [...pages, page]);
        },
      },
    );
  };
  const draftKey = JSON.stringify([
    seat,
    scope.provider,
    context.workspace,
    context.sender.id,
    scope.record_type,
    scope.id,
    conversation.id,
    root,
  ]);
  return (
    <div className="messenger-conversation">
      {root && (
        <Button onClick={() => onRoot("")}>{t("messenger.back")}</Button>
      )}
      <QueryStates query={query} pendingLabel={t("messenger.loading")}>
        {!query.isError && (
          <div
            className="messenger-transcript"
            ref={box}
            onScroll={(event) => {
              const node = event.currentTarget;
              if (
                !frozen &&
                query.data &&
                node.scrollHeight - node.scrollTop - node.clientHeight > 48
              )
                Frozen(query.data);
            }}
          >
            {cursor && !later && (
              <Button pending={pagination.isPending} onClick={loadMore}>
                {t("messenger.loadEarlier")}
              </Button>
            )}
            {messages.length === 0 && <p>{t("messenger.empty")}</p>}
            {messages.map((message) => (
              <article
                key={message.id}
                className={
                  message.author === context.sender.id
                    ? "messenger-message is-own"
                    : "messenger-message"
                }
              >
                <div className="t-caption messenger-message-meta">
                  {conversation.members.find(
                    (person) => person.id === message.author,
                  )?.name ?? t("messenger.authorUnavailable")}
                  <time dateTime={message.at}>
                    {formatDateTime(message.at, locale, zone)}
                  </time>
                </div>
                <p className="messenger-message-text">
                  {message.deleted ? t("messenger.deleted") : message.text}
                </p>
                {message.edited && <small>{t("messenger.edited")}</small>}
                {(conversation.supports_threads ??
                  conversation.kind === "channel") &&
                  !root &&
                  !message.deleted && (
                    <Button
                      onClick={() => onRoot(message.root_id || message.id)}
                    >
                      {t("messenger.reply")}
                    </Button>
                  )}
              </article>
            ))}
            {cursor && later && (
              <Button pending={pagination.isPending} onClick={loadMore}>
                {t("messenger.loadLater")}
              </Button>
            )}
          </div>
        )}
      </QueryStates>
      {pagination.error && (
        <Callout tone="warning" title={t("messenger.loadProblem")}>
          {problemMessageOf(pagination.error, t)}
        </Callout>
      )}
      {frozen && query.data !== frozen && (
        <Button
          onClick={() => {
            Frozen(null);
            Pages([]);
          }}
        >
          {t("messenger.newMessages")}
        </Button>
      )}
      {!query.isError &&
      conversation.audience_known &&
      !conversation.external &&
      audienceReviewed ? (
        <MessengerComposer
          key={draftKey}
          scope={scope}
          context={context}
          root={root}
          draftKey={draftKey}
          onSent={() => {
            Frozen(null);
            Pages([]);
            void query.refetch();
          }}
        />
      ) : (
        audienceReviewed &&
        !conversation.audience_known && <p>{t("messenger.audienceUnknown")}</p>
      )}
    </div>
  );
}
