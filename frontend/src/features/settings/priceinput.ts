// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * What the server accepts for a per-million-token price typed into a form: a
 * plain non-negative decimal, up to twelve whole digits and six fractional.
 * Zero is a price (a local model costs nothing to call), so it matches.
 *
 * A declared MIRROR of the contract's pattern and of `ai.UsdPerMTokToMicroUSD`,
 * held in both directions by `backend/gates/frontendpriceinput_test.go`: a form
 * that accepts more than the server sends a request that can only be refused,
 * and one that accepts less refuses a price the sheet would have stored.
 */
export const PER_MTOK_PRICE = /^[0-9]{1,12}(\.[0-9]{1,6})?$/;
