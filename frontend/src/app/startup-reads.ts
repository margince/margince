// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { onboardingProgressOptions } from "../features/onboarding/onboarding-progress";
import { companyContextCapabilitiesOptions } from "../lib/companycontextreach";
import { sessionOptions } from "../lib/session";
import { installationSettingsOptions } from "../lib/uploadlimit";

/**
 * Asks, all at once, the reads the shell holds its splash for. Each lands under
 * the options its hook reads, so the hook takes the answer and does not ask.
 */
export function prefetchStartupReads(client: QueryClient): void {
  void client.prefetchQuery(sessionOptions);
  void client.prefetchQuery(installationSettingsOptions);
  void client.prefetchQuery(companyContextCapabilitiesOptions);
  void client.prefetchQuery(onboardingProgressOptions);
}
