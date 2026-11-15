// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A meeting whose invitation mail History folds onto it says who was invited
// and how they answered, in place of one mail row per copy.

import type { components } from "../api/schema";
import { type Locale, translatePlural } from "../i18n";

type Activity = components["schemas"]["Activity"];

/** "3 invited · 1 accepted · 1 declined"; undefined without a folded invitation. */
export function meetingInvitationLabel(
  activity: Pick<Activity, "invitation">,
  locale: Locale | undefined,
): string | undefined {
  const invitation = activity.invitation;
  if (!locale || !invitation) {
    return undefined;
  }
  const said = (
    base: "invited" | "accepted" | "declined" | "tentative",
    count: number,
  ) =>
    translatePlural(locale, `meeting.invitation.${base}`, count, {
      count: String(count),
    });
  const parts = [said("invited", invitation.invited)];
  for (const answer of ["accepted", "declined", "tentative"] as const) {
    if (invitation[answer] > 0) {
      parts.push(said(answer, invitation[answer]));
    }
  }
  return parts.join(" · ");
}
