// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { isMailbox } from "../format/connectorprovider";
import type { MessageKey } from "../i18n/en";
import { connectorsPollInterval } from "./capture-progress";
import { problemCode, throwProblem } from "./problem";
import { captureRefusalCopy } from "./problemcodecopy";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type CaptureMissing = components["schemas"]["SeatCapture"]["missing"];

export type ConnectorsResult = {
  // A 501 not_implemented is a deployment that never wired mail capture: a
  // documented feature-off state, never an error card.
  notConfigured: boolean;
  data: CaptureConnection[];
  providers: ProviderReadiness[];
  /** False when the reader's role or seat cannot capture: no connect is
   *  offered, because the server refuses every one. */
  captureAllowed: boolean;
  // The grant the reader lacks, so the refusal can name it.
  captureMissing?: CaptureMissing;
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
    refreshHealth?: boolean;
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
        return {
          notConfigured: true,
          data: [],
          providers: [],
          captureAllowed: true,
        };
      }
      if (error) {
        throwProblem(error);
      }
      return {
        notConfigured: false,
        data: data.data,
        providers: data.providers ?? [],
        // A server that does not report the seat yet refuses nothing up front.
        captureAllowed: data.seat_capture?.allowed ?? true,
        captureMissing: data.seat_capture?.missing,
        publicOrigin: data.public_origin,
      };
    },
    // Live while a mailbox imports: the orb, the capture chip and the import
    // dialog all draw the run from this read.
    refetchInterval: (query) =>
      connectorsPollInterval(query.state.data) ||
      (options?.refreshHealth ? 60_000 : false),
  });
}

/**
 * The sentence that stands in for a connect button the server would refuse,
 * or null while connecting is allowed or the roster has not answered.
 */
export function captureRefusalOf(
  data: ConnectorsResult | undefined,
): MessageKey | null {
  if (!data || data.captureAllowed) return null;
  return captureRefusalCopy(data.captureMissing);
}

/**
 * Whether this reader has a mailbox the product can send from: a connected
 * mail connection, not a calendar. Unanswered reads as none, because a drawer
 * offered on a guess would open onto a refusal.
 */
export function useMailboxConnected(): boolean {
  const connectors = useConnectors();
  return (connectors.data?.data ?? []).some(
    (connection) =>
      connection.status === "connected" && isMailbox(connection.provider),
  );
}
