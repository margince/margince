import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import type { BillingContact, BillingContactRole } from "./billingcontacts";
import { throwProblem } from "./common";

// The three writes the billing-contacts panel makes, sharing one invalidation.
// The same panel is read from two projections — the finance summary the
// Finance tab shows, and the Company360 the Contacts tab reads — so every write
// refetches both, or the tab the reader is not on keeps a saved edit beside its
// own stale value.
//
// A billing contact is an ordinary `relationship` row of kind `billing_contact`
// — there is no billing-specific endpoint, and inventing a hook that pretended
// otherwise would be a second vocabulary for the same edge.

type CreateRelationshipRequest =
  components["schemas"]["CreateRelationshipRequest"];

export function financeSummaryKey(companyId: string) {
  return ["finance-summary", companyId] as const;
}

/**
 * The stored edge's version, asked of the list the write is about to change.
 *
 * The finance summary projects a billing contact without its version — it is a
 * reading, not the row — so a PATCH or an archive has to resolve one. Sending
 * the write unpinned instead would put it straight over whatever changed
 * underneath rather than failing loud with a conflict.
 *
 * Filtered by CONTACT as well as company. `/relationships` answers one page at
 * a time, and a company with more billing rows than fit on it would leave
 * every row past the first page unresolvable — the reader would be told the
 * edge could not be read back, and a reload would say the same thing again.
 * Narrowing to the one contact makes that page hold their handful of edges.
 * The cursor is still followed, because a contact may hold several roles and
 * `works_with` edges name a contact in either column.
 */
async function billingEdgeVersion(
  companyId: string,
  contact: BillingContact,
): Promise<number | undefined> {
  let cursor: string | undefined;
  do {
    const { data, error } = await api.GET("/relationships", {
      params: {
        query: {
          company_id: companyId,
          contact_id: contact.contact_id,
          kind: "billing_contact",
          cursor,
        },
      },
    });
    if (error) {
      throwProblem(error);
    }
    const found = data.data.find((rel) => rel.id === contact.relationship_id);
    if (found) {
      return found.version;
    }
    cursor = data.page.has_more
      ? (data.page.next_cursor ?? undefined)
      : undefined;
  } while (cursor);
  return undefined;
}

/**
 * Naming somebody, changing the capacity they hold, and taking them off.
 *
 * `unresolvedVersion` is the caller's own sentence for the one failure the
 * shared refusal cannot describe: the list this hook scopes its lookup by can
 * legitimately come back without the row (a narrower read scope, a paged
 * response, an edge somebody archived meanwhile), and "the write did not
 * happen" does not tell a reader why.
 */
export function useBillingContactActions(
  companyId: string,
  unresolvedVersion: string,
) {
  const qc = useQueryClient();
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: financeSummaryKey(companyId) });
    qc.invalidateQueries({ queryKey: ["company360", companyId] });
  };

  const patch = async (contact: BillingContact, role: BillingContactRole) => {
    const version = await billingEdgeVersion(companyId, contact);
    if (version === undefined) {
      throwProblem({ detail: unresolvedVersion });
    }
    const { error } = await api.PATCH("/relationships/{id}", {
      params: {
        path: { id: contact.relationship_id },
        ...ifMatch(version),
      },
      body: { role },
    });
    if (error) {
      throwProblem(error);
    }
  };

  const add = useMutation({
    mutationFn: async (args: {
      contactId: string;
      role: BillingContactRole;
    }) => {
      const body: CreateRelationshipRequest = {
        kind: "billing_contact",
        company_id: companyId,
        contact_id: args.contactId,
        role: args.role,
        // The user named this contact, which is what
        // "manual" records — the same value the generic relationship form
        // sends. It is not a provenance the caller may choose freely: an edge
        // a connector discovered says so instead, and reporting one as the
        // other would make a machine's guess read as somebody's decision.
        source: "manual",
      };
      const { data, error } = await api.POST("/relationships", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  const changeRole = useMutation({
    mutationFn: (args: { contact: BillingContact; role: BillingContactRole }) =>
      patch(args.contact, args.role),
    onSuccess: invalidate,
  });

  // Archived, never deleted: who was invoiced last quarter is a fact the
  // ledger's own history rests on.
  //
  // PINNED, like the role change beside it. The endpoint takes an If-Match and
  // the existing unpinned callers are a list the coverage gate lets shrink and
  // never grow — taking somebody off an account is exactly the write that
  // should lose a race with a colleague who just changed their capacity,
  // rather than silently winning it.
  const remove = useMutation({
    mutationFn: async (contact: BillingContact) => {
      const version = await billingEdgeVersion(companyId, contact);
      if (version === undefined) {
        throwProblem({ detail: unresolvedVersion });
      }
      const { error } = await api.DELETE("/relationships/{id}", {
        params: {
          path: { id: contact.relationship_id },
          ...ifMatch(version),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });

  return { add, changeRole, remove };
}

export type BillingContactActions = ReturnType<typeof useBillingContactActions>;
