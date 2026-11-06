// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Route, routeHash, type Screen } from "./router";

/**
 * How many leading segments of an address name the thing on screen. A deeper
 * segment picks a tab of it, so moving between tabs keeps the screen mounted.
 *
 * Only screens with a tab in the address go below `WHOLE_ADDRESS`. `Record`
 * makes every screen pick a depth. A flow step or a second record id is not a
 * tab and keeps the whole address.
 */
const WHOLE_ADDRESS = 4;

const IDENTITY_DEPTH: Readonly<Record<Screen, number>> = {
  home: WHOLE_ADDRESS,
  // #/contacts/<contact>/<tab>: the contact tabs are views of one contact.
  contacts: 2,
  // #/companies/<account>/<tab>: the account tabs are views of one account.
  companies: 2,
  partners: WHOLE_ADDRESS,
  // #/leads/<lead>/<tab>: the lead tabs are views of one lead.
  leads: 2,
  deals: WHOLE_ADDRESS,
  projects: WHOLE_ADDRESS,
  // #/worklist/<owner> reaches the screen as `opensOn`, read in useState
  // initialisers, so only a remount applies a new owner. A lower depth shows
  // the manager's own queue under a colleague's address.
  // features/worklist/worklist.identity.test.tsx holds this.
  worklist: WHOLE_ADDRESS,
  // #/analytics/<section>: each section is a view of one screen.
  analytics: 1,
  // Not a tab, however much the sidebar looks like one: every settings entry is
  // its own page, and the admin half is a segment deeper.
  settings: WHOLE_ADDRESS,
  filters: WHOLE_ADDRESS,
  scheduled: WHOLE_ADDRESS,
  offers: WHOLE_ADDRESS,
  "legal-reviews": WHOLE_ADDRESS,
  search: WHOLE_ADDRESS,
  // Each tag is its own page. A remount starts a fresh read, so one tag's
  // counts never show under another tag's name.
  tags: WHOLE_ADDRESS,
  lists: WHOLE_ADDRESS,
  share: WHOLE_ADDRESS,
  onboarding: WHOLE_ADDRESS,
  client: WHOLE_ADDRESS,
  book: WHOLE_ADDRESS,
  preferences: WHOLE_ADDRESS,
  // Token and purpose together name the page: one reader may hold links for
  // two kinds of mail.
  unsubscribe: WHOLE_ADDRESS,
  confirm: WHOLE_ADDRESS,
  room: WHOLE_ADDRESS,
  "oauth-consent": WHOLE_ADDRESS,
  "reset-password": WHOLE_ADDRESS,
  ext: WHOLE_ADDRESS,
  // The key in `#/x/<key>` picks the fork screen, so two keys must remount.
  // One depth serves every fork screen. A fork screen with tabs keeps the tab
  // in its own state, so it never needs an entry here.
  x: WHOLE_ADDRESS,
  "not-found": WHOLE_ADDRESS,
};

/**
 * The address without its tab segments. Built with `routeHash`, so a short
 * address is spelled the same way as a whole one.
 */
export function routeIdentity(route: Route): string {
  const depth = IDENTITY_DEPTH[route.screen];
  return routeHash({
    screen: route.screen,
    id: depth > 1 ? route.id : undefined,
    id2: depth > 2 ? route.id2 : undefined,
    id3: depth > 3 ? route.id3 : undefined,
  });
}
