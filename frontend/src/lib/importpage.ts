// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The id of the contacts screen's import page, `#/contacts/import`. It sits in
// lib/ because both the shell (app/reservedpages.ts) and the shared import link
// (components/vcardimportlink.tsx) name it, and neither may import the other's layer.
// Only on its own screen: `#/leads/import` is the lead whose id is "import".
export const IMPORT_ID = "import";
