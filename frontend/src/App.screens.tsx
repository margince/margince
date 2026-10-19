import { lazy } from "react";
import { findCustomScreen } from "./app/custom";
import { opensReservedPage } from "./app/nav";
import {
  Card,
  EmptyState,
  PendingBody,
  SectionHeader,
} from "./design-system/atoms";
import { AuthScreen } from "./features/sign-in/auth";
import { useLocale, useT } from "./i18n";
import type { MessageKey } from "./i18n/en";
import { composedScreens, findExtension, unitTitle } from "./lib/extensions";
import { navigate } from "./lib/router";

// Static imports are what a first paint needs: the shell, the auth surfaces and
// the router. screens/onboarding is static too, since the shell calls its
// company probe. A module imported statically anywhere lands in the entry chunk
// whatever a dynamic import says, so splitting it costs a round-trip for nothing.
// Every route chunk, registered as it is declared. A screen is split so the
// login screen does not carry the app. It is warmed once a session exists, so a
// reader past that point never pays the fetch on a click. Splitting alone moves
// the cost from first paint onto the first visit to each screen. That visit is
// when a record open is measured. Wrapping the factory keeps the two lists one
// list: a screen cannot be added to the router and forgotten here.
export const ROUTE_CHUNKS: Array<() => Promise<unknown>> = [];

// Registers a screen's chunk for warming and hands the factory back unchanged,
// so `lazy` keeps the loading and this keeps the list. A warmed chunk still
// suspends on its first render: `lazy` calls its factory and throws the promise
// whatever the module cache holds. So warming alone does not make a navigation
// cheap. ScreenView deferring the route does, since it keeps React from
// committing the fallback it would then have to hold on screen.
function routed<T>(factory: () => Promise<T>): () => Promise<T> {
  ROUTE_CHUNKS.push(factory);
  return factory;
}

export const BookingScreen = lazy(
  routed(() =>
    import("./features/booking/book").then((m) => ({
      default: m.BookingScreen,
    })),
  ),
);

export const ClientSurfaceScreen = lazy(
  routed(() =>
    import("./screens/client").then((m) => ({
      default: m.ClientSurfaceScreen,
    })),
  ),
);

const DealScreen = lazy(
  routed(() =>
    import("./features/deals/deals").then((m) => ({ default: m.DealScreen })),
  ),
);

const DealsScreen = lazy(
  routed(() =>
    import("./features/deals/deals").then((m) => ({ default: m.DealsScreen })),
  ),
);

const DealRoomPage = lazy(
  routed(() =>
    import("./features/deals/dealroompage").then((m) => ({
      default: m.DealRoomPage,
    })),
  ),
);

export const FiltersScreen = lazy(
  routed(() =>
    import("./features/lists/filters").then((m) => ({
      default: m.FiltersScreen,
    })),
  ),
);

export const BriefScreen = lazy(
  routed(() =>
    import("./features/brief/brief").then((m) => ({ default: m.BriefScreen })),
  ),
);

export const LeadScreen = lazy(
  routed(() =>
    import("./features/leads/leads").then((m) => ({ default: m.LeadScreen })),
  ),
);

export const LeadsScreen = lazy(
  routed(() =>
    import("./features/leads/leads").then((m) => ({ default: m.LeadsScreen })),
  ),
);

export const ProjectScreen = lazy(
  routed(() =>
    import("./screens/project360").then((m) => ({
      default: m.ProjectScreen,
    })),
  ),
);

export const ProjectsScreen = lazy(
  routed(() =>
    import("./screens/projects").then((m) => ({
      default: m.ProjectsScreen,
    })),
  ),
);

export const OAuthConsent = lazy(
  routed(() =>
    import("./features/automations/oauthconsent").then((m) => ({
      default: m.OAuthConsent,
    })),
  ),
);

export const OfferScreen = lazy(
  routed(() =>
    import("./features/deals/offers").then((m) => ({ default: m.OfferScreen })),
  ),
);

export const CompaniesScreen = lazy(
  routed(() =>
    import("./features/companies/companies").then((m) => ({
      default: m.CompaniesScreen,
    })),
  ),
);

export const CompanyScreen = lazy(
  routed(() =>
    import("./features/companies/companies").then((m) => ({
      default: m.CompanyScreen,
    })),
  ),
);

export const PartnersScreen = lazy(
  routed(() =>
    import("./screens/partners").then((m) => ({ default: m.PartnersScreen })),
  ),
);

