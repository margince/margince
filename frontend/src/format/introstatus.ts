// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export const INTRO_STATUS_LABELS: Record<
  components["schemas"]["IntroRequest"]["status"],
  MessageKey
> = {
  requested: "contact.intro.stateRequested",
  accepted: "contact.intro.stateAccepted",
  name_drop_approved: "contact.intro.stateNameDropApproved",
  suggest_other: "contact.intro.stateSuggestOther",
  declined: "contact.intro.stateDeclined",
  introduced: "contact.intro.stateIntroduced",
  name_dropped: "contact.intro.stateNameDropped",
  replied: "contact.intro.stateReplied",
  expired: "contact.intro.stateExpired",
  cancelled: "contact.intro.stateCancelled",
};
