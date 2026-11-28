// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The route segment every unit's screen lives under: `#/ext/<name>`. App.tsx's
// dispatch, every link and the embed path's check read this one spelling. It
// has its own module, free of the composed registry, so the router's address
// code reads it without loading every unit.
export const EXTENSION_SCREEN = "ext";
