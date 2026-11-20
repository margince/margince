// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Contract = components["schemas"]["Contract"];
type ContractStatus = NonNullable<Contract["status"]>;

// The words for an agreement's asserted status, read by every screen that
// shows one.
export const CONTRACT_STATUS_LABEL: Record<ContractStatus, MessageKey> = {
  draft: "contracts.status.draft",
  active: "contracts.status.active",
  expired: "contracts.status.expired",
  cancelled: "contracts.status.cancelled",
  superseded: "contracts.status.superseded",
};
