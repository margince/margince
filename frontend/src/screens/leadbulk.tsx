import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { leadIdentityName } from "../format/leadname";
import { useLocale, usePlural, useT } from "../i18n";
import { ProblemError, problemMessageOf, throwProblem } from "./common";
import { useRoster } from "./entityref";
import { leadWriteKeys } from "./leadkeys";
import { useLeadDisqualifyReasons } from "./leadsources";

type Lead = components["schemas"]["Lead"];

/** One row's outcome in a fan-out: it went through, or it did not and why. */
export type BulkOutcome = { id: string; name: string; error?: string };

/**
 * Which verb a bulk run applied, for the caller that has to say what moved.
 *
 * Each arm carries everything that verb's write needs, because this is also
 * the mutation's variable: a `mutationFn` reading an owner or a reason out of
 * render state would run whatever the previous render closed over, and the
 * click that carries the variable belongs to the render the reader pressed.
 */
export type BulkAction =
  | { kind: "assign"; ownerId: string; ownerName: string }
  | { kind: "disqualify"; reasonId: string };

/**
 * One bulk run: the verb, and the rows it applies to.
 *
 * The rows travel WITH the verb rather than being read off the `leads` prop
 * inside the mutation. The press belongs to the render that drew the bar, so
 * the selection it hands over is the one the reader could see — a `mutationFn`
 * reaching for the prop runs against whichever render it closed over, and a
 * selection that changed underneath would send writes for rows nobody chose.
 */
type BulkRun = { action: BulkAction; rows: readonly Lead[] };

/**
 * Bulk verbs over selected leads: assign an owner, disqualify. Both are a
 * client-side fan-out of the record's own write — there is no bulk endpoint,
 * and inventing one would bypass the per-row version guard.
 *
 * Disqualify asks why, exactly as the single-lead dialog does: the reason is
 * an ACTIVE administered reason, it is required before the verb will run, and
 * it rides on every row's own DELETE. A bulk path that skipped it would leave
 * `disqualify_reason_id` null on whole batches, which is how the column stops
 * being worth reporting on. No note here, though the dialog offers one: a note
 * is prose about ONE lead, and the same sentence stamped on eight rows is
 * detail nobody wrote about any of them.
 *
 * Every row sends ITS OWN If-Match: PATCH /leads/{id} requires the version
 * (428 without it), and one version copied across the selection would 409
 * on every row but the one it came from. The versions come from the rows the
 * list holds; a row that moved under the reader answers 409, is reported by
 * name, and can be retried after the list refreshes.
 */
