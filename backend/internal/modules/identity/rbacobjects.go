// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
)

// RbacObject is an RBAC object name an extension contributes to the
// vocabulary. It is an ALIAS, not a wrapper: identity owns the vocabulary but
// internal/policy owns the grammar, and a wrapper type here would need a
// conversion that could be written without the grammar's Validate — which is
// the one thing standing between a directory an operator dropped in and what a
// stored role document may grant.
type RbacObject = policy.Object

// RbacObjects is the RBAC object vocabulary one composition extends core's
// with, and the starting grants its units declare on the system roles. See
// policy/composable.go for why the vocabulary has to be composable at all.
//
// The zero value registers nothing: a reader handed none drops every grant on
// an extension object, which denies rather than widens.
type RbacObjects struct {
	vocabulary policy.Vocabulary
	defaults   []RbacDefault
}

// NewRbacObjects builds a composition's vocabulary. It validates everything
// before it builds anything. Each default must name one of the objects, because
// a role document drops a grant on an object it cannot name.
//
// It lives on identity because identity OWNS the vocabulary: internal/policy is
// unreachable from the composition root.
func NewRbacObjects(objects []RbacObject, defaults []RbacDefault) (RbacObjects, error) {
	vocabulary, err := policy.NewVocabulary(objects...)
	if err != nil {
		return RbacObjects{}, err
	}
	if err := ValidateRbacDefaults(defaults...); err != nil {
		return RbacObjects{}, err
	}
	for _, d := range defaults {
		if !vocabulary.IsRegistered(string(d.Object)) {
			return RbacObjects{}, fmt.Errorf("identity: default grant names %q, which no unit registered", d.Object)
		}
	}
	return RbacObjects{vocabulary: vocabulary, defaults: slices.Clone(defaults)}, nil
}

// Grantable reports whether an object name is one a role document may grant at
// all: a core object or one this vocabulary registers.
//
// The gates that derive an authority requirement somewhere else ask it. It
// proves the requirement is satisfiable before they certify what depends on it.
// No principal that can exist holds an object outside this set. A requirement
// naming one is a permanent refusal, not a strict rule.
func (r RbacObjects) Grantable(object string) bool {
	return r.vocabulary.IsGrantable(object)
}

// WithRbacObjects hands the service the composition's vocabulary. Every grant it
// resolves, edits or seeds reads it.
func (s *Service) WithRbacObjects(rbac RbacObjects) *Service {
	s.rbac = rbac
	return s
}
