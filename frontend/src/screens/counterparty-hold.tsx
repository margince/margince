import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";

// "My mail with this party is nobody else's."
//
// A hold is about the CORRESPONDENT rather than about any one message: holding
// a lawyer's domain says that whatever passes between you is private, without
// having to read each message to decide. It belongs on the record page because
// that is where a person thinks about the correspondent — the Senders page
// answers "what did the classifier decide", this answers "I have decided, about
// this one".
//
// Per seat, and only ever the caller's own. One rep's lawyer says nothing about
// another rep's, and a shared list would let anyone keep a colleague's customer
// out of the CRM by naming their domain. So there is no admin view of this and
// no id here that reaches another seat's holds.
//
// Placing a hold binds mail captured FROM HERE ON. Narrowing the history is a
// separate press with the count in front of you, and lifting a hold widens
// NOTHING: mail held while the hold stood was held for a reason that was true at
// the time, and re-opening a year of correspondence as a side effect of tidying
// a list is not something anybody asked for.

type Hold = components["schemas"]["CaptureCounterpartyHold"];

function useHolds() {
  return useQuery({
    queryKey: ["capture-counterparty-holds"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/capture/counterparty-holds",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The hold that covers this address, if the caller placed one. An address hold
// names it exactly; a domain hold covers every address under it, including a
// subdomain, the way the server folds one.
export function holdCovering(
  holds: readonly Hold[] | undefined,
  email: string | undefined,
): Hold | undefined {
  if (!email) {
    return undefined;
  }
  const address = email.trim().toLowerCase();
  const domain = address.slice(address.indexOf("@") + 1);
  return holds?.find((hold) =>
    hold.kind === "address"
      ? hold.value === address
      : domain === hold.value || domain.endsWith(`.${hold.value}`),
  );
}

function usePlaceHold() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // Kind and value both arrive as variables: the row a reader pressed belongs
    // to the committed render (frontend/AGENTS.md, mutation-variable-coverage).
    mutationFn: async (vars: { kind: "address" | "domain"; value: string }) => {
      const { error } = await api.POST("/capture/counterparty-holds", {
        body: { kind: vars.kind, value: vars.value },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["capture-counterparty-holds"],
      });
      toast.show(t("settings.saved"));
    },
  });
}

function useLiftHold() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/capture/counterparty-holds/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["capture-counterparty-holds"],
      });
      toast.show(t("settings.saved"));
    },
  });
}

/**
 * The hold control for one correspondent, for the record rails.
 *
 * `email` is the address the record is known by; a record with none cannot be
 * held by address and the control stands down rather than offering a press that
 * would have nothing to name.
 */
export function CounterpartyHoldRow({
  email,
}: Readonly<{ email: string | undefined }>) {
  const t = useT();
  const holds = useHolds();
  const place = usePlaceHold();
  const lift = useLiftHold();
  const [asking, setAsking] = useState<"address" | "domain" | null>(null);

  if (!email) {
    return null;
  }
  const address = email.trim().toLowerCase();
  const domain = address.slice(address.indexOf("@") + 1);
  const held = holdCovering(holds.data?.data, address);

  if (held) {
    return (
      <div className="pe-rail-hold">
        <p className="t-caption">
          {held.kind === "domain"
            ? t("hold.heldByDomain", { domain: held.value })
            : t("hold.heldByAddress")}
        </p>
        <Button
          small
          variant="ghost"
          pending={lift.isPending}
          onClick={() => lift.mutate(held.id)}
        >
          {t("hold.lift")}
        </Button>
        {/* Said on the way OUT, not behind a confirm on the way in: lifting is
            the reversible half, and what surprises a reader is that it does not
            re-open what was already held. */}
        <p className="t-caption">{t("hold.liftingWidensNothing")}</p>
      </div>
    );
  }

  return (
    <div className="pe-rail-hold">
      <p className="t-caption">{t("hold.notHeld")}</p>
      <div className="card-actions">
        <Button small variant="ghost" onClick={() => setAsking("address")}>
          {t("hold.holdAddress")}
        </Button>
        {/* A domain hold is the one worth having for an advisor: a firm answers
            from whichever address picked up the file. Offered as its own verb
            rather than a modifier, because the two reach very different sets. */}
        <Button small variant="ghost" onClick={() => setAsking("domain")}>
          {t("hold.holdDomain", { domain })}
        </Button>
      </div>
      {place.isError && (
        <p className="t-caption" role="alert">
          {problemMessageOf(place.error, t)}
        </p>
      )}
      <ConfirmModal
        open={asking !== null}
        onClose={() => setAsking(null)}
        title={t("hold.confirmTitle")}
        confirmLabel={t("hold.confirmVerb")}
        pending={place.isPending}
        onConfirm={() => {
          if (asking) {
            place.mutate(
              { kind: asking, value: asking === "domain" ? domain : address },
              { onSuccess: () => setAsking(null) },
            );
          }
        }}
      >
        <p>
          {asking === "domain"
            ? t("hold.confirmDomainBody", { domain })
            : t("hold.confirmAddressBody", { address })}
        </p>
        {/* The limit, stated where the decision is made rather than discovered
            afterwards: a hold placed today does not reach back over a year of
            correspondence by itself. */}
        <p className="t-caption">{t("hold.confirmHistoryNote")}</p>
      </ConfirmModal>
    </div>
  );
}
