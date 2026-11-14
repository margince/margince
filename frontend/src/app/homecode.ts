// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Starts Home's code on a cold load on Home. The route's own import in
 * App.screens.tsx names the same module, so both share one fetch.
 */
export const loadHome = () => import("../features/brief/brief");
