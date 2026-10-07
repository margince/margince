// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ENTITY } from "../app/entity";
import { routeHash } from "../app/router";
import type { WorklistItem } from "./worklist.queries";

type T = (
  key: "worklist.meeting.hostedBy" | "worklist.meeting.hostedByYou",
  values?: { name: string },
) => string;

/**
 * Who hosted the meeting a row is about. The reader's own meeting says so
 * without a name; a colleague's says their name; a host whose name the server
 * withheld is not drawn, because the server said the reader may not have it.
 */
export function hostText(
  item: WorklistItem,
  viewer: string | undefined,
  t: T,
): string | null {
  const host = item.host;
  if (!host?.id) return null;
  if (host.id === viewer) return t("worklist.meeting.hostedByYou");
  return host.label
    ? t("worklist.meeting.hostedBy", { name: host.label })
    : null;
}

/** The account the row's contact works for, linked, where the server sent one. */
export function employerOf(
  contact: NonNullable<WorklistItem["contact"]>,
): { href: string; label: string } | undefined {
  const employer = contact.employer;
  if (!employer) return undefined;
  return {
    href: routeHash(ENTITY.company.route(employer.company_id)),
    label: employer.company_name,
  };
}
