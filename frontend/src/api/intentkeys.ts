// The key lifecycle every sender of a retryable write shares. One key per
// intent: kept while the outcome is unknown, dropped once it is settled.
// It has no React, so a choice card's single-file bundle can import it.

/** keyedIntents holds one key per intent text. A retry of an intent still open
 *  reuses its key, so the server replays the first answer. A settled call drops
 *  its key, but only while the stored key is the one that call used: an
 *  overlapping call's late answer must not drop the key of a newer press. The
 *  map holds at most `limit` open intents, oldest dropped first. */
export function keyedIntents<R>(
  isSettled: (result: R) => boolean,
  limit = 16,
): (intent: unknown, call: (key: string) => Promise<R>) => Promise<R> {
  const open = new Map<string, string>();
  return async (intent, call) => {
    const text = JSON.stringify(intent);
    let key = open.get(text);
    if (key === undefined) {
      key = randomKey();
      open.set(text, key);
      const oldest = open.keys().next().value;
      if (open.size > limit && oldest !== undefined) open.delete(oldest);
    }
    const result = await call(key);
    if (isSettled(result) && open.get(text) === key) open.delete(text);
    return result;
  };
}

/** randomKey is a version 4 UUID from getRandomValues. A choice card can run in
 *  a sandboxed frame that is no secure context, where randomUUID is absent. */
export function randomKey(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  // The version nibble is 4 and the variant bits are 10, as RFC 9562 sets them.
  const hex = Array.from(bytes, (b, i) => {
    const marked =
      i === 6 ? (b & 0x0f) | 0x40 : i === 8 ? (b & 0x3f) | 0x80 : b;
    return marked.toString(16).padStart(2, "0");
  }).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
