// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { connectorsPollInterval } from "./capture-progress";
import { problemCode, throwProblem } from "./problem";

type CaptureConnection = components["schemas"]["CaptureConnection"];

export type ConnectorsResult = {
  // A 501 not_implemented is a deployment that never wired mail capture: a
  // documented feature-off state, never an error card.
  notConfigured: boolean;
  data: CaptureConnection[];
  providers: ProviderReadiness[];
  // Absent when none is configured, which is itself the answer.
  publicOrigin?: PublicOriginStatus;
};

type PublicOriginStatus =
  components["schemas"]["CaptureConnectionListResponse"]["public_origin"];

type ProviderReadiness = NonNullable<
  components["schemas"]["CaptureConnectionListResponse"]["providers"]
>[number];

// Readers share the connector query shape because its cache entry is shared.
export function useConnectors(
  options?: Readonly<{
    /** False holds the request without leaving the key: a reader that only
     *  wants the answer when it already exists still shares the one entry. */
    enabled?: boolean;
    refetchOnWindowFocus?: boolean | "always";
  }>,
) {
  return useQuery({
    ...options,
    queryKey: ["connectors"],
    enabled: options?.enabled ?? true,
    queryFn: async (): Promise<ConnectorsResult> => {
      const { data, error, response } = await api.GET("/connectors");
      if (response.status === 501 && problemCode(error) === "not_implemented") {
        return { notConfigured: true, data: [], providers: [] };
      }
      if (error) {
        throwProblem(error);
      }
      return {
        notConfigured: false,
        data: data.data,
        providers: data.providers ?? [],
        publicOrigin: data.public_origin,
      };
    },
    // Live while a mailbox imports: the orb, the capture chip and the import
    // dialog all draw the run from this read.
    refetchInterval: (query) => connectorsPollInterval(query.state.data),
  });
}
