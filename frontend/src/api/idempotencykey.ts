import { useCallback, useState } from "react";

// One key per create attempt, kept across retries and renewed once the record
// exists. A repeated press then reaches the server as the same request, which
// answers the first result and makes no second record.
export function useIdempotencyKey(): { key: string; renew: () => void } {
  const [key, setKey] = useState(() => crypto.randomUUID());
  const renew = useCallback(() => setKey(crypto.randomUUID()), []);
  return { key, renew };
}

// One key per send intent. Asked again with the same request, as a retry is,
// it answers the same key; any other request, such as one after an edit, gets
// a fresh one. The server then never sees one key over two bodies.
export function intentKeys(): (intent: unknown) => string {
  let last: string | undefined;
  let key = "";
  return (intent) => {
    const text = JSON.stringify(intent);
    if (text !== last) {
      last = text;
      key = crypto.randomUUID();
    }
    return key;
  };
}
