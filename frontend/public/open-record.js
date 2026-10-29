// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const link = document.getElementById("record");
if (link instanceof HTMLAnchorElement) {
  const destination = new URL(link.href);
  const identity = destination.hash.split("?")[0];
  const current = window.location.hash;
  if (current === identity || current.startsWith(`${identity}?`) || current.startsWith(`${identity}/`)) {
    destination.hash = current;
  }
  window.location.replace(destination.href);
}
