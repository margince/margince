import type { components } from "../../api/schema";
import type { ContactTab } from "../../lib/contacttab";
import { omitted } from "./contact360";
import { dealCards } from "./contactdeals";

type Contact360 = components["schemas"]["Contact360"];

/**
 * The counts the tab strip may print: only a list read whole. A section the
 * server cut (`has_more`) or withheld knows a floor at best. A floor
 * printed beside a tab name reads as the total, so those tabs carry none.
 * History, Meetings and Network have no whole list on the page to count.
 *
 * A zero is left off: an empty tab reads as an empty tab without a figure.
 *
 * `files` is the number of the contact's files once every page is read, and
 * absent while any page is unread or the read failed.
 */
export function contactTabCounts(
  view: Contact360,
  files: number | undefined,
): Partial<Record<ContactTab, number>> {
  const counts: Partial<Record<ContactTab, number>> = {};
  const roles = view.deal_roles;
  if (roles && !omitted(view, "deal_roles") && !roles.page.has_more) {
    counts.deals = dealCards(view).length;
  }
  if (files !== undefined) {
    counts.documents = files;
  }
  for (const tab of Object.keys(counts) as ContactTab[]) {
    if (counts[tab] === 0) {
      delete counts[tab];
    }
  }
  return counts;
}
