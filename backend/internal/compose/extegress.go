// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
	"github.com/margince/margince/backend/pkg/extension"
)

// BindExtensionEgress hands the published egress client the deployment
// posture and the name a unit's request carries. Every role that runs unit code
// calls it once at boot. Only the development posture lets a unit dial loopback.
func BindExtensionEgress(env runtimeenv.Environment) {
	extension.BindEgress(extension.Egress{
		AdmitLoopback: env == runtimeenv.Development,
		UserAgent:     outbound.ExtensionHeader,
	})
}
