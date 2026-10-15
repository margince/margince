// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy, Suspense, useState } from "react";
import { AuthSplash } from "../app/bootsplash";
import { AuthProbeError, type AuthProbeKind } from "../lib/session";
import { ForcedPasswordChangeScreen } from "./forcedpassword";

// Split off: the setup carries the QR encoder, which the sign-in surface must not download.
const ForcedMfaScreen = lazy(() =>
  import("./forcedmfa").then((m) => ({ default: m.ForcedMfaScreen })),
);

type ConfinedKind = Extract<
  AuthProbeKind,
  "must-change-password" | "must-enrol-mfa"
>;

function confinementOf(error: unknown): ConfinedKind | null {
  if (!(error instanceof AuthProbeError)) return null;
  const { kind } = error;
  return kind === "must-change-password" || kind === "must-enrol-mfa"
    ? kind
    : null;
}

/**
 * The confinement the boundary shows in place of login, which would loop on
 * the same correct credentials. It is held until the account resolves it.
 * Confirming a factor lifts the refusal at once. A focus refetch would
 * otherwise swap in the app while the recovery codes are still on screen.
 */
export function useConfinement(
  error: unknown,
  refetch: () => Promise<unknown>,
) {
  const probed = confinementOf(error);
  const [held, setHeld] = useState<ConfinedKind | null>(null);
  const signedOut =
    error instanceof AuthProbeError && error.kind === "unauthorized";
  if (probed !== null && probed !== held) setHeld(probed);
  if (signedOut && held !== null) setHeld(null);
  const kind = probed ?? (signedOut ? null : held);
  const onResolved = async () => {
    await refetch();
    setHeld(null);
  };
  return kind === null ? null : { kind, onResolved };
}

/** The boundary's answer to an account confined until it does one thing. */
export function ConfinedAccountScreen({
  kind,
  onResolved,
}: Readonly<{
  kind: ConfinedKind;
  onResolved: () => void;
}>) {
  if (kind === "must-change-password") {
    return <ForcedPasswordChangeScreen onChanged={onResolved} />;
  }
  return (
    <Suspense fallback={<AuthSplash />}>
      <ForcedMfaScreen onEnrolled={onResolved} />
    </Suspense>
  );
}
