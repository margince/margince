import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ownerRequestHooks } from "../../lib/ownerrequestapi";
import { throwProblem, unwrap } from "../../lib/problem";

export type OwnershipRequest = components["schemas"]["OwnershipRequest"];

/** The record kinds a colleague may ask to be handed. */
export type TransferableRecordType = OwnershipRequest["record_type"];

const PATH = "/records/{record_type}/{id}/ownership-request";

const requests = ownerRequestHooks<TransferableRecordType, OwnershipRequest>(
  "ownership-request",
  {
    read: ({ recordType, id }) =>
      api.GET(PATH, { params: { path: { record_type: recordType, id } } }),
    send: ({ recordType, id }) =>
      api.POST(PATH, {
        params: { path: { record_type: recordType, id } },
        body: {},
      }),
    withdraw: ({ recordType, id }) =>
      api.DELETE(PATH, { params: { path: { record_type: recordType, id } } }),
  },
);

export const ownershipRequestKey = requests.key;

/** The viewer's own open request to be handed this record, or null. */
export const useOwnershipRequest = requests.useRequest;

/** Ask the record's owner to hand it over, or take that request back. */
export const useOwnershipRequestActions = requests.useActions;

type RequestTarget = Readonly<{
  recordType: TransferableRecordType;
  id: string;
}>;

export type OwnershipOffer = components["schemas"]["OwnershipOffer"];

const OFFER_PATH = "/records/{record_type}/{id}/ownership-offer";

export const ownershipOfferKey = (
  recordType: TransferableRecordType,
  id: string,
) => ["ownership-offer", recordType, id] as const;

/**
 * The viewer's own open offer of this record to a colleague, or null when
 * they have none. The read answers 404 then, and that 404 is the answer.
 */
export function useOwnershipOffer(
  recordType: TransferableRecordType,
  id: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ownershipOfferKey(recordType, id),
    enabled,
    queryFn: async (): Promise<OwnershipOffer | null> => {
      const { data, error, response } = await api.GET(OFFER_PATH, {
        params: { path: { record_type: recordType, id } },
      });
      if (error) {
        if (response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data ?? null;
    },
  });
}

type OfferInput = RequestTarget & Readonly<{ toId: string; note: string }>;

/** Offer the viewer's own record to a colleague, or take that offer back. */
export function useOwnershipOfferActions() {
  const queryClient = useQueryClient();
  const refresh = (target: RequestTarget) =>
    queryClient.invalidateQueries({
      queryKey: ownershipOfferKey(target.recordType, target.id),
    });
  const offer = useMutation({
    mutationFn: async ({ recordType, id, toId, note }: OfferInput) =>
      unwrap(
        await api.POST(OFFER_PATH, {
          params: { path: { record_type: recordType, id } },
          body: note.trim()
            ? { to_id: toId, note: note.trim() }
            : { to_id: toId },
        }),
      ),
    onSuccess: (_, target) => refresh(target),
  });
  const withdraw = useMutation({
    mutationFn: async ({ recordType, id }: RequestTarget) => {
      const { error } = await api.DELETE(OFFER_PATH, {
        params: { path: { record_type: recordType, id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: (_, target) => refresh(target),
  });
  return { offer, withdraw };
}

export type OwnershipOfferCandidate =
  components["schemas"]["OwnershipOfferCandidate"];

/**
 * The colleagues the server would accept an offer of this record to. Only the
 * owner may read it, so the caller asks only while the picker is open.
 */
export function useOwnershipOfferCandidates(
  recordType: TransferableRecordType,
  id: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: [...ownershipOfferKey(recordType, id), "candidates"],
    enabled,
    queryFn: async (): Promise<OwnershipOfferCandidate[]> =>
      unwrap(
        await api.GET(`${OFFER_PATH}/candidates`, {
          params: { path: { record_type: recordType, id } },
        }),
      ).data,
  });
}
