// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The two fields that can name a colleague in a greeting. Structural, so `/me`
 *  and any other read of a seat can pass its user. */
type GreetableSeat = Readonly<{
  greeting_name?: string | null;
  display_name?: string | null;
}>;

/**
 * The name a greeting to a colleague uses: the greeting name they chose, else
 * the first word of their display name, else null when neither names anybody.
 *
 * The same rule as `draftfloor.GreetingName` on the server, so the brief's
 * "Good morning" and a drafted message greet a colleague by the same name.
 */
export function greetingNameOf(seat: GreetableSeat | undefined): string | null {
  const chosen = seat?.greeting_name?.trim();
  if (chosen) {
    return chosen;
  }
  const first = seat?.display_name?.trim().split(/\s+/)[0];
  return first || null;
}
