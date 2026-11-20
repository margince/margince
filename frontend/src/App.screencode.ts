// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { opensReservedPage } from "./app/reservedpages";
import type { Route, Screen } from "./lib/router";

// Each screen's code by its module. bootstrap.ts imports this, so it names
// modules only through `import()` and imports nothing that draws.
export const SCREEN_CODE = {
  analytics: () => import("./App.analytics"),
  book: () => import("./features/booking/book"),
  brief: () => import("./features/brief/brief"),
  buyerRoom: () => import("./features/deals/buyerroom"),
  client: () => import("./features/settings/client"),
  companies: () => import("./features/companies/companies"),
  confirm: () => import("./features/privacy/confirm"),
  contactPage: () => import("./features/contacts/contactpage"),
  contacts: () => import("./features/contacts/contacts"),
  dealRoom: () => import("./features/deals/dealroompage"),
  deals: () => import("./features/deals/deals"),
  filters: () => import("./features/lists/filters"),
  goals: () => import("./features/analytics/goals"),
  leads: () => import("./features/leads/leads"),
  legalReviews: () => import("./features/privacy/legalreviews"),
  oauthConsent: () => import("./features/automations/oauthconsent"),
  offers: () => import("./features/deals/offers"),
  onboarding: () => import("./features/onboarding/onboarding"),
  partners: () => import("./features/projects/partners"),
  preferences: () => import("./features/settings/preferences"),
  progress: () => import("./features/analytics/progress"),
  project: () => import("./features/projects/project360"),
  projects: () => import("./features/projects/projects"),
  scheduled: () => import("./features/mail/scheduledsends"),
  search: () => import("./features/voice-knowledge/search"),
  settings: () => import("./App.settings"),
  share: () => import("./features/records/share"),
  tags: () => import("./features/lists/tagresult"),
  unsubscribe: () => import("./features/privacy/unsubscribe"),
  worklist: () => import("./features/brief/brief.queue"),
} as const;

type Code = () => Promise<unknown>;

// Total over `Screen`, so a new screen fails the build until it names its code.
const ADDRESSED: Readonly<Record<Screen, (route: Route) => Code | undefined>> =
  {
    home: () => SCREEN_CODE.brief,
    contacts: (route) =>
      route.id === undefined || opensReservedPage(route)
        ? SCREEN_CODE.contacts
        : SCREEN_CODE.contactPage,
    companies: () => SCREEN_CODE.companies,
    partners: () => SCREEN_CODE.partners,
    leads: () => SCREEN_CODE.leads,
    deals: (route) =>
      route.id !== undefined &&
      !opensReservedPage(route) &&
      route.id2 === "room"
        ? SCREEN_CODE.dealRoom
        : SCREEN_CODE.deals,
    projects: (route) =>
      route.id === undefined || opensReservedPage(route)
        ? SCREEN_CODE.projects
        : SCREEN_CODE.project,
    worklist: () => SCREEN_CODE.worklist,
    analytics: () => SCREEN_CODE.analytics,
    progress: () => SCREEN_CODE.progress,
    goals: () => SCREEN_CODE.goals,
    settings: () => SCREEN_CODE.settings,
    filters: () => SCREEN_CODE.filters,
    scheduled: () => SCREEN_CODE.scheduled,
    offers: (route) => (route.id ? SCREEN_CODE.offers : undefined),
    "legal-reviews": () => SCREEN_CODE.legalReviews,
    search: () => SCREEN_CODE.search,
    share: (route) => (route.id && route.id2 ? SCREEN_CODE.share : undefined),
    tags: () => SCREEN_CODE.tags,
    lists: () => SCREEN_CODE.filters,
    onboarding: () => SCREEN_CODE.onboarding,
    client: () => SCREEN_CODE.client,
    book: () => SCREEN_CODE.book,
    preferences: () => SCREEN_CODE.preferences,
    unsubscribe: () => SCREEN_CODE.unsubscribe,
    confirm: () => SCREEN_CODE.confirm,
    room: () => SCREEN_CODE.buyerRoom,
    "oauth-consent": () => SCREEN_CODE.oauthConsent,
    "reset-password": () => undefined,
    ext: () => undefined,
    x: () => undefined,
    "not-found": () => undefined,
  };

/** The code of the screen `route` draws, or undefined when the shell draws it. */
export function addressedScreenCode(route: Route): Code | undefined {
  return ADDRESSED[route.screen](route);
}
