// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLayoutEffect, useRef, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { NoteMarkdown } from "../../design-system/markdown";
import { QueryStates } from "../../design-system/querystates";
import { stable } from "../../format/collate";
import { formatDateTime } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemCodeOf, problemMessageOf } from "../../lib/problem";
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
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
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
    scope.provider === "slack" &&
      context.conversation.supports_outcome_draft === false
      ? 60000
      : 15000,
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
      {query.dataUpdatedAt > 0 && (
        <p className="t-caption">
          {t("messenger.lastChecked", {
            at: formatDateTime(
              new Date(query.dataUpdatedAt).toISOString(),
              locale,
              zone,
            ),
          })}
        </p>
      )}
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
  const conversation = context.conversation;
  const pagination = useMessengerPage();
  const [frozen, Frozen] = useState<MessengerPage | null>(null);
  const [pages, Pages] = useState<MessengerPage[]>([]);
  const box = useRef<HTMLDivElement>(null);
  const anchor = useRef<{ height: number; top: number } | null>(null);
  const snapshot = frozen ?? query.data;
  const retainSnapshot =
    Boolean(snapshot) &&
    problemCodeOf(query.error) === "messenger_rate_limited";
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
      <MessengerOutcome
        scope={scope}
        conversation={conversation.id}
        root={root}
        supportsDraft={conversation.supports_outcome_draft}
        messageIds={messages
          .filter((message) => !message.deleted)
          .slice(-100)
          .map((message) => message.id)}
      />
      {root && (
        <Button onClick={() => onRoot("")}>{t("messenger.back")}</Button>
      )}
      {query.isError && retainSnapshot && (
        <Callout tone="warning" title={t("messenger.loadProblem")}>
          {problemMessageOf(query.error, t)}
        </Callout>
      )}
      <QueryStates
        query={{ ...query, isError: query.isError && !retainSnapshot }}
        pendingLabel={t("messenger.loading")}
      >
        {(!query.isError || retainSnapshot) && (
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
              <MessengerMessageRow
                key={message.id}
                message={message}
                context={context}
                root={root}
                onRoot={onRoot}
              />
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
      <MessengerWriting
        scope={scope}
        context={context}
        root={root}
        draftKey={draftKey}
        readable={!query.isError || retainSnapshot}
        reviewed={audienceReviewed}
        onSent={() => {
          Frozen(null);
          Pages([]);
          void query.refetch();
        }}
      />
    </div>
  );
}

function MessengerMessageRow({
  message,
  context,
  root,
  onRoot,
}: Readonly<{
  message: MessengerPage["messages"][number];
  context: MessengerContext;
  root: string;
  onRoot: (root: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const conversation = context.conversation;
  return (
    <article
      className={
        message.author === context.sender.id
          ? "messenger-message is-own"
          : "messenger-message"
      }
    >
      <div className="t-caption messenger-message-meta">
        {conversation.members.find((person) => person.id === message.author)
          ?.name ?? t("messenger.authorUnavailable")}
        <time dateTime={message.at}>
          {formatDateTime(message.at, locale, zone)}
        </time>
      </div>
      <div className="messenger-message-text">
        <NoteMarkdown
          source={message.deleted ? t("messenger.deleted") : message.text}
        />
      </div>
      {message.content_incomplete && !message.deleted && (
        <small>{t("messenger.incompleteContent")}</small>
      )}
      {message.source_url && /^https:\/\//.test(message.source_url) && (
        <AppLink href={message.source_url}>{t("messenger.openSource")}</AppLink>
      )}
      {message.edited && <small>{t("messenger.edited")}</small>}
      {(conversation.supports_threads ?? conversation.kind === "channel") &&
        !root &&
        !message.deleted && (
          <Button onClick={() => onRoot(message.root_id || message.id)}>
            {message.reply_count
              ? plural("messenger.replyCount", message.reply_count)
              : t("messenger.reply")}
          </Button>
        )}
    </article>
  );
}

function MessengerWriting({
  scope,
  context,
  root,
  draftKey,
  readable,
  reviewed,
  onSent,
}: Readonly<{
  scope: MessengerScope;
  context: MessengerContext;
  root: string;
  draftKey: string;
  readable: boolean;
  reviewed: boolean;
  onSent: () => void;
}>) {
  const t = useT();
  if (!readable || !reviewed) return null;
  if (!context.conversation.audience_known)
    return <p>{t("messenger.audienceUnknown")}</p>;
  if (context.conversation.external) return null;
  return (
    <MessengerComposer
      key={draftKey}
      scope={scope}
      context={context}
      root={root}
      draftKey={draftKey}
      onSent={onSent}
    />
  );
}
