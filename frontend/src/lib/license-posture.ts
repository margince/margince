import {
  type QueryClient,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useNow } from "../format/now";
import { useCan } from "./capability";
import { logUnexpectedError, throwProblem } from "./problem";

// The installation's licence, and the one thing the chrome does with it.
//
// Here rather than in either place it grew. The entitlement query lived in
// features/settings/license.tsx, and the posture on it in the agent rail's reads
// module. So the shell's own banner reached a settings screen through the rail.
// Neither file was the home of a read three surfaces share.
//
// ONE query key for all three, which is the point: the banner, the rail's pill
// and the settings card can never disagree about how many seats are in use, and
// the second and third readers pay no second request.

type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

/**
 * What the installation's entitlement adds up to, for a surface that reports
 * rather than enforces.
 *
 * `none` and `refused` are the two a contact has to act on, and they are why the
 * Core carries this at all: an installation with no licence is not a healthy
 * agent with a footnote, it is a standing fault, and the rail used to state it
 * as a grey row at the very bottom that nobody read.
 *
 * `trial` is a production installation with no licence, on its free 14 days
 * or past them. It is not a fault while it runs; `useTrialStanding` counts it.
 *
 * `grace` is a valid licence past its expiry. It works today, but a restart
 * after the module's grace period ends refuses to boot, so the chrome warns
 * until it is renewed. Over the seat cap and renewal due stay off this type:
 * nothing stops on either, and the settings card gives the numbers.
 */
export type LicensePosture = "ok" | "grace" | "refused" | "none" | "trial";

export const LICENSE_ENTITLEMENT_KEY = ["installation-license"] as const;

/**
 * Reads again what the trial lock shows: /me, which holds every seat to the
 * lock screen, and the entitlement the trial notices count from.
 */
export function refreshTrialLock(client: QueryClient): void {
  for (const queryKey of [["me"], LICENSE_ENTITLEMENT_KEY]) {
    client
      .invalidateQueries({ queryKey, exact: true })
      .catch(logUnexpectedError);
  }
}

/**
 * What the license grants and how much of it is used.
 *
 * The queryFn throws, so a screen showing this record can gate on `error`; the
 * chrome reads the reduced posture below instead and shows nothing on a failure.
 */
export function useLicenseEntitlement(enabled = true) {
  return useQuery({
    queryKey: LICENSE_ENTITLEMENT_KEY,
    // A principal without `license:read` would get a 403 on every route. The
    // caller that knows the grant passes it, and the request is never made.
    enabled,
    // The posture changes when somebody is invited or a license is replaced, not
    // between two page opens. The chrome reads it on every route, so a short
    // staleTime would put a request behind every navigation.
    staleTime: 5 * 60_000,
    queryFn: async (): Promise<LicenseEntitlement> => {
      const { data, error, response } = await api.GET("/installation/license");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * The installation's entitlement, reduced to the posture its chrome shows.
 *
 * Absent for a seat without `license:read`, silently: a read they may not make
 * is not a fact being withheld from them, it is a fact that is none of their
 * work, and a notice about it on every screen they opened would be a permission
 * boundary drawn as a fault.
 */
export function useLicensePosture(): LicensePosture | undefined {
  const mayRead = useCan("license", "read");
  const query = useLicenseEntitlement(mayRead);
  const entitlement = query.data;
  if (!mayRead || !entitlement) {
    return undefined;
  }
  return licensePostureOf(entitlement);
}

export function licensePostureOf(
  entitlement: LicenseEntitlement,
): LicensePosture {
  // The state decides first, so a refused entitlement never reads as grace.
  if (entitlement.state === "rejected") {
    return "refused";
  }
  if (entitlement.trial) {
    return "trial";
  }
  if (entitlement.state !== "valid") {
    return "none";
  }
  return entitlement.license?.in_grace ? "grace" : "ok";
}

type LicenseTrial = components["schemas"]["LicenseTrial"];

/** Where the installation's trial stands, as the chrome counts it. */
export type TrialStanding = Readonly<{
  endsAt: string;
  /** Whole days left, at least 1 while the trial runs and 0 once it ended. */
  daysLeft: number;
  ended: boolean;
  /** The last days, when the chrome stops being calm about it. */
  closing: boolean;
}>;

const DAY_MS = 86_400_000;

/** How many days before the end the chrome turns from calm to a warning. */
export const TRIAL_WARNING_DAYS = 7;

/**
 * The trial counted at `now`. The days round up, so the last afternoon still
 * reads as a day left. Whether it has ended is the server's word, so a reader
 * with a wrong clock sees the state the installation acts on.
 */
export function trialStandingOf(
  trial: LicenseTrial,
  now: number,
): TrialStanding {
  const daysLeft = trial.ended
    ? 0
    : Math.max(1, Math.ceil((Date.parse(trial.ends_at) - now) / DAY_MS));
  return {
    endsAt: trial.ends_at,
    daysLeft,
    ended: trial.ended,
    closing: trial.ended || daysLeft <= TRIAL_WARNING_DAYS,
  };
}

/**
 * The trial this installation runs on, for a seat that may read the licence.
 * Undefined while licensed, for a development installation, and for a seat
 * without `license:read`: the lock screen reaches everyone through /me instead.
 */
export function useTrialStanding(): TrialStanding | undefined {
  const mayRead = useCan("license", "read");
  const query = useLicenseEntitlement(mayRead);
  // Once a minute is plenty for a count in whole days.
  const now = useNow(60_000);
  const trial = query.data?.trial;
  useRefreshAtTrialEnd(trial);
  if (!mayRead || !trial) {
    return undefined;
  }
  return trialStandingOf(trial, now);
}

// The longest delay a browser timer keeps; a longer one fires at once.
const MAX_TIMER_MS = 2 ** 31 - 1;

/** How often a session past the end asks again while the server still runs it. */
export const TRIAL_END_RETRY_MS = 60_000;

// A session left open past the end reads the lock at the end. The server's
// clock may run behind, so it asks again each minute while the trial still
// runs there. An ended trial schedules nothing.
export function useRefreshAtTrialEnd(trial: LicenseTrial | undefined): void {
  const client = useQueryClient();
  const endsAt =
    trial === undefined || trial.ended ? undefined : Date.parse(trial.ends_at);
  useEffect(() => {
    if (endsAt === undefined || Number.isNaN(endsAt)) {
      return;
    }
    const delay = Math.min(Math.max(0, endsAt - Date.now()), MAX_TIMER_MS);
    let retry: ReturnType<typeof setInterval> | undefined;
    const timer = setTimeout(() => {
      refreshTrialLock(client);
      retry = setInterval(() => refreshTrialLock(client), TRIAL_END_RETRY_MS);
    }, delay);
    return () => {
      clearTimeout(timer);
      clearInterval(retry);
    };
  }, [client, endsAt]);
}
