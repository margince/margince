// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "time"

// DataTaskIDs lets the upgrade gate inspect the catalog the worker actually uses.
func DataTaskIDs() []string {
	tasks := dataTaskCatalog(time.Now)
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.ID)
	}
	return out
}
