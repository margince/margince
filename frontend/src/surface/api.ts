// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The data surface: the typed client, and the two helpers that turn a failed
// read into the product's own error card.
//
// `api` is the SHARED client, not a per-unit one, and that is the point of
// exporting it rather than letting a unit build its own: it is parameterised
// by the MERGED contract in the composed lane, so a unit calls its own
// /ext/<unit>/… route with the same `api.POST` a core screen uses — no
// wrapper, no cast, and a route the installation does not serve is a type
// error rather than a 404 discovered by a person.
//
// QueryStates and throwProblem are here for the same reason a unit does not
// write its own loading spinner: an installation should not be able to tell
// which screens the core wrote from how they fail.
//
// The other three are one capability, not three, which is why they arrive
// together. A unit that catches a failed WRITE has to say what went wrong
// (problemMessageOf), and has to tell the one failure a person can act on
// from the ones they cannot — a version skew means somebody else edited the
// record, and the answer is "reload and try again" rather than an error card.
// isVersionSkew takes the RFC 7807 body, and the only way to reach that body
// from a caught error is the class, so exporting two of the three publishes a
// helper a unit cannot call. problemMessageOf's MessageKey is already
// published through useT, so this widens no type surface.
export { api } from "../api/client";
// resetToSignedOut is published for the same reason the rest of this block is:
// a unit's screen that polls will one day meet the 401 of a session that ended
// under it, and the answer — drop every cached answer belonging to the member
// who is gone, then reset the shared probe so the auth boundary notices — has
// an ORDER that is wrong in a way nothing catches. A unit spelling it itself
// gets a cache that outlives its member, and the next person to sign in inside
// the cache lifetime is served the previous one's data.
export {
  isVersionSkew,
  ProblemError,
  problemMessageOf,
  QueryStates,
  resetToSignedOut,
  throwProblem,
} from "../screens/common";
