// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

// What other pages import to offer writing. The window itself is in
// composewindow.tsx, which loads only when someone opens the composer.

// The composer reviews replies, first messages and relinks before submitting.
export type Activity = components["schemas"]["Activity"];

export type VoiceProfile = components["schemas"]["VoiceProfile"];

export { ChannelReplyAction } from "./compose.reply";
export { ComposeModal, loadComposeWindow } from "./composedock";
export type { ComposeProps } from "./composeprops";
export { RELINK_KINDS, type RelinkKind, RelinkModal } from "./composerelink";
