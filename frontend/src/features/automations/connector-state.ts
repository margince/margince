// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

// The MCP URL as the server states it, from its RFC 9728 document. Clients must
// match it byte for byte; the page's own origin only sometimes equals it. A 404
// means the connector is off, which the page says instead of giving steps.
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
