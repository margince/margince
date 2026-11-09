import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";

type Activity = components["schemas"]["Activity"];
/**
 * The anchor activity and the project link its thread carries, if any.
 *
 * A filed sibling message is the settled answer, and it outranks what the
 * deal says. It is the first rung of the ladder the capture side walks.
 */
export function useThreadProject(
  activityId?: string,
  enabled = true,
): {
  activity?: Activity;
  projectId?: string;
  settled: boolean;
  failed: boolean;
  retry: () => void;
} {
  const query = useQuery({
    queryKey: ["activity", activityId, "filing"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}", {
          params: { path: { id: activityId ?? "" } },
        }),
      );
    },
    enabled: enabled && Boolean(activityId),
  });
  return {
    // The anchor itself too: the pane needs the `thread_key` this read carries.
    activity: query.data,
    retry: () => {
      void query.refetch();
    },
    // Apart from `settled`: the filing says nothing on a failure, while the
    // pane stops holding its place.
    failed: query.isError,
    projectId: (query.data?.links ?? []).find(
      (link) => link.entity_type === "project",
    )?.entity_id,
    // An unanswered or failed read is not "no project". The composer waits
    // rather than suggest a filing it may take back.
    settled: !activityId || (!query.isPending && !query.isError),
  };
}
