import { useQuery } from "@tanstack/react-query";

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { providerName } from "../../format/connectorprovider";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

type Carriage = components["schemas"]["ChannelProviderEntry"]["attachments"];

// The transport directory (ADR-0107/A158). Which messaging providers exist is a
// DEPLOYMENT fact, so the generated types carry no enum to render from — the
// server answers instead, and this is the one place the app asks.
//
// One cache key for every surface that needs a label, because the answer is
// identical for every caller and changes only on deploy: a second fetch would
// be a second copy of a value that cannot differ.
export function useChannelProviders(enabled = true) {
  return useQuery({
    // Off for a surface that is mounted but not showing — see useVoiceProfile.
    enabled,
    queryKey: ["channel-providers"],
    queryFn: async () => {
      return unwrap(await api.GET("/channel-providers"));
    },
    // It changes only when the server is redeployed, so refetching it on every
    // window focus spends a request to learn nothing.
    staleTime: Number.POSITIVE_INFINITY,
  });
}

// useProviderLabel returns a function that names a transport for a human.
//
// It resolves against BOTH halves of the directory, because a row's connector is
// not always a transport id. A record an extension unit landed on no channel
// carries the natural key's source system instead — `ext:<unit>:<system>` — and
// the same unit's channel messages carry the provider. Resolving only the first
// half is what put "Dispact" and `ext:dispact-connector:dispact` side by side in
// one list: one transport under two spellings, one of them provenance nobody
// outside this repository can parse.
//
// A capture connector (gmail, imap) is in neither half, so the capture names
// answer next; an id nobody names falls back to itself.
export function useProviderLabel(): (provider: string) => string {
  const t = useT();
  const directory = useChannelProviders();
  const named = new Map<string, string>();
  for (const entry of directory.data?.data ?? []) {
    named.set(entry.provider, entry.label);
  }
  // Second, so a transport's own label wins any collision. The two id spaces
  // cannot overlap today — a provider id admits no `:` — and letting the
  // provider win anyway means a future widening changes nothing here.
  for (const entry of directory.data?.capture_sources ?? []) {
    if (!named.has(entry.source)) {
      named.set(entry.source, entry.label);
    }
  }
  return (provider: string) => named.get(provider) ?? providerName(provider, t);
}

// useProviderCarriage returns a function that reads a transport's carriage
// bounds from the directory, or undefined for one the directory does not name.
//
// A selector beside useProviderLabel because both read the ONE directory fetch:
// a second caller of useChannelProviders would be a second copy of an answer
// that changes only on deploy. Mail is deliberately absent — it is not a channel
// provider — so a composer that asks about the mail transport gets undefined and
// falls to the mail path's own limits rather than a channel's.
export function useProviderCarriage(
  enabled = true,
): (provider: string) => Carriage | undefined {
  const directory = useChannelProviders(enabled);
  const byProvider = new Map(
    (directory.data?.data ?? []).map((entry) => [
      entry.provider,
      entry.attachments,
    ]),
  );
  return (provider: string) => byProvider.get(provider);
}
