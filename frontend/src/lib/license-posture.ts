import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "./capability";
import { throwProblem } from "./problem";

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
 * `grace` is a valid licence past its expiry. It works today, but a restart
 * after the module's grace period ends refuses to boot, so the chrome warns
 * until it is renewed. Over the seat cap and renewal due stay off this type:
 * nothing stops on either, and the settings card gives the numbers.
 */
export type LicensePosture = "ok" | "grace" | "refused" | "none";

/**
 * What the license grants and how much of it is used.
 *
 * The queryFn throws, so a screen showing this record can gate on `error`; the
 * chrome reads the reduced posture below instead and shows nothing on a failure.
 */
export function useLicenseEntitlement(enabled = true) {
  return useQuery({
    queryKey: ["installation-license"],
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
  if (entitlement.state !== "valid") {
    return "none";
  }
  return entitlement.license?.in_grace ? "grace" : "ok";
}
