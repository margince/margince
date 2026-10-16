import { useMemo } from "react";
import { useHash } from "./router";
import { parseParams } from "./urlstate";

/**
 * How a unit OAuth app's consent ended. The server's consent callback
 * redirects to `#/ext/<unit>?oauth=<outcome>&provider=<provider>`, and these are
 * the outcomes it sends (`GET /ext-oauth/{provider}/callback` in crm.yaml).
 */
export const OAUTH_RETURN_OUTCOMES = [
  "ok",
  "denied",
  "rejected",
  "bad_client",
  "error",
] as const;

export type OAuthReturnOutcome = (typeof OAUTH_RETURN_OUTCOMES)[number];

export type OAuthReturn = Readonly<{
  outcome: OAuthReturnOutcome;
  provider: string;
}>;

function isOutcome(value: string): value is OAuthReturnOutcome {
  return (OAUTH_RETURN_OUTCOMES as readonly string[]).includes(value);
}

/**
 * The consent outcome `hash` carries, or null when it carries none. An outcome
 * outside the set reads as none, so a hand-edited address cannot make a screen
 * claim a connection that never happened.
 */
export function oauthReturnOf(hash: string): OAuthReturn | null {
  const params = parseParams(hash);
  const outcome = params.get("oauth");
  const provider = params.get("provider");
  if (outcome === undefined || provider === undefined || !isOutcome(outcome)) {
    return null;
  }
  return { outcome, provider };
}

/** The consent outcome the current address carries, for a unit's screen. */
export function useOAuthReturn(): OAuthReturn | null {
  const hash = useHash();
  return useMemo(() => oauthReturnOf(hash), [hash]);
}
