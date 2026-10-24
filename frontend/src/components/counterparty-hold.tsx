import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { throwProblem, unwrap } from "../lib/problem";

// "My mail with this party is nobody else's."
//
// A hold is about the CORRESPONDENT rather than about any one message: holding
// a lawyer's domain says that whatever passes between you is private, without
// having to read each message to decide. It belongs on the record page because
// that is where a contact thinks about the correspondent — the Senders page
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

/** What a hold control is about: one address, or one whole domain. */
export type HoldScope =
  | { address: string; domain?: never }
  | { domain: string; address?: never };

// A domain hold covers its own domain and every subdomain under it, the way the
// server folds one.
function domainHoldCovers(hold: Hold, domain: string): boolean {
  return (
    hold.kind === "domain" &&
    (domain === hold.value || domain.endsWith(`.${hold.value}`))
  );
}

// The server stores domains as lowercase ASCII without a trailing dot, so an
// internationalised domain must be compared in its xn-- form.
function normalizeDomain(domain: string): string {
  const lower = domain.trim().toLowerCase().replace(/\.$/, "");
  try {
    return new URL(`http://${lower}`).hostname || lower;
  } catch {
    return lower;
  }
}

// The narrowest domain hold over this domain: lifting a parent hold would
// unprotect sibling subdomains that the pressed row never named.
function narrowestDomainHold(
  holds: readonly Hold[] | undefined,
  domain: string,
): Hold | undefined {
  return holds
    ?.filter((hold) => domainHoldCovers(hold, domain))
    .sort((a, b) => b.value.length - a.value.length)[0];
}

// The hold that covers this address, if the caller placed one: the address
// itself, or a domain hold over its domain.
export function holdCoveringAddress(
  holds: readonly Hold[] | undefined,
  address: string | undefined,
): Hold | undefined {
  if (!address) {
    return undefined;
  }
  const normal = address.trim().toLowerCase();
  const domain = normalizeDomain(normal.slice(normal.indexOf("@") + 1));
  return (
    holds?.find((hold) => hold.kind === "address" && hold.value === normal) ??
    narrowestDomainHold(holds, domain)
  );
}

// The hold that covers this whole domain. An address hold never does: it names
// one mailbox, not the company.
export function holdCoveringDomain(
  holds: readonly Hold[] | undefined,
  domain: string | undefined,
): Hold | undefined {
  if (!domain) {
    return undefined;
  }
  return narrowestDomainHold(holds, normalizeDomain(domain));
}

// Everything the render and the press need, derived once from the scope so no
// branch can build an address out of a domain.
function resolveScope(scope: HoldScope) {
  const address = scope.address?.trim().toLowerCase();
  const rawDomain = scope.domain ?? address?.slice(address.indexOf("@") + 1);
  if (!rawDomain?.trim()) {
    return undefined;
  }
  const domain = normalizeDomain(rawDomain);
  return {
    address,
    domain,
    verbs: (address ? ["address", "domain"] : ["domain"]) as readonly string[],
  };
}

function usePlaceHold() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // Kind and value both arrive as variables: the row a reader pressed belongs
    // to the committed render (frontend/AGENTS.md, mutation-variable-coverage).
    mutationFn: async (vars: { kind: "address" | "domain"; value: string }) => {
      unwrap(
        await api.POST("/capture/counterparty-holds", {
          body: { kind: vars.kind, value: vars.value },
        }),
      );
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
      unwrap(
        await api.DELETE("/capture/counterparty-holds/{id}", {
          params: { path: { id } },
        }),
      );
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
 * `scope` is either the address a contact is known by or the domain a company
 * is known by. A contact offers both verbs: this address, or everyone at its
 * domain. A company offers only the domain verb, since it has no address of its
 * own. An empty value stands the control down rather than offering a press
 * with nothing to name.
 */
export function CounterpartyHoldRow({ scope }: Readonly<{ scope: HoldScope }>) {
  const t = useT();
  const holds = useHolds();
  const place = usePlaceHold();
  const lift = useLiftHold();
  const [asking, setAsking] = useState<"address" | "domain" | null>(null);

  const resolved = resolveScope(scope);
  if (!resolved) {
    return null;
  }
  const { address, domain, verbs } = resolved;
  const held = address
    ? holdCoveringAddress(holds.data?.data, address)
    : holdCoveringDomain(holds.data?.data, domain);

  if (held) {
    return (
      <div>
        <p>
          {held.kind === "domain"
            ? t("hold.heldByDomain", { domain: held.value })
            : t("hold.heldByAddress")}
        </p>
        <Button
          variant="ghost"
          pending={lift.isPending}
          onClick={() => lift.mutate(held.id)}
        >
          {t("hold.lift")}
        </Button>
        {/* Said on the way OUT, not behind a confirm on the way in: lifting is
            the reversible half, and what surprises a reader is that it does not
            re-open what was already held. */}
        <p>{t("hold.liftingWidensNothing")}</p>
      </div>
    );
  }

  return (
    <div>
      <p>{t(address ? "hold.notHeld" : "hold.notHeldCompany")}</p>
      <div className="card-actions">
        {verbs.includes("address") && (
          <Button variant="ghost" onClick={() => setAsking("address")}>
            {t("hold.holdAddress")}
          </Button>
        )}
        {/* A domain hold is the one worth having for an advisor: a firm answers
            from whichever address picked up the file. Offered as its own verb
            rather than a modifier, because the two reach very different sets. */}
        {/* The label carries the domain itself, whose length nothing here
            chooses — a subdomain under a regional TLD is routinely longer than
            this rail is wide, and a button that will not wrap draws it past
            both of its own edges. */}
        <Button
          variant="ghost"
          className="btn-valuelabel"
          onClick={() => setAsking("domain")}
        >
          {t("hold.holdDomain", { domain })}
        </Button>
      </div>
      <ErrorLine error={place.error} />
      <ConfirmModal
        open={asking !== null}
        onClose={() => setAsking(null)}
        title={t("hold.confirmTitle")}
        confirmLabel={t("hold.confirmVerb")}
        pending={place.isPending}
        onConfirm={() => {
          const value = asking === "domain" ? domain : address;
          if (asking && value) {
            place.mutate(
              { kind: asking, value },
              { onSuccess: () => setAsking(null) },
            );
          }
        }}
      >
        <p>
          {asking === "domain"
            ? t("hold.confirmDomainBody", { domain })
            : t("hold.confirmAddressBody", { address: address ?? "" })}
        </p>
        {/* The limit, stated where the decision is made rather than discovered
            afterwards: a hold placed today does not reach back over a year of
            correspondence by itself. */}
        <p className="t-caption">{t("hold.confirmHistoryNote")}</p>
      </ConfirmModal>
    </div>
  );
}
