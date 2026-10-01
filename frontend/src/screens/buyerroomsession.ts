// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../app/storage";
import type { useT } from "../i18n";
import { throwProblem } from "./common";

// The buyer's session, as this tab holds it: the token the credential exchange
// issued, kept in sessionStorage and presented as a Bearer on every call, and
// the one way every call answers when that token has stopped admitting them.

export function readSession(): string | null {
  return readStored(STORAGE_KEYS.buyerRoomSession);
}

// A browser refusing storage still gets this one page view: the token lives in
// React state for the tab's lifetime and is simply not kept.
export function writeSession(token: string | null): void {
  if (token === null) {
    removeStored(STORAGE_KEYS.buyerRoomSession);
  } else {
    writeStored(STORAGE_KEYS.buyerRoomSession, token);
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
