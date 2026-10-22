import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { useT } from "../../i18n";
import { useRecordWriteRefusal } from "../../lib/capability";
import { useClaimRecord } from "../../lib/claimrecord";
import { throwProblem, unwrap } from "../../lib/problem";
import type { Lead, UpdateLeadRequest } from "./leads";

/**
 * One write's variables: what to change, and the record it is changing.
 *
 * The version and the closed flag travel with the body, not read off `lead`
 * inside the mutation. A handler belongs to the render that drew the pressed
 * control, so its version cannot be older than that control. A `mutationFn`
 * reads the render it closes over, which after React Query's passive re-arm is
 * not always the one on screen.
 */
type LeadWrite = {
  body: UpdateLeadRequest;
  version: number;
  /** Whether the record was already closed when the reader pressed. */
  archived: boolean;
};

export function useLeadPatch(lead: Lead, id: string, onChanged: () => void) {
  const t = useT();
  // A lead that takes no writes (closed, or not this caller's to change)
  // refuses every control here by one fact, derived once. A per-control test
  // misses the control that had to remember on its own. The sentence travels
  // with the answer, so each refused control can say which fact it was.
  const readOnlyReason = useRecordWriteRefusal("lead", lead, {
    archived: t("lead.terminalReadOnly"),
    notYours: t("lead.notYoursToChange"),
  });
  const readOnly = Boolean(readOnlyReason);
  const patch = useMutation({
    mutationKey: ["lead-edit", id],
    mutationFn: async ({ body, version, archived }: LeadWrite) => {
      // The last word on a terminal lead, in one place rather than per control.
      // The server refuses every one of these writes, and a later control would
      // otherwise have to remember. It reads the flag the press carried, so a
      // lead that went terminal while the page was open is refused.
      if (archived) {
        // Catalog copy in a problem body, like every other refusal this screen
        // shows. "A terminal lead takes no writes" is a sentence for whoever
        // reads this file, not for whoever is refused.
        throwProblem({ detail: t("lead.terminalReadOnly") });
      }
      return unwrap(
        await api.PATCH("/leads/{id}", {
          params: { path: { id }, ...ifMatch(requireVersion(version)) },
          body,
        }),
      );
    },
    onSuccess: onChanged,
  });

  // The record as it stands in this render, stamped onto the write the caller
  // is starting. Every control goes through one of these two, so no call site
  // has to remember to carry the version.
  const write = (body: UpdateLeadRequest): LeadWrite => ({
    body,
    version: requireVersion(lead.version),
    archived: Boolean(lead.archived_at),
  });
  const save = (body: UpdateLeadRequest) => patch.mutate(write(body));

  // Taking an unowned lead is a claim, not a patch. The write arm refuses an
  // ownerless row, so the request that works here is the claim door. It lives
  // beside `patch` because the page states a refused write in one banner. A
  // refusal thrown inside a rail that is not rendered reaches nobody.
  const claim = useMutation({
    mutationKey: ["lead-claim", lead.id],
    mutationFn: useClaimRecord("lead", lead.id, lead.version),
    onSuccess: onChanged,
  });

  return { patch, claim, readOnly, readOnlyReason, save };
}
