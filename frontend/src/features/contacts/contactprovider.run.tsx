// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { canEnrichNow } from "../../screens/provider-status";
import type { Profile, Provider } from "./contactprovider";

type EnrichRun = {
  contactId: string;
  provider: Provider;
  // What this press buys. Absent asks for the connection's whole selection;
  // named, it purchases one priced detail without changing what every future
  // run spends.
  categories?: string[];
};

// A component rather than a hook call in the section, so the watch mounts
// only where a profile exists. The section returns early without one, and a
// hook cannot sit behind that return.
export function RunWatch({
  contactId,
  profile,
}: Readonly<{ contactId: string; profile: Profile }>) {
  useRunWatch(contactId, profile);
  return null;
}

/** The lookup itself, hoisted so the header button and the first-run plate
 *  drive ONE mutation: two `useMutation` calls would each carry their own
 *  pending and error state, and the reader would see a failure on whichever
 *  control they did not press.
 *
 *  WHO is looked up and WHERE the answer lands both arrive as mutation
 *  VARIABLES rather than as captured values. React Query re-arms a mutation's
 *  options in a passive effect, so between the commit that renders a control
 *  for one contact and that effect, the observer still holds the previous
 *  render's closure: a captured id would spend a credit on the contact the
 *  reader just left and refresh that page instead of this one. A variable
 *  cannot be older than the control that carried it. */
export function useEnrichRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ contactId, provider, categories }: EnrichRun) => {
      return unwrap(
        await api.POST("/contacts/{id}/enrichment-runs", {
          params: { path: { id: contactId } },
          body: categories ? { provider, categories } : { provider },
        }),
      );
    },
    onSuccess: (_data, { contactId }) => {
      // The run is durable and the provider has not been called yet, so the
      // page re-reads and RunWatch picks the run up from there. Keyed off the
      // variables the run was started with, so the page that refreshes is the
      // one the request was sent for.
      void queryClient.invalidateQueries({
        queryKey: ["contact360", contactId],
      });
    },
  });
}

export function EnrichNow({
  contactId,
  profile,
  enrich,
  free,
  catalogPending,
  running,
  recheck = false,
}: Readonly<{
  free: string[];
  /** Whether a run is still moving. See canEnrichNow: the section state alone
   *  cannot answer it, and a button offered over a live run spends twice. */
  running: boolean;
  // Whether the connection catalog has answered yet. Separate from `free`
  // being empty, which a connection selling nothing free also produces.
  catalogPending: boolean;
  contactId: string;
  profile: Profile;
  enrich: ReturnType<typeof useEnrichRun>;
  // Whether this contact has been looked up before. The press buys the same
  // free set either way. What differs is the offer: a first lookup, or asking
  // again in case a job changed. "Look this contact up" on a record already
  // showing a lookup reads as fetching the email, which it never does.
  recheck?: boolean;
}>) {
  const t = useT();
  if (!canEnrichNow(profile.state, running)) {
    return null;
  }
  return (
    <Button
      type="button"
      pending={enrich.isPending}
      busyLabel={t("provider.profile.lookingUp")}
      // Held while the catalog is in flight, when `free` is empty for some
      // reason other than "nothing is free". An omitted category list asks the
      // server for the whole permitted selection, priced ones included
      // (runcategories.go). That buys a work email under a button labelled
      // free, and costs more the wider an admin sets the selection.
      disabled={catalogPending}
      // A profile carries no provider name until a run exists, so the first
      // lookup on a contact names the one the contract has.
      onClick={() =>
        enrich.mutate({
          contactId,
          provider: profile.provider ?? "surfe",
          // The free categories, named and never omitted. A connection that
          // sells nothing free sends an empty list, which the contract refuses
          // with 422 (minItems). The reader can see a refusal; a purchase, no.
          categories: free,
        })
      }
    >
      <Search aria-hidden="true" />{" "}
      {t(recheck ? "provider.profile.recheck" : "provider.profile.enrichNow")}
    </Button>
  );
}

