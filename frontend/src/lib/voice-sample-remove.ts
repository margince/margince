import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { unwrap } from "./problem";
import { ensureProfileId } from "./voice-profile";

type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];

/** Removes one sample at the version the reader saw; both voice surfaces call it. */
export async function deleteVoiceSample(
  profileId: string,
  sourceId: string,
  version: number,
): Promise<void> {
  unwrap(
    await api.DELETE("/voice-profiles/{id}/sources/{sourceId}", {
      params: { path: { id: profileId, sourceId }, ...ifMatch(version) },
    }),
  );
}

/** Removes one sample, then reads the corpus meter as it stands without it. */
export async function removeSource(
  sourceId: string,
  version: number,
): Promise<CorpusSummary> {
  const profileId = await ensureProfileId();
  await deleteVoiceSample(profileId, sourceId, version);
  const after = unwrap(
    await api.GET("/voice-profiles/{id}/sources", {
      params: { path: { id: profileId } },
    }),
  );
  return after.summary;
}
