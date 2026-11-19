// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";

// The buyer's session is an HttpOnly cookie the credential exchange sets,
// scoped to the buyer API. No script here reads or stores the token; the
// browser sends it, and every call answers the same way once it stops admitting.

// The session stopped answering — revoked, lapsed, or signed out elsewhere.
export class SessionRefusedError extends Error {}

// refuseOrThrow turns a failed public call into the right error: a 401 is the
// session ending (the caller retires it), anything else is the server's own
// explanation. Every buyer write goes through it so none can keep a dead token.
export function refuseOrThrow(
  error: unknown,
  response: Response,
  t: ReturnType<typeof useT>,
): never {
  if (response.status === 401) {
    throw new SessionRefusedError();
  }
  throwProblem(error, t);
  throw new Error("unreachable");
}

export function retireOnRefusal(onSessionLost: () => void) {
  return (error: unknown) => {
    if (error instanceof SessionRefusedError) {
      onSessionLost();
    }
  };
}
