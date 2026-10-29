// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ProblemError } from "./problem";

/**
 * Reads one page of ids. It answers the ids it found and throws a
 * `ProblemError` when the server refused the page; an id it leaves out is a
 * settled absence.
 */
export type PageReader<T> = (
  page: readonly string[],
) => Promise<ReadonlyMap<string, T>>;

// What one batch came back with. A page's refusal is about the ids ON that
// page: the answers an earlier page returned are ones the server gave, and a
// single map could only report them as a read that never arrived.
type Answers<T> = Readonly<{
  found: ReadonlyMap<string, T>;
  failures: ReadonlyMap<string, ProblemError>;
}>;

/**
 * Coalesces every id asked for within one tick into reads of at most
 * `pageSize` ids, rather than one request per reference. A leaf component
 * cannot see its siblings, so the coalescing lives beside the request: one
 * shared batch, every caller served from the one answer it produces.
 *
 * The returned function resolves an id to its answer, to `null` for an id the
 * pages left out, and throws the page's problem for an id on a refused page.
 */
export function idBatcher<T>(
  pageSize: number,
  readPage: PageReader<T>,
): (id: string) => Promise<T | null> {
  let queued: string[] = [];
  let batch: Promise<Answers<T>> | null = null;

  async function readAll(ids: readonly string[]): Promise<Answers<T>> {
    const found = new Map<string, T>();
    const failures = new Map<string, ProblemError>();
    for (let at = 0; at < ids.length; at += pageSize) {
      const page = ids.slice(at, at + pageSize);
      const answer = await readOrRefusal(readPage, page);
      if (answer instanceof ProblemError) {
        for (const id of page) {
          failures.set(id, answer);
        }
      } else {
        for (const [id, value] of answer) {
          found.set(id, value);
        }
      }
    }
    return { found, failures };
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
    const failed = failures.get(id);
    if (failed) {
      throw failed;
    }
    return found.get(id) ?? null;
  };
}

// A refused page is held against its own ids rather than rejecting the shared
// batch, which would lose what another page already read.
async function readOrRefusal<T>(
  readPage: PageReader<T>,
  page: readonly string[],
): Promise<ReadonlyMap<string, T> | ProblemError> {
  try {
    return await readPage(page);
  } catch (error) {
    if (error instanceof ProblemError) {
      return error;
    }
    throw error;
  }
}
