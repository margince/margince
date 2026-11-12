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
 * The tabs, in strip order, each URL-addressable. History holds both what was
 * said and what changed. Companies mirrors the account's Contacts tab.
 * Network is drawn as "Relationships" and keeps its id so old links hold.
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
