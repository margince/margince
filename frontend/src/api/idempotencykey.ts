import { useCallback, useState } from "react";

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

// Keys for send intents, one per request body. A retry of a request still
// open reuses its key even after other requests, so the server replays the
// first answer. A settled request drops its key. The map holds at most
// `limit` open requests, oldest dropped first.
export type IntentKeys = <T extends { response: Response; error?: unknown }>(
  intent: unknown,
  call: (key: string) => Promise<T>,
) => Promise<T>;

export function intentKeys(limit = 16): IntentKeys {
  const open = new Map<string, string>();
  return async (intent, call) => {
    const text = JSON.stringify(intent);
    let key = open.get(text);
    if (key === undefined) {
      key = crypto.randomUUID();
      open.set(text, key);
      const oldest = open.keys().next().value;
      if (open.size > limit && oldest !== undefined) open.delete(oldest);
    }
    const result = await call(key);
    const code = (result.error as { code?: unknown } | undefined)?.code;
    if (settled(result.response.status, code)) open.delete(text);
    return result;
  };
}
