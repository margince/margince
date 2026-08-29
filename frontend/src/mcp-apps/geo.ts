// Reading the device's position from inside a view, and — far more often —
// finding out precisely why it could not be read.
//
// WHY THIS EXISTS AT ALL. A rep hands over a business card at a conference. The
// useful thing is not the card, it is that the person filing it is standing in
// the hall: the venue is the tag, and nobody should have to type it. Every other
// route to that fact either asks the user (which makes it a form, not a
// convenience) or guesses from an IP address (which on conference wifi names the
// wrong city). The device already knows.
//
// WHY IT IS SEPARATE FROM bridge.ts. The bridge is the transport, and it is the
// one thing every view depends on to render at all. This is a sensor read that
// usually fails, on a permission the host is free to refuse. Keeping it apart
// means a view that never asks carries none of it, and a change here cannot
// break the handshake.
//
// THE PERMISSION IS DECLARED SERVER-SIDE, in apps.sandbox() — a view asks for
// geolocation in the `_meta.ui` its resource is published with, and the host
// turns that into an iframe `allow` attribute IF it chooses to. The extension is
// explicit that a host "MAY honor these permissions… but are not required to",
// so refusal is a normal outcome and not an error to report as a fault.
//
// WHAT A CALLER MAY ASSUME: nothing. There is no position until there is one,
// this never throws, and it never rejects. A view that renders differently
// depending on whether it got an answer has already made the answer required.

/** How long to wait before giving up on a fix. */
const TIMEOUT_MS = 15_000;

/** How stale a cached position may be before a fresh fix is required. */
const MAX_AGE_MS = 60_000;

/**
 * What the read asks the device for. Named rather than inline so the one
 * decision in it can be asserted: `enableHighAccuracy` stays FALSE, because
 * the question is whether the frame may ask at all and a coarse fix answers
 * that identically (readPosition says the rest).
 */
export const READ_OPTIONS: Readonly<PositionOptions> = {
  enableHighAccuracy: false,
  timeout: TIMEOUT_MS,
  maximumAge: MAX_AGE_MS,
};

/**
 * Why a position could not be read, distinguished by what a reader can DO about
 * it. The browser's own numeric codes do not separate the two cases that matter
 * most: a host that never allowed the frame to ask, and a person who was asked
 * and said no. Both arrive as code 1.
 */
export type GeoRefusal =
  /** The iframe carries no `allow="geolocation"`. Nobody was ever prompted. */
  | "host-blocked"
  /** A prompt was shown and refused, and the message says so. */
  | "user-declined"
  /**
   * Refused with a code-1 message this code does not recognise.
   *
   * It is a state of its own rather than a default, and that is the point. Code
   * 1 covers a permissions-policy block, an insecure context, a person
   * declining, AND a persistent OS-level denial, and the messages are not
   * standardised — every engine words them differently. Folding an unfamiliar
   * message into "user-declined" would tell a tester the permission got through
   * and to try again, which is the exact OPPOSITE conclusion when the truth was
   * a host block, and retrying cannot help. Saying "unrecognised" is worth more
   * than a confident guess: `message` carries the evidence either way.
   */
  | "refused-unclassified"
  /** The API is not present — an old browser, or a non-secure context. */
  | "unavailable"
  /** Asked and allowed, but no fix arrived in time. */
  | "timeout"
  /** Allowed, but the device could not produce a position. */
  | "position-unavailable";

export type GeoResult =
  | {
      readonly ok: true;
      readonly latitude: number;
      readonly longitude: number;
      readonly accuracyM: number;
    }
  | {
      readonly ok: false;
      readonly refusal: GeoRefusal;
      /**
       * The browser's own message, verbatim and untrimmed.
       *
       * This is the field the whole module exists to produce. "Geolocation has
       * been disabled in this document by permissions policy" is a finding about
       * the HOST; "User denied Geolocation" is a finding about a person. They
       * are the same numeric code and they mean opposite things, so the string
       * is the evidence and paraphrasing it destroys it.
       */
      readonly message: string;
      /** The browser's numeric code, or -1 where no error object existed. */
      readonly code: number;
    };

/** Engine wordings that mean the frame was never allowed to ask. */
const HOST_BLOCKED =
  /permissions?\s+policy|feature\s+policy|disabled in this document/i;

/** Engine wordings that mean a person was asked and refused. */
const USER_DECLINED = /user\s+denied|denied by (the )?user|user\s+declined/i;

