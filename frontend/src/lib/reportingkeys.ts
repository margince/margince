// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";

/**
 * The prefix every reporting evaluation is cached under. A write to a figure a
 * report reads, such as a deal's product allocations, invalidates it.
 */
export const REPORTING_EVALUATION_KEY: QueryKey = ["reporting-evaluation"];
