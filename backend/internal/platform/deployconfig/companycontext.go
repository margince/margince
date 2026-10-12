// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

// CompanyContextRollout is the ordered deployment capability for company
// knowledge. The empty YAML value resolves to onboarding so an upgrade keeps
// today's behavior until an operator deliberately stages it backward.
type CompanyContextRollout string

const (
	// CompanyContextOff disables context reads, task injection, and onboarding.
	CompanyContextOff CompanyContextRollout = "off"
	// CompanyContextRead enables the canonical read model and settings surface.
	CompanyContextRead CompanyContextRollout = "read"
	// CompanyContextTasks additionally enables declared AI task injection.
	CompanyContextTasks CompanyContextRollout = "tasks"
	// CompanyContextOnboarding additionally enables the first-run experience.
	CompanyContextOnboarding CompanyContextRollout = "onboarding"
)

// CompanyContext configures the operator-controlled company-context rollout.
type CompanyContext struct {
	Rollout CompanyContextRollout `yaml:"rollout"`
}

// EffectiveRollout applies the compiled-in default without mutating the
// decoded configuration.
func (c CompanyContext) EffectiveRollout() CompanyContextRollout {
	if c.Rollout == "" {
		return CompanyContextOnboarding
	}
	return c.Rollout
}

// ReadEnabled reports whether typed reads, refresh, and settings are active.
func (c CompanyContext) ReadEnabled() bool {
	stage := c.EffectiveRollout()
	return stage == CompanyContextRead || stage == CompanyContextTasks || stage == CompanyContextOnboarding
}

// TasksEnabled reports whether declared model tasks may receive company data.
func (c CompanyContext) TasksEnabled() bool {
	stage := c.EffectiveRollout()
	return stage == CompanyContextTasks || stage == CompanyContextOnboarding
}

// OnboardingEnabled reports whether the five-step first-run surface is active.
func (c CompanyContext) OnboardingEnabled() bool {
	return c.EffectiveRollout() == CompanyContextOnboarding
}
