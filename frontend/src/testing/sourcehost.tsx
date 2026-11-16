// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A source opens through two lazy drawers. Load their modules before a test's
// DOM wait, so the first case pays no chunk compilation inside that wait.
import "../features/mail/openemaildrawer";
import "../features/voice-knowledge/activitydrawer";

export { SourceHost } from "../app/sourcehost";