export function LeadBulkBar({
  leads,
  onDone,
}: Readonly<{
  /** The selected rows, with the versions the list currently holds. */
  leads: readonly Lead[];
  /** Called after any run — the caller refetches and clears the selection. */
  onDone: (outcomes: readonly BulkOutcome[], action: BulkAction) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const queryClient = useQueryClient();
  const [ownerId, setOwnerId] = useState("");
  const roster = useRoster("user", true);
  const [reasonId, setReasonId] = useState("");
  // The batch confirm's own open state. It is not part of the run's state:
  // a refused batch leaves the bar showing what failed, with the dialog
  // already gone.
  const [confirming, setConfirming] = useState(false);
  const reasons = useLeadDisqualifyReasons();
  const [outcomes, setOutcomes] = useState<readonly BulkOutcome[]>([]);

  // One row's write, chosen by the verb the run carries. The row identity and
  // its version come from the row in hand; the owner or the reason comes from
  // the mutation's variable, so neither can be a value that had left the
  // screen by the time the write went out.
  const apply = async (lead: Lead, action: BulkAction) => {
    if (action.kind === "assign") {
      const { error } = await api.PATCH("/leads/{id}", {
        params: {
          path: { id: lead.id },
          ...ifMatch(requireVersion(lead.version)),
        },
        body: { owner_id: action.ownerId },
      });
      if (error) {
        throwProblem(error, t);
      }
      return;
    }
    const { error } = await api.DELETE("/leads/{id}", {
      params: { path: { id: lead.id } },
      body: { reason_id: action.reasonId },
    });
    if (error) {
      throwProblem(error, t);
    }
  };

  const run = useMutation({
    mutationFn: async ({ action, rows }: BulkRun): Promise<BulkOutcome[]> =>
      // Sequential, not Promise.all: a bulk verb over a work queue is a
      // handful of rows, and a burst of concurrent writes against one
      // rep's own leads buys nothing but contention.
      rows.reduce<Promise<BulkOutcome[]>>(async (acc, lead) => {
        const done = await acc;
        // The id, not the word: this name goes into a per-row outcome the
        // reader reads back afterwards, and two unnamed leads must be tellable
        // apart there.
        const name = leadIdentityName(lead) || lead.id;
        try {
          await apply(lead, action);
          done.push({ id: lead.id, name });
        } catch (error) {
          done.push({
            id: lead.id,
            name,
            error:
              error instanceof ProblemError
                ? problemMessageOf(error, t)
                : t("lead.bulkFailedRow"),
          });
        }
        return done;
      }, Promise.resolve([])),
    onSuccess: async (result, { action }) => {
      // EVERY lead the run touched, refused ones included, and not only the
      // list.
      //
      // Not only the list, because the list is `["leads", query]` and a
      // detail page is the sibling `["lead", id]`: a bulk assign moved forty
      // owners and left forty detail pages showing the old one.
      //
      // Refused ones included, because a refusal is the case where the
      // client's copy is MOST likely wrong. The commonest refusal here is a
      // version conflict, which says the server's row moved — so the row that
      // refused is exactly the row whose cached version must not be trusted.
      // Skipping them also emptied this set whenever a whole run refused, and
      // then nothing was invalidated at all: the reader keeps the selection to
      // retry, retries against the version that just conflicted, and conflicts
      // forever until they reload the page by hand.
      //
      // Awaited: the rows that refused keep their selection so they can be
      // retried, and a retry that fired before the refetch landed would resend
      // the very version that just conflicted. The run stays pending — and the
      // verbs disabled — until the refetch has SETTLED.
      //
      // Settled, not succeeded, and the difference is worth writing down
      // because the comment here used to promise the stronger thing.
      // `invalidateQueries` resolves whether the refetch it triggered
      // succeeded or failed — it swallows the refetch error unless
      // `throwOnError` is set. So this closes the window in which a retry
      // races a refetch still in flight, which is the failure it was written
      // for, and it does NOT promise fresh versions after a refetch the server
      // refused. That case is the ordinary one of a list that could not be
      // read, and it announces itself the way any failed read does rather than
      // by turning a completed bulk run into an error the reader cannot act on.
      await Promise.all(
        result
          .flatMap((row) => leadWriteKeys(row.id))
          .map((key) => queryClient.invalidateQueries({ queryKey: key })),
      );
      setOutcomes(result);
      onDone(result, action);
    },
  });

  const ownerName =
    (roster.data ?? [])
      .filter((entry) => entry.id === ownerId)
      .map((entry) => ("display_name" in entry ? entry.display_name : null))
      .find((name) => typeof name === "string") ?? ownerId;
  const assign = () =>
    run.mutate({
      action: { kind: "assign", ownerId, ownerName },
      rows: leads,
    });
  // The reason list is administered (Settings › Data model); only its ACTIVE
  // rows may be applied, and a payload that is not the contract's array is
  // read as nothing rather than crashing the bar that renders it.
  const activeReasons = (
    Array.isArray(reasons.data) ? reasons.data : []
  ).filter((reason) => reason.active);
  // The reason is read where the CLICK happens and travels as the mutation's
  // variable, so what reaches every row's DELETE is the reason that was on
  // screen when the reader pressed the verb.
  const disqualify = () => {
    setConfirming(false);
    run.mutate({ action: { kind: "disqualify", reasonId }, rows: leads });
  };
  const reasonLabel =
    activeReasons.find((reason) => reason.id === reasonId)?.label ?? "";

  const failed = outcomes.filter((o) => o.error);
  return (
    <>
      <span className="t-caption">
        {t("lead.bulkSelected", { count: formatNumber(leads.length, locale) })}
      </span>
      <Select
        aria-label={t("lead.bulkOwner")}
        value={ownerId}
        placeholder={t("lead.bulkOwnerPick")}
        disabled={run.isPending}
        onChange={setOwnerId}
        options={(roster.data ?? []).map((entry) => ({
          value: entry.id,
          // The user roster: every entry carries a display name; a team
          // (the other roster kind) is never asked for here.
          label: "display_name" in entry ? entry.display_name : entry.id,
        }))}
      />
      <Button
        small
        variant="primary"
        disabled={run.isPending || ownerId === "" || leads.length === 0}
        onClick={assign}
      >
        {t("lead.bulkAssign")}
      </Button>
      <Select
        aria-label={t("lead.disqualify.reason")}
        value={reasonId}
        placeholder={t("lead.disqualify.pickReason")}
        disabled={run.isPending}
        onChange={setReasonId}
        options={activeReasons.map((reason) => ({
          value: reason.id,
          label: reason.label,
        }))}
      />
      <Button
        small
        disabled={run.isPending || leads.length === 0}
        // The same requirement, and the same sentence, as the single-lead
        // dialog: a batch closed with no reason is exactly what the
        // administered list exists to prevent.
        reason={reasonId ? undefined : t("lead.disqualify.reasonRequired")}
        onClick={() => setConfirming(true)}
      >
        {t("lead.bulkDisqualify")}
      </Button>
      {/* Closing one lead opens a dialog; closing forty from a toolbar used to
          fire on the press. The batch is the LESS reversible of the two — the
          reader cannot see the rows they are about to close, only a count —
          so it asks the same question, and names the reason it is about to
          write on all of them. */}
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={plural("lead.bulkDisqualifyTitle", leads.length, {
          count: formatNumber(leads.length, locale),
        })}
        confirmLabel={t("lead.bulkDisqualify")}
        confirmVariant="danger"
        onConfirm={disqualify}
        pending={run.isPending}
      >
        <p className="t-caption">
          {t("lead.bulkDisqualifyBody", { reason: reasonLabel })}
        </p>
      </ConfirmModal>
      {failed.length > 0 && (
        <span className="t-caption t-danger">
          {t("lead.bulkFailed", {
            count: formatNumber(failed.length, locale),
          })}{" "}
          {failed.map((o) => `${o.name}: ${o.error}`).join(" · ")}
        </span>
      )}
    </>
  );
}
