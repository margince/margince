// The key lifecycle shared by every sender of a retryable write: one key per
// intent, kept while the outcome is unknown and dropped once it is settled.
// No React here, so a choice card's single-file bundle can import it.

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
      key = crypto.randomUUID();
      open.set(text, key);
      const oldest = open.keys().next().value;
      if (open.size > limit && oldest !== undefined) open.delete(oldest);
    }
    const result = await call(key);
    if (isSettled(result) && open.get(text) === key) open.delete(text);
    return result;
  };
}
