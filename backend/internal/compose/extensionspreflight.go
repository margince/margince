// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/pkg/extension"

// preflightDeclarations runs the checks that read one unit alone, in the order
// validateExtensionSet used to run them inline.
func preflightDeclarations(e extension.Extension) error {
	if err := preflightDocuments(e); err != nil {
		return err
	}
	if err := preflightTools(e); err != nil {
		return err
	}
	if err := preflightSecrets(e); err != nil {
		return err
	}
	if err := preflightJobs(e); err != nil {
		return err
	}
	if err := preflightFailureClasses(e); err != nil {
		return err
	}
	if err := preflightSubscriptions(e); err != nil {
		return err
	}
	if err := preflightSystemReads(e); err != nil {
		return err
	}
	if err := preflightHooks(e); err != nil {
		return err
	}
	return preflightIngress(e)
}