/** Watches a run that is still moving and refreshes the page when it lands.
 *
 *  It polls the RUN, not the page, and reads whether to continue from the
 *  response it just received — which is the only version that can stop
 *  itself. Deciding from the `profile` prop instead would freeze the answer
 *  at whatever state mounted the poll: the prop only changes when the page
 *  refetches, which is the thing the poll exists to cause.
 */
function useRunWatch(contactId: string, profile: Profile) {
  const queryClient = useQueryClient();
  const runId = profile.latest_run?.id;
  useQuery({
    queryKey: ["provider-run", contactId, runId],
    queryFn:
      runId != null
        ? async () => {
            const data = unwrap(
              await api.GET("/contacts/{id}/enrichment-runs/{run_id}", {
                params: { path: { id: contactId, run_id: runId } },
              }),
            );
            // Re-read on every answer that is not still moving, including the
            // completed-but-unapplied tick that turns the section into values.
            if (!stillMoving(data)) {
              void queryClient.invalidateQueries({
                queryKey: ["contact360", contactId],
              });
            }
            return data;
          }
        : skipToken,
    // The run's own state, never the section's: on an impaired connection the
    // section reads provider_error while a run the reader started is in flight.
    // `applied` and `claims_unwritten` keep it armed until the values are on
    // the record, because the apply commits after the run completes.
    enabled: stillMoving(profile.latest_run),
    // The same rule the arming uses, read from the answer just received rather
    // than from the prop. The prop changes only when the page refetches, which
    // is what this poll exists to cause.
    //
    // Under a ceiling, because the rule alone has no end. Every ordinary route
    // out of `stillMoving` is a state the platform writes: a terminal run, or an
    // exhausted hand-off. A run the workers stop advancing leaves these flags
    // as they are, and an open tab would ask every 2.5 seconds. The ceiling
    // turns that into a page that stopped waiting, which a reader can act on.
    refetchInterval: (query) =>
      keepPolling(query.state.data, query.state.dataUpdateCount)
        ? POLL_EVERY_MS
        : false,
  });
}

/** Whether the page asks after this run again.
 *
 *  The rule and its ceiling in one place, so a test can put the ceiling to it
 *  without waiting out ten minutes of real interval. */
export function keepPolling(
  run: components["schemas"]["ProviderRun"] | undefined,
  asked: number,
): boolean {
  return stillMoving(run) && asked < POLL_LIMIT;
}

const POLL_EVERY_MS = 2500;

/** How many times the page asks after a run before giving up on it.
 *
 *  Ten minutes at POLL_EVERY_MS. Long enough to cover the slowest honest path —
 *  the hand-off's retry ladder, which ends by writing `claims_unwritten` and
 *  stopping the watch on its own — and short enough that a genuinely stuck run
 *  stops costing a request every few seconds for as long as somebody leaves the
 *  tab open. */
export const POLL_LIMIT = 240;

/** The run states that are still moving. `submitting` counts: the run is
 *  mid-flight to the provider, and treating it as finished would offer a
 *  second "look them up" while the first is still out. */
const RUNNING_RUN_STATES = new Set<
  components["schemas"]["ProviderRun"]["state"]
>(["queued", "submitting", "in_progress"]);

/** Whether this page should keep watching a run.
 *
 *  Longer than the run machine's own idea of moving. A completed run's values
 *  reach the record in a SECOND commit, so a watch that stopped at `completed`
 *  refreshed the page one step before the thing the reader is waiting for — the
 *  values — existed. `applied` is the server saying that step happened.
 *
 *  `claims_unwritten` ends it too, and is not the same fact: the purchase
 *  succeeded and the hand-off did not, so nothing further is coming and the
 *  section says so rather than spinning forever.
 *
 *  A page opened AFTER the run completed but before the apply committed still
 *  starts the watch, which is why this is asked of the run rather than of the
 *  click that started it. */
export function stillMoving(
  run: components["schemas"]["ProviderRun"] | undefined,
): boolean {
  if (!run) {
    return false;
  }
  if (RUNNING_RUN_STATES.has(run.state)) {
    return true;
  }
  return run.state === "completed" && !run.applied && !run.claims_unwritten;
}
