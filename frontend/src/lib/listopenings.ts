// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The order each list screen opens on. Here and not in the screens, so code
// that reads a list's address from outside it reads the same opening.

export const CONTACTS_INITIAL_SORT = "-created_at";
export const COMPANIES_INITIAL_SORT = "-created_at";
export const PROJECTS_INITIAL_SORT = "-last_activity_at";
/** Empty: the leads queue opens in the server's own order. */
export const LEADS_INITIAL_SORT = "";
