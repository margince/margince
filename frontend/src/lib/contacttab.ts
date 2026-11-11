import type { RecordTabId } from "./recordtaborder";
import type { Route } from "./router";

// The contact record's tabs, and the ONE way to address one.
//
// They live apart from the page that draws them because three other surfaces
// send a reader to a tab — the header's verbs and the rail's "view all" — and
// `navigate` takes `id2` as a bare string. When the tab set last changed, two
// of those call sites kept pointing at ids that no longer existed: the router
// fell back to Overview and nothing anywhere said so. `contactTabRoute` is
// typed on `ContactTab`, so the next rename fails to compile instead.

/**
 * The tabs, in the concept's order (§5.4). URL-addressable, so a tab
 * survives a reload and can be linked to.
 *
 * Activity and History are ONE tab, as they are on the account page: what was
 * said to a contact and what was changed about them are one chronology to the
 * contact reading them, and the filter above the list is what separates them.
 *
 * Companies is where they work and worked, the mirror of the account's
 * Contacts tab. Network, drawn as "People", follows: who else this contact
 * reaches. It is a tab rather than an overview card because its
 * answer is a working surface — a reader interrogates the routes and the
 * seats — and the overview stack is a glance.
 */
export const CONTACT_TABS = [
  "overview",
  "timeline",
  "companies",
  "network",
  "deals",
  "meetings",
  "research",
  "documents",
] as const satisfies readonly RecordTabId[];

export type ContactTab = (typeof CONTACT_TABS)[number];

/** isContactTab narrows a URL segment, which is any string a reader can type. */
export function isContactTab(value: string | undefined): value is ContactTab {
  return CONTACT_TABS.some((tab) => tab === value);
}

/** contactTabRoute addresses one tab of one contact. */
export function contactTabRoute(id: string, tab: ContactTab): Route {
  return { screen: "contacts", id, id2: tab };
}
