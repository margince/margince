// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMe } from "../lib/session";

export const ME_ANSWERED = "me answered";

/** Drawn once /me has answered, so a control absent after it was refused, not still waiting. */
export function MeAnswered() {
  return useMe().data ? <p>{ME_ANSWERED}</p> : null;
}
