// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import _ "embed"

// BootstrapTemplate is the version, workspace and bootstrap_admin part
// of the generated config/margince.example.yaml. It is written by hand because
// a seed is an example to adapt, not a default to state.
//
//go:embed bootstraptemplate.yaml
var BootstrapTemplate string
