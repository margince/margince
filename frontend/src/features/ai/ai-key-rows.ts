// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A vendor whose key is optional calls without one, so an absent key there is
// no warning to draw. A provider that sends another row's key (`sent_by`) is
// unkeyed with that row.
export function unkeyedProviders(
  providers:
    | readonly {
        provider: string;
        configured: boolean;
        optional: boolean;
        sent_by?: readonly string[];
      }[]
    | undefined,
): ReadonlySet<string> | null {
  if (!providers) {
    return null;
  }
  return new Set(
    providers
      .filter((p) => !p.configured && !p.optional)
      .flatMap((p) => [p.provider, ...(p.sent_by ?? [])]),
  );
}

/**
 * The key row a provider's calls send: its own, or the row that names it in
 * `sent_by`. Undefined for a provider that takes no key.
 */
export function keyRowFor<
  K extends { provider: string; sent_by?: readonly string[] },
>(provider: string, keys: readonly K[] | undefined): K | undefined {
  return keys?.find(
    (k) => k.provider === provider || (k.sent_by ?? []).includes(provider),
  );
}
