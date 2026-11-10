// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// preflightInstrumentation validates through the same published Validate the
// manifest generator runs.
func preflightInstrumentation(e extension.Extension) error {
	if e.Instrumentation == nil {
		return nil
	}
	if err := e.Instrumentation.Validate(); err != nil {
		return fmt.Errorf("compose: extension %q: %w", e.Name, err)
	}
	return nil
}
