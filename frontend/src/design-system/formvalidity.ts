// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// DateInput reports text it cannot read as "", so a save past it erases the day.
// Not checkValidity() on all: native step and required rules belong to the caller and server.
export function refuseUnreadableField(from: Element): boolean {
  const scope =
    from.closest('form, [role="dialog"], [data-date-scope]') ?? from;
  const unread = Array.from(scope.querySelectorAll("input")).find(
    (field) => field.validity.customError,
  );
  if (unread === undefined) {
    return false;
  }
  // The invalid event shows DateInput's own refusal. Not reportValidity(): it adds a browser bubble.
  unread.checkValidity();
  unread.focus();
  return true;
}
