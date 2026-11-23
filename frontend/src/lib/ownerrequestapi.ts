import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { throwProblem, unwrap } from "./problem";

// A colleague's own request to the owner of a record, read, sent and taken
// back. The handover and the edit access request differ only in their route.

export type RequestTarget<K extends string> = Readonly<{
  recordType: K;
  id: string;
}>;

type Answer<R> = Readonly<{
  data?: R;
  error?: unknown;
  response: Response;
}>;

/** The three calls a request route answers, typed by its own client. */
export type RequestRoute<K extends string, R> = Readonly<{
  read: (target: RequestTarget<K>) => Promise<Answer<R>>;
  send: (target: RequestTarget<K>) => Promise<Answer<R>>;
  withdraw: (target: RequestTarget<K>) => Promise<Answer<unknown>>;
}>;

/**
 * The hooks over one request route. The read answers null when the viewer has
 * no open request: the route answers 404 then, and that 404 is the answer.
 */
export function ownerRequestHooks<K extends string, R>(
  name: string,
  route: RequestRoute<K, R>,
) {
  const key = (recordType: K, id: string) => [name, recordType, id] as const;
  function useRequest(recordType: K, id: string, enabled: boolean) {
    return useQuery({
      queryKey: key(recordType, id),
      enabled,
      queryFn: async (): Promise<R | null> => {
        const { data, error, response } = await route.read({ recordType, id });
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
  function useActions() {
    const queryClient = useQueryClient();
    const refresh = (target: RequestTarget<K>) =>
      queryClient.invalidateQueries({
        queryKey: key(target.recordType, target.id),
      });
    const ask = useMutation({
      mutationFn: async (target: RequestTarget<K>) =>
        unwrap(await route.send(target)),
      onSuccess: (_, target) => refresh(target),
    });
    const withdraw = useMutation({
      mutationFn: async (target: RequestTarget<K>) => {
        const { error } = await route.withdraw(target);
        if (error) {
          throwProblem(error);
        }
      },
      onSuccess: (_, target) => refresh(target),
    });
    return { ask, withdraw };
  }
  return { key, useRequest, useActions };
}
