import { useCallback, useState } from "react";
import { keyedIntents } from "./intentkeys";

// One key per create attempt, kept across retries and renewed once the record
// exists. A repeated press then reaches the server as the same request, which
// answers the first result and makes no second record.
export function useIdempotencyKey(): { key: string; renew: () => void } {
  const [key, setKey] = useState(() => crypto.randomUUID());
  const renew = useCallback(() => setKey(crypto.randomUUID()), []);
  return { key, renew };
}

// The answers after which a key is spent. A 5xx, a thrown fetch or a claim
// still in flight may have left the request running, so its key is kept.
function settled(status: number, code: unknown): boolean {
  return status < 500 && code !== "idempotency_key_in_progress";
}

// Keys for send intents, one per request body: see keyedIntents.
export type IntentKeys = <T extends { response: Response; error?: unknown }>(
  intent: unknown,
  call: (key: string) => Promise<T>,
) => Promise<T>;

export function intentKeys(limit = 16): IntentKeys {
  return keyedIntents(
    (result: { response: Response; error?: unknown }) =>
      settled(
        result.response.status,
        (result.error as { code?: unknown } | undefined)?.code,
      ),
    limit,
  ) as IntentKeys;
}
