// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// record writes a run's message stream as it comes. claude exits nonzero on
// the turn cap or an API error, after its result says so, and the SDK then
// throws. The CLI route scores such a run, so a throw after a result is the
// run's own end, and only a throw before one is a run that never finished.
export async function record(messages: AsyncIterable<unknown>, write: (message: unknown) => void): Promise<void> {
  let ended = false;
  try {
    for await (const message of messages) {
      write(message);
      ended ||= isResult(message);
    }
  } catch (stopped: unknown) {
    if (!ended) {
      throw stopped;
    }
  }
}

function isResult(message: unknown): boolean {
  return typeof message === "object" && message !== null && "type" in message && message.type === "result";
}
