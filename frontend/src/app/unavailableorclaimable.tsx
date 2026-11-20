// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { fetchSetupStatus } from "../features/sign-in/setupclaim.queries";
import { AvailabilityScreen, SetupClaimScreen } from "./authscreens";

// Splits the boundary's 503 into its two product states. An installation that
// holds no company and waits to be claimed (ADR-0105) gets the claim screen.
// Anything else keeps the availability message.
//
// Only the installation kind probes: a connection failure says nothing about a
// claim. The availability screen stands while the probe is in flight.
export function UnavailableOrClaimable({
  kind,
  onRetry,
}: Readonly<{ kind: "connection" | "installation"; onRetry: () => void }>) {
  const claimable = useQuery({
    queryKey: ["setup-status"],
    queryFn: fetchSetupStatus,
    enabled: kind === "installation",
    retry: false,
  });
  // Retry probes both. fetchSetupStatus resolves rather than throws, so a
  // one-off failure caches `false`. Without a refetch, "Try again" would never
  // show the claim screen to an installation that can be claimed.
  const retryBoth = () => {
    onRetry();
    if (kind === "installation") {
      void claimable.refetch();
    }
  };
  if (kind === "installation" && claimable.data?.claimable) {
    // A claim provisions the installation, so the /v1/me probe now answers.
    // The boundary decides where to go next, not this screen.
    return <SetupClaimScreen onClaimed={onRetry} status={claimable.data} />;
  }
  return <AvailabilityScreen kind={kind} onRetry={retryBoth} />;
}
