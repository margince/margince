// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The two fields that can name a colleague in a greeting. Structural, so `/me`
 *  and any other read of a seat can pass its user. */
type GreetableSeat = Readonly<{
  greeting_name?: string | null;
  display_name?: string | null;
}>;

/**
 * Common Vietnamese family names, folded to ASCII. A Vietnamese name puts the
 * family name first and the given name last. The server's list in
 * `draftfloor/givenname.go` is the same, held by a backend parity gate.
 */
export const VIETNAMESE_FAMILY_NAMES: ReadonlySet<string> = new Set([
  "nguyen",
  "tran",
  "le",
  "pham",
  "hoang",
  "huynh",
  "phan",
  "vu",
  "vo",
  "dang",
  "bui",
  "do",
  "ho",
  "ngo",
  "duong",
  "ly",
  "dinh",
  "truong",
  "dao",
  "doan",
  "trinh",
  "luong",
  "cao",
  "chu",
  "quach",
  "tang",
  "thai",
  "trieu",
  "kieu",
  "vuong",
  "luu",
  "phung",
  "lam",
  "ta",
]);

const foldName = (word: string) =>
  word
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Mn}/gu, "")
    .replaceAll("đ", "d");

/**
 * The name a greeting to a colleague uses: the greeting name they chose, else
 * the given name of their display name, else null.
 * The given name is the last word after a Vietnamese family name, else the
 * first word.
 *
 * The same rule as `draftfloor.GreetingName` on the server, so the brief's
 * "Good morning" and a drafted message greet a colleague by the same name.
 */
export function greetingNameOf(seat: GreetableSeat | undefined): string | null {
  const chosen = seat?.greeting_name?.trim();
  if (chosen) {
    return chosen;
  }
  const words = seat?.display_name?.trim().split(/\s+/).filter(Boolean) ?? [];
  const familyFirst =
    words.length > 1 && VIETNAMESE_FAMILY_NAMES.has(foldName(words[0] ?? ""));
  return (familyFirst ? words.at(-1) : words[0]) || null;
}
