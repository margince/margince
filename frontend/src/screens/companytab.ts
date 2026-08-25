import type { Route } from "../app/router";

// The account record's tabs, and the ONE way to address one.
//
// They live apart from the page that draws them for the same reason the
// contact's do (screens/persontab.ts): `navigate` takes `id2` as a bare string,
// so a call site that names a tab by hand keeps pointing at an id that no
// longer exists after a rename, and the router falls back to Overview with
// nothing anywhere saying so. `companyTabRoute` is typed on `CompanyTab`, so
// the next rename fails to compile instead.

/**
 * The account's tabs, in the order the strip draws them. URL-addressable, so a
 * tab survives a reload and can be linked to — the same promise the contact
 * page has had, on the record a rep opens most.
 *
 * Partner is in the list but not always on the strip: it renders a commercial
 * arrangement most accounts do not have, and `companyTabsFor` decides. An
 * address naming it is still honoured, which is what keeps the overflow menu's
 * route to a first partner row open.
 */
export const COMPANY_TABS = [
  "overview",
  "people",
  "deals",
  "tasks",
  "timeline",
  "documents",
  "profile",
  "partner",
] as const;

export type CompanyTab = (typeof COMPANY_TABS)[number];

/** isCompanyTab narrows a URL segment, which is any string a reader can type. */
export function isCompanyTab(value: string | undefined): value is CompanyTab {
  return COMPANY_TABS.some((tab) => tab === value);
}

/** companyTabRoute addresses one tab of one account. */
export function companyTabRoute(id: string, tab: CompanyTab): Route {
  return { screen: "companies", id, id2: tab };
}
