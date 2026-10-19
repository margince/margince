// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

// The MCP URL as the SERVER states it, read from the RFC 9728 document the
// connector serves. It is `--public-base-url` + /mcp, the value clients are
// required to match exactly, so a guide built from it is the command that
// actually works — the SPA's own origin would only coincide with it.
//
// A 404 is not a failure here: the connector is off for this installation
// (`mcp.connector_enabled`), and the whole route group is absent. That is worth
// saying plainly instead of printing steps that cannot connect.
export type ConnectorState =
  | { readonly enabled: true; readonly url: string }
  | { readonly enabled: false };

export async function fetchConnectorState(): Promise<ConnectorState> {
  const response = await fetch("/.well-known/oauth-protected-resource", {
    headers: { accept: "application/json" },
  });
  if (response.status === 404) {
    return { enabled: false };
  }
  if (!response.ok) {
    throw new Error(`discovery answered ${response.status}`);
  }
  const document: unknown = await response.json();
  const resource =
    typeof document === "object" &&
    document !== null &&
    "resource" in document &&
    typeof document.resource === "string"
      ? document.resource
      : "";
  if (resource === "") {
    throw new Error("the discovery document names no resource");
  }
  return { enabled: true, url: resource };
}

export function useConnectorState() {
  return useQuery({
    queryKey: ["mcp-connector-state"],
    queryFn: fetchConnectorState,
  });
}
