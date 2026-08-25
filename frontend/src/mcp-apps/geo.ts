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
 * Why a position could not be read, distinguished by what a reader can DO about
 * it. The browser's own numeric codes do not separate the two cases that matter
 * most: a host that never allowed the frame to ask, and a person who was asked
 * and said no. Both arrive as code 1.
 */
export type GeoRefusal =
  /** The iframe carries no `allow="geolocation"`. Nobody was ever prompted. */
  | "host-blocked"
  /** A prompt was shown and refused, or the OS withheld it. */
  | "user-declined"
  /** The API is not present — an old browser, or a non-secure context. */
  | "unavailable"
  /** Asked and allowed, but no fix arrived in time. */
  | "timeout"
  /** Allowed, but the device could not produce a position. */
  | "position-unavailable";

export type GeoResult =
  | { readonly ok: true; readonly latitude: number; readonly longitude: number; readonly accuracyM: number }
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

/**
 * classify turns a GeolocationPositionError into the distinction that matters.
 *
 * The permissions-policy wording is matched case-insensitively and loosely
 * because it is not specified anywhere — it is what engines happen to say, and
 * Chrome, Firefox and Safari each word it differently. A miss here is not
 * dangerous: it degrades to "user-declined", and `message` still carries the
 * truth for anyone reading the detail.
 */
function classify(err: GeolocationPositionError): GeoRefusal {
  if (err.code === err.TIMEOUT) return "timeout";
  if (err.code === err.POSITION_UNAVAILABLE) return "position-unavailable";
  return /permissions?\s+policy|feature\s+policy|disabled in this document/i.test(err.message)
    ? "host-blocked"
    : "user-declined";
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
      (err) => resolve({ ok: false, refusal: classify(err), code: err.code, message: err.message }),
      { enableHighAccuracy: true, timeout: TIMEOUT_MS, maximumAge: MAX_AGE_MS },
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
    api: typeof navigator !== "undefined" && "geolocation" in navigator ? "present" : "absent",
    secureContext: String(typeof window !== "undefined" && window.isSecureContext),
    framed: String(typeof window !== "undefined" && window.self !== window.top),
  };
  try {
    const permissions = navigator.permissions;
    if (permissions?.query) {
      // The name is a PermissionName the DOM lib types as a union; "geolocation"
      // is a member of it, so this needs no assertion.
      env.permissionState = (await permissions.query({ name: "geolocation" })).state;
    } else {
      env.permissionState = "query-unavailable";
    }
  } catch (e) {
    env.permissionState = `query-threw: ${e instanceof Error ? e.message : String(e)}`;
  }
  return env;
}
