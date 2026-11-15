// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Reads one page of ids. It answers the ids it found, and an id it leaves out
 * is a settled absence. It throws a `ProblemError` when the server refused.
 */
export type PageReader<T> = (
  page: readonly string[],
) => Promise<ReadonlyMap<string, T>>;

// A batch reads at most this many pages at a time. A batch of 101 to 200 ids
// then costs one round trip, not two. A big batch never opens every page.
const PAGES_IN_FLIGHT = 2;

// What one batch came back with. A page's refusal is about the ids ON that
// page. The answers another page returned are ones the server gave, and one
// map could only report them as a read that never arrived.
type Answers<T> = Readonly<{
  found: ReadonlyMap<string, T>;
  failures: ReadonlyMap<string, unknown>;
}>;

/**
 * Coalesces every id asked for within one tick into reads of at most
 * `pageSize` ids, rather than one request per reference. A leaf component
 * cannot see its siblings, so the coalescing lives beside the request. One
 * batch is shared, and every caller is served from its one answer.
 *
 * The returned function resolves an id to its answer, or to `null` when the
 * pages left it out. For an id on a refused page it throws that page's problem.
 */
export function idBatcher<T>(
  pageSize: number,
  readPage: PageReader<T>,
): (id: string) => Promise<T | null> {
  let queued: string[] = [];
  let batch: Promise<Answers<T>> | null = null;

  async function readAll(ids: readonly string[]): Promise<Answers<T>> {
    const pages: (readonly string[])[] = [];
    for (let at = 0; at < ids.length; at += pageSize) {
      pages.push(ids.slice(at, at + pageSize));
    }
    return merge(pages, await readPages(readPage, pages));
  }

  function join(id: string): Promise<Answers<T>> {
    queued.push(id);
    if (!batch) {
      // Opened as a microtask: every component that renders in this tick has
      // enqueued by the time it runs, and nothing waits on a timer.
      batch = Promise.resolve().then(() => {
        const asking = [...new Set(queued)];
        queued = [];
        batch = null;
        return readAll(asking);
      });
    }
    return batch;
  }

  return async (id) => {
    const { found, failures } = await join(id);
    if (failures.has(id)) {
      throw failures.get(id);
    }
    return found.get(id) ?? null;
  };
}

// Each worker asks for its next page only once its last one settles, so one
// batch has at most PAGES_IN_FLIGHT reads open. The cap is per batch: a later
// batch starts at once and never waits behind an older one. The answers keep
// page order, so a merge reads the same however the pages finish.
async function readPages<T>(
  readPage: PageReader<T>,
  pages: readonly (readonly string[])[],
): Promise<(ReadonlyMap<string, T> | Refusal)[]> {
  const answers: (ReadonlyMap<string, T> | Refusal)[] = [];
  let next = 0;
  async function worker() {
    while (next < pages.length) {
      const at = next;
      next += 1;
      answers[at] = await readOrRefusal(readPage, pages[at] ?? []);
    }
  }
  const workers = Math.min(PAGES_IN_FLIGHT, pages.length);
  await Promise.all(Array.from({ length: workers }, worker));
  return answers;
}

function merge<T>(
  pages: readonly (readonly string[])[],
  answers: readonly (ReadonlyMap<string, T> | Refusal | undefined)[],
): Answers<T> {
  const found = new Map<string, T>();
  const failures = new Map<string, unknown>();
  for (const [at, page] of pages.entries()) {
    const answer = answers[at];
    if (answer instanceof Refusal) {
      for (const id of page) {
        failures.set(id, answer.cause);
      }
    } else if (answer) {
      for (const [id, value] of answer) {
        found.set(id, value);
      }
    }
  }
  return { found, failures };
}

// A page that failed, refused or never answered, is held against its own ids.
// Rejecting the shared batch would lose what another page already read.
class Refusal {
  constructor(readonly cause: unknown) {}
}

async function readOrRefusal<T>(
  readPage: PageReader<T>,
  page: readonly string[],
): Promise<ReadonlyMap<string, T> | Refusal> {
  try {
    return await readPage(page);
  } catch (error) {
    return new Refusal(error);
  }
}
