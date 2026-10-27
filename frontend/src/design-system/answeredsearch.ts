// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext } from "react";

/**
 * The search a list's rows answer, handed by `ListTable` to a body it draws in
 * place of its grid. It lags the search box while the typing settles, so a
 * board checks its matches against the cards that search found.
 */
export const AnsweredSearch = createContext<string | undefined>(undefined);
