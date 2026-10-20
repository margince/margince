// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";

/**
 * The analytics vocabulary: populations, fields, and the label and choices of
 * every custom field an admin put in reports. A custom field write in Settings
 * makes it stale, so the key lives here, where both sides import it.
 */
export const ANALYTICS_SCHEMA_KEY: QueryKey = ["analytics-schema"];
