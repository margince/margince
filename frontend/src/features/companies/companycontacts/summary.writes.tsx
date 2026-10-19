import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch } from "../../../api/version";
import { Button } from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../../i18n";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../../../lib/problem";
import { navigate } from "../../../lib/router";
import type { Seat } from "./summary";

/**
 * useCommitteeWrites is the three writes this board performs. Each POSTs to the
 * deal and then invalidates the account's coverage, the key this band reads.
 * A seat written to a deal changes the account's committee, so refreshing the
 * deal alone would leave the board stale.
 *
 * Confirm and Change are the same patch on the same row. The store reassigns
 * `captured_by` to whoever edits, so a human edit clears the mark. The mark
 * asks "did a human answer this", and an edit is that answer.
 */
export function useCommitteeWrites(
  companyId: string,
  dealId: string | null | undefined,
) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const refresh = () => {
    queryClient.invalidateQueries({
      queryKey: ["company-coverage", companyId],
    });
  };

  const suggest = useMutation({
    mutationFn: async (dealId: string) => {
      const { data, error } = await api.POST("/deals/{id}/role-proposals", {
        params: { path: { id: dealId } },
      });
      if (error || !data) {
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: refresh,
  });

  // Which rows are being confirmed, held here rather than read off the
  // mutation. One observer serves every card, and TanStack switches it to the
  // newest call. The set lets two cards be busy at once, each with its own spinner.
  const [confirming, setConfirming] = useState<ReadonlySet<string>>(new Set());
  const patch = useMutation({
    onMutate: (seat: SeatPatch) => {
      setConfirming((busy) => new Set(busy).add(seat.relationshipId));
    },
    onSettled: (_data, _error, seat) => {
      setConfirming((busy) => {
        const next = new Set(busy);
        next.delete(seat.relationshipId);
        return next;
      });
    },
    mutationFn: async (seat: SeatPatch) => {
      unwrap(
        await api.PATCH("/relationships/{id}", {
          // The conditional write. A colleague may have changed this seat while the
          // page was open. Without the version, the confirmation would overwrite
          // their change and leave no trace of it.
          params: {
            path: { id: seat.relationshipId },
            ...ifMatch(seat.version),
          },
          body: { role: seat.role },
        }),
      );
    },
    onSuccess: refresh,
  });

  return {
    suggest,
    confirming,
    // Confirming writes the role back unchanged. That is not a no-op: the
    // store reassigns captured_by to the editing human. So the same role,
    // written by a human, turns the machine's reading into theirs.
    confirm: (seat: Seat) => {
      if (seat.relationship_id && seat.relationship_version !== undefined) {
        patch.mutate({
          relationshipId: seat.relationship_id,
          role: seat.role,
          version: seat.relationship_version,
        });
      }
    },
    // The deal's stakeholder panel, which is where a role is edited. The
    // contact's own page shows the role as a read-only badge, so it would
    // promise an edit and deliver a label.
    change: () => {
      if (dealId) {
        navigate({ screen: "deals", id: dealId });
      }
    },
    note: <WriteNote suggest={suggest} patch={patch} locale={locale} t={t} />,
  };
}

/**
 * WriteNote says what the last write did, in words rather than a count alone.
 * The two empty answers are different facts. Nothing proposed means their
 * messages do not say who buys. Everything refused means the product read
 * something and would not stand behind it. A single "no roles found" would
 * report the second as the first.
 */
function WriteNote({
  suggest,
  patch,
  locale,
  t,
}: Readonly<{
  suggest: ReturnType<
    typeof useMutation<DealRoleProposalResult, Error, string>
  >;
  patch: ReturnType<typeof useMutation<void, Error, SeatPatch>>;
  locale: Locale;
  t: ReturnType<typeof useT>;
}>) {
  const plural = usePlural();
  // The PATCH's failure first: a suggestion that failed minutes ago would
  // otherwise mask the newer write the reader pressed last.
  if (patch.isError) {
    return <ErrorLine error={patch.error} />;
  }
  if (suggest.isError) {
    return (
      <p className="cp-write-note">
        {problemCodeOf(suggest.error) === "not_implemented"
          ? t("co.contacts.board.suggestUnavailable")
          : problemMessageOf(suggest.error, t)}
      </p>
    );
  }
  const result = suggest.data;
  if (!result) {
    return null;
  }
  if (result.written.length > 0) {
    return (
      <p className="cp-write-note">
        {plural("co.contacts.board.suggestWrote", result.written.length, {
          count: formatNumber(result.written.length, locale),
        })}
      </p>
    );
  }
  return (
    <p className="cp-write-note">
      {result.skipped > 0
        ? plural("co.contacts.board.suggestRefused", result.skipped, {
            count: formatNumber(result.skipped, locale),
          })
        : t("co.contacts.board.suggestNothing")}
    </p>
  );
}

/**
 * SuggestRoles is the verb that reads the committee out of the correspondence.
 *
 * Disabled with a reason rather than hidden when the account has no open deal.
 * A role is recorded on a deal. A reader who cannot see why the button is inert
 * learns nothing about what to do instead.
 */
export function SuggestRoles({
  writes,
  dealId,
}: Readonly<{
  writes: {
    suggest: ReturnType<
      typeof useMutation<DealRoleProposalResult, Error, string>
    >;
  };
  dealId: string | null | undefined;
}>) {
  const t = useT();
  return (
    <Button
      variant="ai"
      onClick={() => dealId && writes.suggest.mutate(dealId)}
      reason={dealId ? undefined : t("co.contacts.board.suggestNoDeal")}
      pending={writes.suggest.isPending}
      busyLabel={t("co.contacts.board.suggesting")}
    >
      <Sparkles aria-hidden="true" />
      {t("co.contacts.board.suggest")}
    </Button>
  );
}

type DealRoleProposalResult = components["schemas"]["DealRoleProposalResult"];

type SeatPatch = { relationshipId: string; role: string; version: number };
