// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta } from "@storybook/react-vite";
import { TaskDetailModal } from "./taskdetailmodal";

export {
  CapturedEmailRequest,
  DetailOpen,
  FiledUnderDeal,
  SourceTranscriptOpen,
} from "./taskactions.stories";

export default {
  title: "Patterns/Task detail",
  component: TaskDetailModal,
} satisfies Meta<typeof TaskDetailModal>;
