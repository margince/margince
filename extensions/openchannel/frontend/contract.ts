// What the screen and its parts agree on: the shape the unit's operations
// answer with, the RBAC objects those operations gate on, and the cache keys
// the two writes invalidate.
//
// It is a module rather than a section of screen.tsx because three components
// read the same endpoint row and two of them invalidate the same query key. A
// key spelled twice is a card that stops refreshing after a write, and nothing
// fails when it happens.

/**
 * One member's inbound endpoint, exactly as the unit's operations answer.
 *
 * `ref` is an ADDRESS and not a second secret: it travels in the request path,
 * so it reaches every access log and proxy between a sender and this
 * installation. Only the signing secret admits a request.
 */
export type Endpoint = {
  id: string;
  user_id: string;
  slug: string;
  ref: string;
  url: string;
  enabled: boolean;
  inbound_received: number;
  outbound_sent: number;
  last_inbound_at?: string;
  last_outbound_at?: string;
  version: number;
};

/** One request that arrived on the member's own edge. */
export type InboundEntry = {
  id: string;
  nonce: string;
  state: string;
  attempts: number;
  last_error_class?: string;
  body_bytes: number;
  sent_at: string;
  received_at: string;
};

/** One message this connector posted to the member's registered address. */
export type OutboundAttempt = {
  id: string;
  delivery_key: string;
  attempt: number;
  recipient: string;
  outcome: string;
  error_class?: string;
  created_at: string;
};

/**
 * The three objects the unit's operations gate on.
 *
 * Three rather than one because they are three decisions an operator makes
 * separately: who may open a path into this installation, who may read what
 * arrived on it, and who may read what left.
 */
export const ENDPOINT_OBJECT = "ext_openchannel_endpoint";
export const INBOUND_OBJECT = "ext_openchannel_inbound";
export const OUTBOUND_OBJECT = "ext_openchannel_outbound";

/** The cache key each read is held under, spelled once. */
export const ENDPOINT_KEY = ["ext", "openchannel", "endpoint"];
export const INBOUND_KEY = ["ext", "openchannel", "inbound"];
export const OUTBOUND_KEY = ["ext", "openchannel", "outbound"];

/**
 * How often the endpoint and the two listings re-read while the screen is open.
 *
 * A member who has just pasted the curl is WAITING for a row, so this is the
 * one screen in the unit where a stale list is the failure the reader notices.
 * It is nonetheless slower than the drain's own minute cadence would reward,
 * because what the arrival changes first is the inbound row — which lands at
 * the moment of the request, not at the moment of the drain.
 */
export const POLL_MS = 15_000;

/**
 * The last path segment of `/webhooks/ext/openchannel/<slug>/<ref>` is minted
 * per member; the two before it are this unit's name and its declared slug.
 * The slug arrives on the endpoint row rather than being spelled here, because
 * the server is what declares it.
 */
export function inboundUrl(origin: string, endpoint: Endpoint): string {
  return `${origin}/webhooks/ext/openchannel/${endpoint.slug}/${endpoint.ref}`;
}
