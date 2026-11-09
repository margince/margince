// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What one run of the dialog-rows gate has worked out. Every gate of a run reads
// the run's keys and sheets, so an answer is fixed by the objects its key names.
export type Memo = {
  ids: Map<object, number>;
  boxes: Map<string, string[][][]>;
  stacks: Map<string, boolean>;
};

export const newMemo = (): Memo => ({
  ids: new Map(),
  boxes: new Map(),
  stacks: new Map(),
});

export function memoKey(
  memo: Memo,
  ...parts: (object | boolean | undefined)[]
): string {
  const idOf = (part: object) => {
    const known = memo.ids.get(part);
    if (known !== undefined) return known;
    memo.ids.set(part, memo.ids.size);
    return memo.ids.size - 1;
  };
  return parts
    .map((part) => (typeof part === "object" ? idOf(part) : part))
    .join(" ");
}

export function remembered<V>(
  answers: Map<string, V>,
  key: string,
  make: () => V,
): V {
  const known = answers.get(key);
  if (known !== undefined) return known;
  const made = make();
  answers.set(key, made);
  return made;
}
