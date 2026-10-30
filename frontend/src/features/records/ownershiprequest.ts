import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem, unwrap } from "../../lib/problem";

export type OwnershipRequest = components["schemas"]["OwnershipRequest"];

/** The record kinds a colleague may ask to be handed. */
export type TransferableRecordType = OwnershipRequest["record_type"];

const PATH = "/records/{record_type}/{id}/ownership-request";

export const ownershipRequestKey = (
  recordType: TransferableRecordType,
  id: string,
) => ["ownership-request", recordType, id] as const;

/**
 * The viewer's own open request to be handed this record, or null when they
 * have none: the read answers 404 then, and that 404 is the answer.
 */
export function useOwnershipRequest(
  recordType: TransferableRecordType,
  id: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ownershipRequestKey(recordType, id),
    enabled,
    queryFn: async (): Promise<OwnershipRequest | null> => {
      const { data, error, response } = await api.GET(PATH, {
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

type RequestTarget = Readonly<{
  recordType: TransferableRecordType;
  id: string;
}>;

/** Ask the record's owner to hand it over, or take that request back. */
export function useOwnershipRequestActions() {
  const queryClient = useQueryClient();
  const refresh = (target: RequestTarget) =>
    queryClient.invalidateQueries({
      queryKey: ownershipRequestKey(target.recordType, target.id),
    });
  const ask = useMutation({
    mutationFn: async ({ recordType, id }: RequestTarget) =>
      unwrap(
        await api.POST(PATH, {
          params: { path: { record_type: recordType, id } },
          body: {},
        }),
      ),
    onSuccess: (_, target) => refresh(target),
  });
  const withdraw = useMutation({
    mutationFn: async ({ recordType, id }: RequestTarget) => {
      const { error } = await api.DELETE(PATH, {
        params: { path: { record_type: recordType, id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: (_, target) => refresh(target),
  });
  return { ask, withdraw };
}
