import { useCallback, useState } from "react";

// One key per create attempt, kept across retries and renewed once the record
// exists. A repeated press then reaches the server as the same request, which
// answers the first result and makes no second record.
export function useIdempotencyKey(): { key: string; renew: () => void } {
  const [key, setKey] = useState(() => crypto.randomUUID());
  const renew = useCallback(() => setKey(crypto.randomUUID()), []);
  return { key, renew };
}
