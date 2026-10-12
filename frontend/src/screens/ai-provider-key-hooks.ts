// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { invalidateProviderHealth } from "./ai-provider-health";
import type { CredentialKind } from "./ai-provider-key-entry";
import { throwProblem, unwrap } from "./common";

// Reading and changing the vendor credentials. No hook here returns a key: the
// server has no read path for one, and neither does this file.

export function useProviderKeys(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["ai-provider-keys"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ai/provider-keys");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Every read whose answer a provider's key decides: whether it is keyed, and
// the locations, models and health that are asked with that key. Not awaited
// by the save: a model list is a call to the vendor, and the save is done first.
function invalidateProviderKeyState(
  queryClient: QueryClient,
  provider: string,
) {
  queryClient.invalidateQueries({ queryKey: ["ai-provider-keys"] });
  queryClient.invalidateQueries({
    queryKey: ["ai-provider-locations", provider],
  });
  queryClient.invalidateQueries({
    queryKey: ["ai-available-models", provider],
  });
  invalidateProviderHealth(queryClient);
}

// Exported for onboarding's AI step, which writes the same credential through
// the same endpoint. A second mutation there would be a second set of rules
// about how long a key lives in memory, and the ones below are not obvious
// enough to expect anybody to rediscover them.
export function useSetProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because what this mutation's
    // `variables` hold is a credential rather than a form field.
    gcTime: 0,
    // The provider AND the key travel as variables rather than closing over
    // render state: a click belongs to the render that drew it, so a value it
    // carries cannot be older than the button.
    //
    // That is also why the caller RESETS this mutation once it settles. React
    // Query keeps `variables` in the mutation's state after success, and for
    // this one mutation the variables are a credential — so what is convenient
    // for every other form is a secret held in memory, readable through the
    // observer and the devtools, until garbage collection gets to it. Passing
    // the key some other way would trade that for a stale-closure refusal,
    // which is the defect the variables rule exists to prevent, so the answer
    // is to keep the variable and drop it early.
    mutationFn: async (vars: {
      provider: string;
      kind: CredentialKind;
      secret: string;
    }) => {
      // The server refuses the field the vendor does not take, so the kind
      // decides which one carries the secret.
      const body =
        vars.kind === "service_account"
          ? { service_account_json: vars.secret }
          : { api_key: vars.secret };
      unwrap(
        await api.PUT("/ai/provider-keys/{provider}", {
          params: { path: { provider: vars.provider } },
          body,
        }),
      );
    },
    onSuccess: (_, vars) => {
      invalidateProviderKeyState(queryClient, vars.provider);
    },
  });
}

export function useRemoveProviderKey() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { provider: string }) => {
      unwrap(
        await api.DELETE("/ai/provider-keys/{provider}", {
          params: { path: { provider: vars.provider } },
        }),
      );
    },
    onSuccess: (_, vars) => {
      invalidateProviderKeyState(queryClient, vars.provider);
    },
  });
}