export const ContactsScreen = lazy(
  routed(() =>
    import("./features/contacts/contacts").then((m) => ({
      default: m.ContactsScreen,
    })),
  ),
);

export const ContactPageV2 = lazy(
  routed(() =>
    import("./features/contacts/contactpage").then((m) => ({
      default: m.ContactPageV2,
    })),
  ),
);

export const BuyerRoomScreen = lazy(() =>
  import("./features/deals/buyerroom").then((m) => ({
    default: m.BuyerRoomScreen,
  })),
);

export const PreferenceCenterScreen = lazy(
  routed(() =>
    import("./screens/preferences").then((m) => ({
      default: m.PreferenceCenterScreen,
    })),
  ),
);

export const UnsubscribeScreen = lazy(
  routed(() =>
    import("./features/privacy/unsubscribe").then((m) => ({
      default: m.UnsubscribeScreen,
    })),
  ),
);

export const ConfirmDetailsScreen = lazy(
  routed(() =>
    import("./features/privacy/confirm").then((m) => ({
      default: m.ConfirmDetailsScreen,
    })),
  ),
);

export const AnalyticsScreen = lazy(
  routed(() =>
    import("./features/analytics/analytics").then((m) => ({
      default: m.AnalyticsScreen,
    })),
  ),
);

export const LegalReviewsRoute = lazy(
  routed(() =>
    import("./features/privacy/legalreviews").then((m) => ({
      default: m.LegalReviewsRoute,
    })),
  ),
);

export const ScheduledSendsScreen = lazy(
  routed(() =>
    import("./features/mail/scheduledsends").then((m) => ({
      default: m.ScheduledSendsScreen,
    })),
  ),
);

export const TagResultScreen = lazy(() =>
  import("./features/lists/tagresult").then((m) => ({
    default: m.TagResultScreen,
  })),
);

export const SearchRoute = lazy(
  routed(() =>
    import("./screens/search").then((m) => ({ default: m.SearchRoute })),
  ),
);

export const SettingsScreen = lazy(
  routed(() =>
    import("./screens/settings").then((m) => ({ default: m.SettingsScreen })),
  ),
);

const ShareScreen = lazy(
  routed(() =>
    import("./screens/share").then((m) => ({ default: m.ShareScreen })),
  ),
);

export const WorklistRedirect = lazy(() =>
  import("./features/brief/brief.queue").then((m) => ({
    default: m.WorklistRedirect,
  })),
);

// safeDecode tolerates malformed percent-encoding, such as a stray "%2" from a
// hand-edited hash route. decodeURIComponent throws a URIError on bad escapes.
// A route param is untrusted input, so a decode failure falls back to the raw
// string rather than crashing the render.
export function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

// What stands in the content column when the address names no page, or names
// one half written. Both are statements about the address. A small card is the
// right shape for a statement, since it is all there is to say.
export function ScreenNotice({
  messageKey,
}: Readonly<{ messageKey: MessageKey }>) {
  const t = useT();
  return (
    <div className="wrap">
      <EmptyState>{t(messageKey)}</EmptyState>
    </div>
  );
}

// What stands there while a screen's code is still arriving, a different fact.
// A small card like the notices above (about 90px) would sit in an empty
// column on the first visit to a screen. A whole page would then replace it.
// A reserved placeholder exists to prevent that pop and reflow, and this path
// reaches every reader.
//
// Eight lines, the reservation ceiling: nobody knows which screen is coming, so
// this cannot match its shape. Between a card far too small and a column of
// roughly the right size, the column is the truer choice. It says "a page is
// arriving" rather than "here is a page with one sentence on it".
export function ScreenPending() {
  const t = useT();
  return (
    <div className="wrap">
      <PendingBody label={t("common.loading")} lines={8} />
    </div>
  );
}

// Split out of the dispatch table to keep the deals list/detail split in one
// place. `#/deals/new` is the reserved segment for the list with its form open.
// The shell's layout policy reads it the same way.
export function DealsRoute({
  id,
  id2,
}: Readonly<{ id?: string; id2?: string }>) {
  if (!id || opensReservedPage({ screen: "deals", id })) {
    return <DealsScreen startCreating={id !== undefined} />;
  }
  return id2 === "room" ? <DealRoomPage dealId={id} /> : <DealScreen id={id} />;
}

// #/share/<record_type>/<record_id> (AS-3/4/5): both segments are required.
// A bare #/share renders the pending state instead of a screen with nothing to
// share.
export function ShareRoute({
  id,
  id2,
}: Readonly<{ id?: string; id2?: string }>) {
  return id && id2 ? (
    <ShareScreen recordType={id} recordId={id2} />
  ) : (
    <ScreenNotice messageKey="screen.pending" />
  );
}

