// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useT } from "../i18n";
import { throwProblem } from "./common";

// The buyer's session, as this tab holds it: the token the credential exchange
// issued, kept in sessionStorage and presented as a Bearer on every call, and
// the one way every call answers when that token has stopped admitting them.

const SESSION_KEY = "margince.room.session";

export function readSession(): string | null {
  try {
    return globalThis.sessionStorage?.getItem(SESSION_KEY) ?? null;
  } catch {
    return null;
  }
}

export function writeSession(token: string | null): void {
  try {
    if (token === null) {
      globalThis.sessionStorage?.removeItem(SESSION_KEY);
    } else {
      globalThis.sessionStorage?.setItem(SESSION_KEY, token);
    }
  } catch {
    // A browser refusing storage still gets this one page view: the token
    // lives in React state for the tab's lifetime and is simply not kept.
  }
}

export function bearer(token: string): { headers: { Authorization: string } } {
  return { headers: { Authorization: `Bearer ${token}` } };
}

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
