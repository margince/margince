// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";
import { INSTALLATION_SETTINGS_KEY } from "../../lib/uploadlimit";

export type AgentClient = components["schemas"]["AgentClient"];
export type AgentClientMethod = components["schemas"]["AgentClientMethod"];
export type AgentClientRegistration =
  components["schemas"]["AgentClientRegistration"];
export type RegisterAgentClientRequest =
  components["schemas"]["RegisterAgentClientRequest"];
export type AgentClientSecret = components["schemas"]["AgentClientSecret"];
export type RegisteredAgentClient =
  components["schemas"]["RegisteredAgentClient"];

type MethodChange = Pick<
  components["schemas"]["UpdateInstallationSettingsRequest"],
  | "dynamic_client_registration"
  | "metadata_document_clients"
  | "mcp_client_check"
>;

// One key for the list and the method switches: both come from the same read.
export const AGENT_CLIENTS_KEY = ["agent-clients"] as const;

export function useAgentClients(enabled: boolean) {
  return useInfiniteQuery({
    enabled,
    queryKey: AGENT_CLIENTS_KEY,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/installation/agent-clients", {
          params: { query: { cursor: pageParam ?? undefined } },
        }),
      ),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
}

function useInvalidateAgentClients() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: AGENT_CLIENTS_KEY });
}

export function useSaveClientMethods() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (change: MethodChange) => {
      unwrap(await api.PATCH("/installation/settings", { body: change }));
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: AGENT_CLIENTS_KEY }),
        queryClient.invalidateQueries({ queryKey: INSTALLATION_SETTINGS_KEY }),
      ]);
    },
  });
}

export function useRegisterAgentClient() {
  const invalidate = useInvalidateAgentClients();
  return useMutation({
    mutationFn: async (body: RegisterAgentClientRequest) =>
      unwrap(await api.POST("/installation/agent-clients", { body })),
    onSuccess: invalidate,
  });
}

export function useSetAgentClientDisabled() {
  const invalidate = useInvalidateAgentClients();
  return useMutation({
    mutationFn: async (change: Readonly<{ id: string; disabled: boolean }>) =>
      unwrap(
        await api.PATCH("/installation/agent-clients/{id}", {
          params: { path: { id: change.id } },
          body: { disabled: change.disabled },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useDeleteAgentClient() {
  const invalidate = useInvalidateAgentClients();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/installation/agent-clients/{id}", {
          params: { path: { id } },
        }),
      );
    },
    onSuccess: invalidate,
  });
}

// Nothing to refresh: the list never carries a secret.
export function useRotateAgentClientSecret() {
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/installation/agent-clients/{id}/secret", {
          params: { path: { id } },
        }),
      ),
  });
}
