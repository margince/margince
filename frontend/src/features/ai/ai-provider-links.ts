// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where each vendor publishes what its models cost, for a reader checking the
// sheet against the source. A vendor missing from here publishes no page worth
// a link (self-hosted servers), and gets none rather than a search.

const OPENROUTER_MODELS = "https://openrouter.ai/models";

const PRICING_PAGES: Readonly<Record<string, string>> = {
  gemini: "https://ai.google.dev/gemini-api/docs/pricing",
  gemini_vertex: "https://cloud.google.com/vertex-ai/generative-ai/pricing",
  anthropic: "https://platform.claude.com/docs/en/about-claude/pricing",
  openai: "https://developers.openai.com/api/docs/pricing",
  jev: "https://openrouter.ai/typesafe/jev-1.13",
  jev_compatible: "https://openrouter.ai/typesafe/jev-1.13",
};

// OpenRouter or one of its subdomains, as the server reads a broker host: a
// bare suffix match would take `notopenrouter.ai` for the broker.
export function isOpenRouter(baseUrl: string): boolean {
  try {
    const url = new URL(baseUrl);
    // The scheme as well as the host, as the server's IsOpenRouterHost reads it:
    // a host it cannot send to is not the broker.
    if (url.protocol !== "https:" && url.protocol !== "http:") return false;
    const host = url.hostname.toLowerCase();
    return host === "openrouter.ai" || host.endsWith(".openrouter.ai");
  } catch {
    return false;
  }
}

/**
 * The page to check a vendor's prices against. An `openai_compatible` vendor is
 * whichever server it is pointed at, so it earns a link only when a binding
 * names OpenRouter, whose catalogue lists every model with its price.
 */
export function pricingPageFor(
  provider: string,
  baseUrls: readonly string[],
): string | undefined {
  if (provider === "openai_compatible") {
    return baseUrls.some(isOpenRouter) ? OPENROUTER_MODELS : undefined;
  }
  return PRICING_PAGES[provider];
}
