// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretRemoved declares that a credential is absent and must stay absent.
//
// Once a credential is sealed into the key vault, a key left out of the file
// falls back to the sealed copy. This value tells the boot to delete that copy
// and run with none. It uses the reference grammar so it cannot be a password:
// a secret field refuses every literal.
const SecretRemoved = "${none}"

// RemovableSecretPattern is SecretRefPattern plus the removal sentinel, for the
// one field that accepts it.
const RemovableSecretPattern = SecretRefPattern + `|^\s*\$\{none\}\s*$`

// RemovableSecret is a Secret that the deployment may also declare removed.
//
// Its own type rather than a flag on Secret, so a field accepts the sentinel
// only by being declared with this type. The license token and the bootstrap
// admin password are plain Secrets and refuse it at decode.
type RemovableSecret struct {
	Secret
	removed bool
}

// UnmarshalYAML accepts the sentinel and hands everything else to Secret.
func (s *RemovableSecret) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if strings.TrimSpace(raw) == SecretRemoved {
		*s = RemovableSecret{removed: true}
		return nil
	}
	*s = RemovableSecret{}
	return s.Secret.UnmarshalYAML(node)
}

// Removed reports whether the deployment declared this credential absent.
func (s RemovableSecret) Removed() bool { return s.removed }
