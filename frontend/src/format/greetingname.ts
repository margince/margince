// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The two fields that can name a colleague in a greeting. Structural, so `/me`
 *  and any other read of a seat can pass its user. */
type GreetableSeat = Readonly<{
  greeting_name?: string | null;
  display_name?: string | null;
}>;

/**
 * Common Vietnamese family names, lower-cased, each in its Vietnamese and its
 * plain ASCII spelling. A Vietnamese name puts the family name first and the
 * given name last. The server's list in `draftfloor/givenname.go` is the same,
 * held by a backend parity gate.
 */
export const VIETNAMESE_FAMILY_NAMES: ReadonlySet<string> = new Set([
  "nguyễn",
  "nguyen",
  "trần",
  "tran",
  "lê",
  "le",
  "phạm",
  "pham",
  "hoàng",
  "hoang",
  "huỳnh",
  "huynh",
  "phan",
  "vũ",
  "vu",
  "võ",
  "vo",
  "đặng",
  "dang",
  "bùi",
  "bui",
  "đỗ",
  "do",
  "hồ",
  "ho",
  "ngô",
  "ngo",
  "dương",
  "duong",
  "lý",
  "ly",
  "đinh",
  "dinh",
  "trương",
  "truong",
  "đào",
  "dao",
  "đoàn",
  "doan",
  "trịnh",
  "trinh",
  "lương",
  "luong",
  "cao",
  "chu",
  "quách",
  "quach",
  "tăng",
  "tang",
  "triệu",
  "trieu",
  "kiều",
  "kieu",
  "vương",
  "vuong",
  "lưu",
  "luu",
  "phùng",
  "phung",
  "lâm",
  "lam",
  "tạ",
  "ta",
]);

const familyName = (word: string | undefined) =>
  word !== undefined && VIETNAMESE_FAMILY_NAMES.has(word.toLowerCase());

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
  // A name that also ends with a family name is in Western order already.
  const familyFirst =
    words.length > 1 && familyName(words[0]) && !familyName(words.at(-1));
  return (familyFirst ? words.at(-1) : words[0]) || null;
}