// The reset form itself, so the two places that answer #/reset-password render
// one form. The first is `App`, ahead of the session gate and inside the
// pre-session frame. The second is the routed table, which is total over
// `Screen` and so has an entry for every address, this one included.
//
// A stale or bare reset link has no token, so the embedded form renders as an
// ordinary login rather than ResetForm. LoginForm's onSuccess restores the
// originally requested route, so it never navigates to the Brief for a
// non-empty hash, and this hash is never empty. Without a navigate here, a
// sign-in from this route would leave the reader signed in but still looking
// at a login form.
export function ResetRoute() {
  return <AuthScreen onAuthed={() => navigate({ screen: "home" })} />;
}

// #/ext/<unit> (ADR-0120): the composed extension tier's one route into the
// SPA. The registry is generated per installation, so this arm is the same
// code in the vanilla tree. There every unit name misses and the not-found card
// renders. That lane is the default one and must never crash or paint a blank
// frame.
//
// A unit surface comes from one of two places, in this order:
//
//   1. The unit's own screen. extensions/<name>/frontend/ is a pnpm workspace
//      package (@margince-ext/<name>) whose default export is a component.
//      gen-composition lists it in the composed screen registry
//      ("@composition/screens"). notes, the reference extension, is the one
//      such screen today. This is unit-authored TSX compiled into the SPA
//      bundle. So collectUnitFrontend refuses a package that is not private,
//      is mis-named, or takes a direct dependency on a hosted framework. And
//      check-ext-imports.sh gates what such a screen may import.
//   2. Otherwise the contract-derived descriptor set: the operations the
//      unit's fragments published. That is all the app can say about a unit
//      nobody wrote a screen for (de, crm-hello).
//
// The registry is consulted only after the descriptor resolves. So a screen
// cannot render for a unit this installation did not compose. An entry for a
// disabled unit is inert rather than a route into a surface with no server
// behind it.
// The generated registry is emitted untyped, and the annotation lands here.
//
// That file is written to two locations at different depths: the vanilla stub
// under src/composition/ and the composed output under build/composition/.
// stubMatchesVanilla requires the two to be byte-identical. So the file can
// carry no relative type import, since the paths would differ. It can carry no
// bare one either, since nothing resolves from build/composition/. Annotating
// at the import site costs nothing and moves the check rather than losing it.
// The type check on each unit's default export runs here, in core, in the
// composed lane. This is the one place both halves of the registry are visible.
const extensionScreens = composedScreens;

// A fork's own screen, at `#/x/<key>`.
//
// Rendered from the registry rather than from a case in the dispatch above.
// The seam exists so that a fork adds no case here. A miss renders the same
// not-found the router gives a mistyped address. A key nothing declares is not
// a page, and an empty frame would read as one that failed to load.
export function CustomRoute({ name }: Readonly<{ name?: string }>) {
  const screen = findCustomScreen(name);
  if (!screen) {
    return <ScreenNotice messageKey="shell.unknownPage" />;
  }
  const ForkScreen = screen.component;
  return <ForkScreen />;
}

export function ExtensionRoute({ name }: Readonly<{ name?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const unit = findExtension(name);
  if (!unit) {
    return (
      <div className="wrap narrow">
        <EmptyState>{t("ext.notFound", { name: name ?? "" })}</EmptyState>
      </div>
    );
  }
  // Object.hasOwn, not a bare index: the generated registry is an object
  // literal. So `extensionScreens["constructor"]` answers from the prototype
  // chain with Object itself. That is truthy and a function, so the dispatch
  // below would mount it. React then dies on "Objects are not valid as a React
  // child". The unit-name grammar admits `constructor`, and a unit without a
  // screen should get the fallback card.
  const UnitScreen = Object.hasOwn(extensionScreens, unit.name)
    ? extensionScreens[unit.name]
    : undefined;
  if (UnitScreen) {
    return <UnitScreen />;
  }
  return (
    <div className="wrap narrow">
      {/* level 1: the head yields to a composed unit, so this card is the only
          thing left that can name the page. A unit with no screen would
          otherwise have no page-level heading at all. */}
      <SectionHeader title={unitTitle(unit, locale)} level={1} />
      <Card>
        <ul>
          {unit.verbs.map((verb) => (
            <li key={verb.operationId}>
              {verb.title} — {verb.method} {verb.route}
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
