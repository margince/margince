import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem, unwrap } from "../../lib/problem";

export type EditAccessRequest = components["schemas"]["EditAccessRequest"];

/** The record kinds a colleague may ask the owner for edit access to. */
export type EditAccessRecordType = EditAccessRequest["record_type"];

const PATH = "/records/{record_type}/{id}/edit-access-request";

export const editAccessRequestKey = (
  recordType: EditAccessRecordType,
  id: string,
) => ["edit-access-request", recordType, id] as const;

/**
 * The viewer's own open request for edit access, or null when they have none.
 * The read answers 404 then, and that 404 is the answer.
 */
export function useEditAccessRequest(
  recordType: EditAccessRecordType,
  id: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: editAccessRequestKey(recordType, id),
    enabled,
    queryFn: async (): Promise<EditAccessRequest | null> => {
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
  recordType: EditAccessRecordType;
  id: string;
}>;

/** Ask the record's owner for edit access, or take that request back. */
export function useEditAccessRequestActions() {
  const queryClient = useQueryClient();
  const refresh = (target: RequestTarget) =>
    queryClient.invalidateQueries({
      queryKey: editAccessRequestKey(target.recordType, target.id),
    });
  const ask = useMutation({
    mutationFn: async ({ recordType, id }: RequestTarget) =>
      unwrap(
        await api.POST(PATH, {
          params: { path: { record_type: recordType, id } },
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
