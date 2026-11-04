import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { unwrap } from "./problem";

// The counts and breakdowns installed extensions add, answered for this seat.
// The generated types carry no list of them, so the server answers. Each answer
// has already been through row scope, field masks and the group floor.
export function useExtensionReports() {
  return useQuery({
    queryKey: ["extension-reports"],
    queryFn: async () => unwrap(await api.GET("/extension-reports")),
  });
}
