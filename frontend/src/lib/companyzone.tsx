// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext, useEffect } from "react";
import { isRenderableZone } from "../format/format";
import { FALLBACK_COMPANY_ZONE } from "../format/timezone";
import { useFiguresFollowTheSwitch } from "./individualperformance";
import { logUnexpectedError } from "./problem";
import { useInstallationSettings } from "./uploadlimit";

/**
 * The company's own clock, as the installation configured it: the reading
 * half of `installation.timezone`.
 *
 * Why a context rather than each screen calling the settings query itself:
 * every screen that reads the company zone has to agree on the answer. Two
 * screens reading the same query would agree today, but the value is one
 * decision and a second reading of it is a second place for it to be resolved
 * differently — a fallback spelled one way here and another way there is
 * exactly how the record zone and the viewer zone came apart before
 * `format/timezone.ts` existed.
 *
 * The zone is always a string, never undefined. A `string | undefined` would
 * push the "not yet" case into every call site, and the honest handling
 * there is not a per-site decision — a date rendered in a guessed zone is
 * wrong in the same way on every one of them. So the waiting is done ONCE, at
 * the authenticated boundary in App.tsx, which holds its first paint until the
 * settings read lands. What reaches a screen has already arrived.
 */
const CompanyZoneContext = createContext<string>(FALLBACK_COMPANY_ZONE);

/**
 * The company's clock, for what belongs to the company: report periods and
 * runs, and the days and weeks the server buckets on that clock.
 *
 * Every other time a signed-in screen shows is read on the reader's own clock,
 * `useViewerZone()`. `format/timezone.ts` states the rule, and the census in
 * `zone-by-purpose.test.ts` lists each file that reads this hook and why.
 */
export function useCompanyZone(): string {
  return useContext(CompanyZoneContext);
}

/**
 * The zone a wire value names, or the fallback when this runtime cannot render
 * it.
 *
 * The server validates `installation.timezone` against its own tzdata before
 * storing it, so a conforming server cannot send a name the browser rejects.
 * "Cannot" is a claim about two zone databases being in step, though, and they
 * are shipped by different vendors on different schedules — so a name Go
 * resolves and this browser does not is a real, if unlikely, state.
 *
 * It must not be a thrown error. `Intl.DateTimeFormat` throws a RangeError on
 * an unknown zone, and the record zone reaches every date on every record page:
 * one bad name would take down not a row but the whole application, for every
 * reader, until an admin who could no longer load the settings screen changed
 * it back.
 *
 * So it falls back and reports. Reporting matters as much as the fallback: this
 * state is the server and this browser disagreeing about what a zone name
 * means, and swallowing it would leave every date quietly on the wrong clock
 * with nothing anywhere saying so.
 *
 * Asked with `isRenderableZone` — the formatters' OWN predicate — rather than
 * by probing Intl here. Probing Intl answers a different question: whether the
 * name resolves, which every fixed offset does, so `Etc/GMT-1` and `+01:00`
 * would pass a hand-rolled probe and then throw inside `formatDate` one line
 * later. `scheduledsends.tsx` learned this on `scheduled_tz` and asks the same
 * way for the same reason.
 */
export function renderableCompanyZone(configured: string | undefined): string {
  if (configured === undefined) {
    return FALLBACK_COMPANY_ZONE;
  }
  if (isRenderableZone(configured)) {
    return configured;
  }
  logUnexpectedError(
    new Error(
      `installation.timezone is not a zone this browser can render: "${configured}" — record dates are falling back to ${FALLBACK_COMPANY_ZONE}`,
    ),
  );
  return FALLBACK_COMPANY_ZONE;
}

/**
 * The installation's zone and whether it is still on its way — the ONE reading
 * of the settings query the record zone makes.
 *
 * `enabled` is the session gate. The settings read needs one, and the three
 * public screens (`book`, `preferences`, `unsubscribe`, `confirm`, `room`) render no record dates at all
 * — they show a moment the reader relates to their own clock, on
 * `browserZone()`, which needs no installation.
 *
 * One hook rather than a `useCompanyZone` and a separate `useRecordZonePending`,
 * because a second reader on this key would decide `enabled` for itself:
 * whichever mounted first would settle whether the request fires at all, and
 * only sometimes. `uploadlimit.ts` says exactly this above its own query, and
 * the record zone is the case that would have proved it.
 *
 * `pending` is what the authenticated boundary holds its splash on, which is
 * what lets `useCompanyZone` promise a screen an answer that has already
 * arrived. Rendering through the wait instead would paint every record date on
 * the fallback clock and then move them, so a reader watching a timeline would
 * see the day headings renumber under a page they were reading.
 *
 * A FAILED read does not hold the gate: `isPending` is false once a query has
 * settled either way. The settings request failing is not a reason to refuse
 * the whole application — the record pages still work, on the fallback zone,
 * which is what they did before this existed.
 */
export function useConfiguredCompanyZone(enabled: boolean): {
  zone: string;
  pending: boolean;
} {
  const query = useInstallationSettings(enabled);
  // Mounted here because this is where the installation read already mounts for
  // every session, which is what the switch has to reach.
  useFiguresFollowTheSwitch(enabled);
  // A settled read that carries no timezone is NOT the same state as one still
  // in flight, though `data?.timezone` spells both `undefined`. The contract
  // makes the field required, so a 200 without it is a server older or newer
  // than this bundle — and treated as "not yet", it would silently render every
  // record date on the fallback clock with nothing anywhere saying why.
  const settledWithoutZone =
    query.isSuccess && query.data?.timezone === undefined;
  // In an effect rather than the render body: a render can run many times for
  // one answer, and a console filling with the same line says less than one
  // line does.
  useEffect(() => {
    if (settledWithoutZone) {
      logUnexpectedError(
        new Error(
          `the installation settings carry no timezone; record dates are falling back to ${FALLBACK_COMPANY_ZONE}`,
        ),
      );
    }
  }, [settledWithoutZone]);
  return {
    zone: renderableCompanyZone(query.data?.timezone),
    // A DISABLED query is `isPending` forever — it is waiting for permission
    // to run, not for an answer. Reported as pending, it would hold the splash
    // up over the sign-in screen.
    pending: enabled && query.isPending,
  };
}

/**
 * Serves one zone to every screen under it.
 *
 * The authenticated boundary mounts it with what `useConfiguredCompanyZone`
 * resolved. A test or a story mounts it with a zone it names outright, which is
 * why the value is a prop rather than read here: a suite whose expected day
 * moved with the machine it ran on would assert nothing, and asking a test to
 * serve an installation-settings response to state a fact it already knows is
 * a request about the wrong thing.
 *
 * The prop is not validated. The boundary's value has already been through
 * `renderableCompanyZone`, and a test naming a zone is stating the premise of
 * its own assertion — a fallback there would quietly answer a different
 * question than the one it asked.
 */
export function CompanyZoneProvider({
  zone,
  children,
}: Readonly<{ zone: string; children: ReactNode }>) {
  return (
    <CompanyZoneContext.Provider value={zone}>
      {children}
    </CompanyZoneContext.Provider>
  );
}
