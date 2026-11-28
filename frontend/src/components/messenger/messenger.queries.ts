// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessengerProvider } from "../../lib/messagingproviders";
import { problemCode, unwrap } from "../../lib/problem";
import {
  applyMessengerAttempt,
  reserveMessengerRetry,
} from "./messenger.draft";

export type { MessengerProvider } from "../../lib/messagingproviders";
export type MessengerContext = components["schemas"]["MessengerContext"];
export type MessengerPage = components["schemas"]["MessengerPage"];
export type MessengerMember = components["schemas"]["MessengerMember"];
export type MessengerScope = Readonly<{
  provider: MessengerProvider;
  record_type: components["schemas"]["ChannelLinkRecordType"];
  id: string;
  account?: string;
}>;
export { MESSENGER_PROVIDERS } from "../../lib/messagingproviders";

type MessengerAttempt = components["schemas"]["MessengerAttempt"];

function bindAttempt(
  result: MessengerAttempt,
  request: string,
  conversation?: string,
): MessengerAttempt {
  if (
    result.request_id === request &&
    (conversation === undefined || result.conversation_id === conversation) &&
    (result.state !== "accepted" || Boolean(result.provider_message_id))
  )
    return result;
  return {
    ...result,
    request_id: request,
    conversation_id: conversation ?? "",
    provider_message_id: "",
    state: "unknown",
    failure_code: "delivery_unknown",
    retry_at: undefined,
  };
}

type SendRequest = components["schemas"]["MessengerSendRequest"];

export function messengerPath(scope: MessengerScope) {
  return {
    path: {
      provider: scope.provider,
      record_type: scope.record_type,
      id: scope.id,
    },
  };
}
export function useMessengerIdentity(
  provider: MessengerProvider,
  enabled: boolean,
  seat?: string,
) {
  return useQuery({
    queryKey: ["messenger-identity", seat, provider],
    queryFn: async () =>
      unwrap(
        await api.GET("/messenger/{provider}/identity", {
          params: { path: { provider } },
        }),
      ),
    enabled,
    retry: false,
  });
}
export function useMessengerConversation(
  scope: MessengerScope,
  conversation: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["messenger-context", scope, conversation],
    queryFn: async () =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/conversation", {
          params: {
            ...messengerPath(scope),
            query: { conversation_id: conversation },
          },
        }),
      ),
    enabled,
    retry: false,
    staleTime: 15000,
  });
}
export function useMessengerMessages(
  scope: MessengerScope,
  conversation: string,
  root: string,
  enabled: boolean,
  pollingInterval = 15000,
) {
  return useQuery({
    queryKey: ["messenger-messages", scope, conversation, root],
    queryFn: async () =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/messages", {
          params: {
            ...messengerPath(scope),
            query: {
              conversation_id: conversation,
              root_id: root || undefined,
            },
          },
        }),
      ),
    enabled,
    retry: false,
    refetchInterval: () =>
      document.visibilityState === "visible" ? pollingInterval : false,
  });
}
export function useMessengerSend() {
  const client = useQueryClient();
  return useMutation({
    onSettled: (_result, _error, variables) =>
      client.invalidateQueries({
        queryKey: [
          "messenger-context",
          variables.scope,
          variables.body.conversation_id,
        ],
      }),
    mutationFn: async ({
      scope,
      body,
      draftKey,
    }: Readonly<{
      scope: MessengerScope;
      body: SendRequest;
      draftKey: string;
    }>) => {
      const response = await api.POST(
        "/messenger/{provider}/{record_type}/{id}/messages",
        { params: messengerPath(scope), body },
      );
      if (response.response.status === 429) {
        const seconds = Number(
          response.response.headers.get("Retry-After") ?? "60",
        );
        const hold =
          Number.isFinite(seconds) && seconds >= 1 && seconds <= 86400
            ? seconds
            : 60;
        reserveMessengerRetry(
          draftKey,
          body.request_id,
          new Date(Date.now() + hold * 1000).toISOString(),
        );
      }
      if (problemCode(response.error) === "messenger_delivery_needs_review") {
        applyMessengerAttempt(draftKey, body.request_id, { state: "failed" });
      }
      const result = bindAttempt(
        unwrap(response),
        body.request_id,
        body.conversation_id,
      );
      applyMessengerAttempt(draftKey, body.request_id, result);
      return result;
    },
  });
}
export function useMessengerReceipt() {
  return useMutation({
    mutationFn: async ({
      scope,
      request,
      draftKey,
    }: Readonly<{
      scope: MessengerScope;
      request: string;
      draftKey: string;
    }>) => {
      const result = bindAttempt(
        unwrap(
          await api.GET(
            "/messenger/{provider}/{record_type}/{id}/attempts/{request_id}",
            { params: { path: { ...scope, request_id: request } } },
          ),
        ),
        request,
      );
      applyMessengerAttempt(draftKey, request, result);
      return result;
    },
  });
}
export function useMessengerPage() {
  return useMutation({
    mutationFn: async ({
      scope,
      conversation,
      root,
      cursor,
    }: Readonly<{
      scope: MessengerScope;
      conversation: string;
      root: string;
      cursor: string;
    }>) =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/messages", {
          params: {
            ...messengerPath(scope),
            query: {
              conversation_id: conversation,
              root_id: root || undefined,
              cursor,
            },
          },
        }),
      ),
  });
}
export function useMessengerOpenPeople() {
  return useMutation({
    mutationFn: async ({
      scope,
      members,
    }: Readonly<{ scope: MessengerScope; members: string[] }>) =>
      unwrap(
        await api.POST(
          "/messenger/{provider}/{record_type}/{id}/conversation",
          { params: messengerPath(scope), body: { members } },
        ),
      ),
  });
}

export function useMessengerConnect() {
  return useMutation({
    mutationFn: async ({
      provider,
      returnRecord,
    }: Readonly<{
      provider: MessengerProvider;
      returnRecord?: Pick<MessengerScope, "record_type" | "id">;
    }>) =>
      unwrap(
        await api.POST("/messenger/{provider}/connect", {
          params: { path: { provider } },
          body: returnRecord
            ? {
                record_type: returnRecord.record_type,
                record_id: returnRecord.id,
              }
            : undefined,
        }),
      ),
  });
}

export function useMessengerSetup(provider: MessengerProvider) {
  return useQuery({
    queryKey: ["messenger-setup", provider],
    queryFn: async () =>
      unwrap(
        await api.GET("/messenger/{provider}/setup", {
          params: { path: { provider } },
        }),
      ),
    retry: false,
  });
}

export function useMessengerTeam(scope: MessengerScope) {
  return useQuery({
    queryKey: ["messenger-team", scope],
    queryFn: async () =>
      unwrap(
        await api.GET("/messenger/{provider}/{record_type}/{id}/team", {
          params: messengerPath(scope),
        }),
      ),
    retry: false,
  });
}