/**
 * classify turns a GeolocationPositionError into the distinction that matters.
 *
 * BOTH sides are matched, and anything else is "refused-unclassified" rather
 * than a guess. Code 1 is PERMISSION_DENIED for four different reasons — a
 * permissions-policy block, an insecure context, a person declining, a
 * persistent OS denial — and the messages are not standardised. An earlier
 * version defaulted an unmatched message to "user-declined", which would have
 * told a tester "the permission got through, try again and accept" for a host
 * block where retrying can never work: the opposite of the truth, stated
 * confidently. Two positive matchers and an explicit unknown cannot do that.
 *
 * The patterns are what engines happen to say rather than anything specified,
 * so they will need extending. `message` is carried verbatim either way, which
 * is what makes an unrecognised wording a finding rather than a dead end.
 */
export function classify(err: GeolocationPositionError): GeoRefusal {
  if (err.code === err.TIMEOUT) return "timeout";
  if (err.code === err.POSITION_UNAVAILABLE) return "position-unavailable";
  if (HOST_BLOCKED.test(err.message)) return "host-blocked";
  if (USER_DECLINED.test(err.message)) return "user-declined";
  return "refused-unclassified";
}

/**
 * readPosition asks the device where it is.
 *
 * It resolves either way and never rejects, so a caller writes no try/catch and
 * cannot accidentally make a refused permission look like a crash. Calling it
 * more than once is fine; each call is an independent request.
 *
 * NOTE ON THE PROMPT. Where the host does allow it, the browser may show a
 * permission prompt on the first call. That is the user's decision to make and
 * the reason this is never called on load — a view that asks the moment it
 * renders spends the one prompt it gets before anybody wanted the feature.
 *
 * AND IT ASKS FOR THE COARSE FIX. The finding this module exists to produce is
 * WHETHER the frame was allowed to ask, and if not, in whose words — a
 * yes/no that a network-derived position answers exactly as well as a
 * satellite one. `enableHighAccuracy: true` would obtain the most precise
 * coordinates the device can produce, inside a third-party chat host, to
 * establish a fact that needs no coordinate at all; on a phone it also wakes
 * the GNSS radio for it. Least privilege is the default until a view needs
 * otherwise, and this one does not.
 */
export function readPosition(): Promise<GeoResult> {
  return new Promise<GeoResult>((resolve) => {
    if (typeof navigator === "undefined" || !("geolocation" in navigator)) {
      resolve({
        ok: false,
        refusal: "unavailable",
        code: -1,
        message: "navigator.geolocation is not present in this document",
      });
      return;
    }
    // A settled promise ignores later calls, so the guard is not for
    // correctness — it is so a late success cannot look like a second answer to
    // anyone reading this and wondering.
    navigator.geolocation.getCurrentPosition(
      (pos) =>
        resolve({
          ok: true,
          latitude: pos.coords.latitude,
          longitude: pos.coords.longitude,
          accuracyM: pos.coords.accuracy,
        }),
      (err) =>
        resolve({
          ok: false,
          refusal: classify(err),
          code: err.code,
          message: err.message,
        }),
      READ_OPTIONS,
    );
  });
}

/**
 * describeEnvironment reports what this document can see about its own sandbox,
 * WITHOUT asking for a position.
 *
 * It is here because the first question about a failed read is always whether
 * the frame was ever in a position to succeed, and every field below is
 * available before any prompt. `permissions.query` is the one that answers
 * "would it even ask?" — but it is unimplemented in some engines and throws for
 * an unsupported name in others, so it is treated as best-effort.
 */
export async function describeEnvironment(): Promise<Record<string, string>> {
  const env: Record<string, string> = {
    api:
      typeof navigator !== "undefined" && "geolocation" in navigator
        ? "present"
        : "absent",
    secureContext: String(
      typeof window !== "undefined" && window.isSecureContext,
    ),
    framed: String(typeof window !== "undefined" && window.self !== window.top),
  };
  try {
    const permissions = navigator.permissions;
    if (permissions?.query) {
      // The name is a PermissionName the DOM lib types as a union; "geolocation"
      // is a member of it, so this needs no assertion.
      env.permissionState = (
        await permissions.query({ name: "geolocation" })
      ).state;
    } else {
      env.permissionState = "query-unavailable";
    }
  } catch {
    // The thrown message is deliberately NOT shown. `permissions.query` throws
    // for a name an engine does not implement, and every such message is about
    // the query rather than about geolocation — so it would be an engine's own
    // words on the screen saying nothing a reader can act on. That it could not
    // be asked IS the whole finding, and the real answer comes from the read.
    env.permissionState = "query-threw";
  }
  return env;
}
