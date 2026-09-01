// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext, useEffect } from "react";
import { isRenderableZone } from "../format/format";
import { FALLBACK_RECORD_ZONE } from "../format/timezone";
import { logUnexpectedError } from "../screens/common";
import { useInstallationSettings } from "./uploadlimit";

/**
 * The organization's own clock, as the installation configured it.
 *
 * This is the reading half of `installation.timezone`. The admin has been able
 * to set that value since the settings card shipped; until now nothing on a
 * record page read it, so an installation in Ho Chi Minh City set its zone,
 * saw the field keep the value, and still had every close date, invoice day and
 * timeline heading rendered on a Berlin clock.
 *
 * Why a context rather than each screen calling the settings query itself:
 * every screen that renders a record date has to agree on the answer. Two
 * screens reading the same query would agree today, but the value is one
 * decision and a second reading of it is a second place for it to be resolved
 * differently — a fallback spelled one way here and another way there is
 * exactly how the record zone and the viewer zone came apart before
 * `format/timezone.ts` existed.
 *
 * The zone is always a string, never undefined. A `string | undefined` would
 * push the "not yet" case into all ~150 call sites, and the honest handling
 * there is not a per-site decision — a date rendered in a guessed zone is
 * wrong in the same way on every one of them. So the waiting is done ONCE, at
 * the authenticated boundary in App.tsx, which holds its first paint until the
 * settings read lands. What reaches a screen has already arrived.
 */
const RecordZoneContext = createContext<string>(FALLBACK_RECORD_ZONE);

/**
 * The zone every record date on this page is read in.
 *
 * A record's dates belong to the record rather than to whoever is looking at
 * it: a close date, a renewal, an invoice's issue day and a timeline's day
 * headings must read the same for every colleague, or two people quoting the
 * same page quote different days. That is the rule `format/timezone.ts` states,
 * and this hook is where the organization's answer to it now comes from.
 *
 * For a moment the reader relates to their own clock — when a credential they
 * are lending expires, when a slot they are booking starts — the answer is
 * still `viewerZone()`, not this.
 */
export function useRecordZone(): string {
  return useContext(RecordZoneContext);
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
export function renderableRecordZone(configured: string | undefined): string {
  if (configured === undefined) {
    return FALLBACK_RECORD_ZONE;
  }
  if (isRenderableZone(configured)) {
    return configured;
  }
  logUnexpectedError(
    new Error(
      `installation.timezone is not a zone this browser can render: "${configured}" — record dates are falling back to ${FALLBACK_RECORD_ZONE}`,
    ),
  );
  return FALLBACK_RECORD_ZONE;
}

/**
 * The installation's zone and whether it is still on its way — the ONE reading
 * of the settings query the record zone makes.
 *
 * `enabled` is the session gate. The settings read needs one, and the three
 * public screens (`book`, `preferences`, `unsubscribe`, `confirm`, `room`) render no record dates at all
 * — they show a moment the reader relates to their own clock, on
 * `viewerZone()`, which needs no installation.
 *
 * One hook rather than a `useRecordZone` and a separate `useRecordZonePending`,
 * because a second reader on this key would decide `enabled` for itself:
 * whichever mounted first would settle whether the request fires at all, and
 * only sometimes. `uploadlimit.ts` says exactly this above its own query, and
 * the record zone is the case that would have proved it.
 *
 * `pending` is what the authenticated boundary holds its splash on, which is
 * what lets `useRecordZone` promise a screen an answer that has already
 * arrived. Rendering through the wait instead would paint every record date on
 * the fallback clock and then move them, so a reader watching a timeline would
 * see the day headings renumber under a page they were reading.
 *
 * A FAILED read does not hold the gate: `isPending` is false once a query has
 * settled either way. The settings request failing is not a reason to refuse
 * the whole application — the record pages still work, on the fallback zone,
 * which is what they did before this existed.
 */
export function useConfiguredRecordZone(enabled: boolean): {
  zone: string;
  pending: boolean;
} {
  const query = useInstallationSettings(enabled);
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
          `the installation settings carry no timezone; record dates are falling back to ${FALLBACK_RECORD_ZONE}`,
        ),
      );
    }
  }, [settledWithoutZone]);
  return {
    zone: renderableRecordZone(query.data?.timezone),
    // A DISABLED query is `isPending` forever — it is waiting for permission
    // to run, not for an answer. Reported as pending, it would hold the splash
    // up over the sign-in screen.
    pending: enabled && query.isPending,
  };
}

/**
 * Serves one zone to every screen under it.
 *
 * The authenticated boundary mounts it with what `useConfiguredRecordZone`
 * resolved. A test or a story mounts it with a zone it names outright, which is
 * why the value is a prop rather than read here: a suite whose expected day
 * moved with the machine it ran on would assert nothing, and asking a test to
 * serve an installation-settings response to state a fact it already knows is
 * a request about the wrong thing.
 *
 * The prop is not validated. The boundary's value has already been through
 * `renderableRecordZone`, and a test naming a zone is stating the premise of
 * its own assertion — a fallback there would quietly answer a different
 * question than the one it asked.
 */
export function RecordZoneProvider({
  zone,
  children,
}: Readonly<{ zone: string; children: ReactNode }>) {
  return (
    <RecordZoneContext.Provider value={zone}>
      {children}
    </RecordZoneContext.Provider>
  );
}
