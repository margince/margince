import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { unwrap } from "../../lib/problem";

// The row identity travels as a variable: a closed-over `version` can be older
// than the control pressed, and its If-Match then refuses a write nobody raced.
export type AutomationPatch = {
  id: string;
  version: number | undefined;
  body: {
    name?: string;
    params?: Record<string, unknown>;
    status?: "enabled" | "paused";
    owner_id?: string;
    initial_run_confirmed?: boolean;
  };
};

export async function patchAutomation({ id, version, body }: AutomationPatch) {
  return unwrap(
    await api.PATCH("/automations/{id}", {
      params: { path: { id }, ...ifMatch(requireVersion(version)) },
      body,
    }),
  );
}
