// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/pkg/extension"
)

// RegisterExtensionAITasks adds the AI tasks the composed units declare in
// api/ai-tasks.yaml to the router's tables. The generated composition supplies
// them. Every role that routes model calls calls it once at boot, after
// RegisterExtensions and before any surface serves.
//
// A unit may declare only tasks under its own namespace and only when it
// declares the models need. Nothing is registered when one declaration is
// refused.
func RegisterExtensionAITasks(exts []extension.Extension, decls []extension.AITaskDeclaration) error {
	tasks := make([]ai.ExtensionTask, 0, len(decls))
	for _, d := range decls {
		prefix, err := d.Unit.Namespace()
		if err != nil {
			return fmt.Errorf("compose: AI task %s: %w", d.Task, err)
		}
		if !strings.HasPrefix(d.Task, prefix+"_") {
			return fmt.Errorf("compose: AI task %q of unit %s must start with %s_", d.Task, d.Unit, prefix)
		}
		if !declaresModels(exts, d.Unit) {
			return fmt.Errorf("compose: unit %s declares AI task %q and not the %q need that explains why it wants AI", d.Unit, d.Task, extension.CapabilityModels)
		}
		ladder := make([]ai.Tier, len(d.Ladder))
		for i, tier := range d.Ladder {
			ladder[i] = ai.Tier(tier)
		}
		tasks = append(tasks, ai.ExtensionTask{
			Task: ai.Task(d.Task), Unit: string(d.Unit), DisplayName: d.DisplayName, Summary: d.Summary,
			Ladder: ladder, Background: d.Background, Sites: d.Sites,
		})
	}
	return ai.RegisterExtensionTasks(tasks)
}

func declaresModels(exts []extension.Extension, unit extension.Name) bool {
	for _, e := range exts {
		if e.Name == unit && e.Declares(extension.CapabilityModels) {
			return true
		}
	}
	return false
}
