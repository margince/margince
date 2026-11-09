import { api } from "../api/client";
import { problemMessage } from "./problem";

// Confined sessions need a different remedy from signing in again.
export type AuthProbeKind =
  | "unauthorized"
  | "connection"
  | "installation"
  | "must-change-password"
  | "must-enrol-mfa";

export class AuthProbeError extends Error {
  readonly kind: AuthProbeKind;
  constructor(kind: AuthProbeKind, message: string) {
    super(message);
    this.name = "AuthProbeError";
    this.kind = kind;
  }
}

function probeKindFor(status: number, code?: string): AuthProbeKind {
  if (status === 503) return "installation";
  if (status >= 500) return "connection";
  if (status === 403 && code === "password_change_required") {
    return "must-change-password";
  }
  if (status === 403 && code === "mfa_enrolment_required") {
    return "must-enrol-mfa";
  }
  return "unauthorized";
}

function codeInProblemBody(error: unknown): string | undefined {
  if (typeof error === "object" && error !== null && "code" in error) {
    const { code } = error as { code?: unknown };
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

export async function readSession(signal?: AbortSignal, prepare?: "analytics") {
  const result = await api
    .GET("/me", { signal, params: { query: { prepare } } })
    .catch(() => null);
  if (!result) {
    throw new AuthProbeError("connection", "the API could not be reached");
  }
  const { data, error, response } = result;
  if (error) {
    throw new AuthProbeError(
      probeKindFor(response.status, codeInProblemBody(error)),
      problemMessage(error),
    );
  }
  if (!data?.user) {
    throw new AuthProbeError("connection", "malformed /me response");
  }
  return data;
}

export async function readPreparedAnalyticsSession(signal?: AbortSignal) {
  const data = await readSession(signal, "analytics");
  return { data, dataUpdatedAt: Date.now() };
}
