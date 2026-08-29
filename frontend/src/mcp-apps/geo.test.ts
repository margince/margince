// The classifier's whole job is to keep two opposite findings apart, so this is
// where that claim is held.
//
// A geolocation refusal arrives as code 1 whether the HOST never let the frame
// ask or a PERSON was asked and said no. Those mean opposite things: the first
// says the deck's location scenario cannot be built on this host, the second
// says it can and somebody declined. The messages that distinguish them are not
// standardised — every engine words them differently — which is exactly why the
// mapping needs tests rather than confidence.

import { describe, expect, it } from "vitest";

import { classify, READ_OPTIONS } from "./geo";

/** A stand-in for the browser's error object, which cannot be constructed. */
function err(code: number, message: string): GeolocationPositionError {
  return {
    code,
    message,
    PERMISSION_DENIED: 1,
    POSITION_UNAVAILABLE: 2,
    TIMEOUT: 3,
  } as GeolocationPositionError;
}

describe("a refused position", () => {
  // The wording the 2026-08-19 artifact probe actually produced, plus the
  // variants other engines are known to use. This case is the one the view
  // exists to detect.
  it.each([
    "Geolocation has been disabled in this document by permissions policy.",
    "Geolocation has been disabled in this document by Permissions Policy.",
    "Geolocation has been disabled in this document by Feature Policy.",
    "Access to the feature is disallowed by permissions policy",
  ])("is a host block when the message says %j", (message) => {
    expect(classify(err(1, message))).toBe("host-blocked");
  });

  it.each([
    "User denied Geolocation",
    "User denied geolocation prompt",
    "Origin does not have permission to use the Geolocation service: denied by the user",
  ])("is a decline when the message says %j", (message) => {
    expect(classify(err(1, message))).toBe("user-declined");
  });

  // THE REGRESSION THIS FILE WAS WRITTEN FOR. An earlier version defaulted every
  // unmatched code-1 message to "user-declined", so an unfamiliar host block
  // told the tester "the permission got through, try again and accept" — the
  // opposite of the truth, stated confidently, about the one question the probe
  // exists to answer. An unknown wording has to read as unknown.
  it.each(["Position acquisition failed", "kCLErrorDomain error 1", ""])(
    "is unclassified rather than a guess when the message says %j",
    (message) => {
      expect(classify(err(1, message))).toBe("refused-unclassified");
    },
  );

  // The two codes that are NOT about permission at all. Reading either as a
  // refusal would send somebody looking for a host setting over a device that
  // simply had no fix.
  it("is a timeout when the code says so, whatever the message", () => {
    expect(classify(err(3, "User denied Geolocation"))).toBe("timeout");
  });

  it("is position-unavailable when the code says so, whatever the message", () => {
    expect(classify(err(2, "disabled by permissions policy"))).toBe(
      "position-unavailable",
    );
  });
});

// The probe collects less than it is allowed to.
//
// What this view produces is whether the frame was ALLOWED to ask, and in whose
// words when it was not — the refusal branch is the whole of the meaning table
// it feeds. A network-derived fix answers that identically to a satellite one,
// so asking for the precise one would obtain maximum-precision coordinates
// inside a third-party chat host to establish a fact that needs no coordinate,
// and wake a phone's GNSS radio to do it.
describe("what the read asks for", () => {
  it("does not ask for a GPS-grade fix to answer a yes/no question", () => {
    expect(READ_OPTIONS.enableHighAccuracy).toBe(false);
  });

  // The other two are the reason the read still terminates and still refreshes:
  // a bound on the wait, and a bound on how stale a cached answer may be. They
  // are asserted as PRESENT rather than at a value, since neither number is a
  // claim this case is making.
  it("still bounds the wait and the staleness", () => {
    expect(READ_OPTIONS.timeout).toBeGreaterThan(0);
    expect(READ_OPTIONS.maximumAge).toBeGreaterThan(0);
  });
});
