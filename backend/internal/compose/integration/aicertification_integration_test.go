// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// How well the bound models perform each AI job, read through the wire.
//
// The two things only a real request can show: that the STORED binding is what
// the answer resolves against — not a seed file, not a config on disk — and
// that an installation which has bound nothing says so rather than reporting
// every job as unmeasured.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func jobOf(t *testing.T, view crmcontracts.AiCertification, task string) crmcontracts.AiCertificationJob {
	t.Helper()
	for _, job := range view.Jobs {
		if job.Task == task {
			return job
		}
	}
	t.Fatalf("no job %q among the %d reported", task, len(view.Jobs))
	return crmcontracts.AiCertificationJob{}
}

// A fresh installation has bound nothing, and that is a choice nobody has made
// rather than a measurement nobody took. Reported as its own word, because a
// reader told "not checked" would go looking for a certification run that would
// not help them.
func TestAiCertificationReportsAFreshInstallationAsUnbound(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var got crmcontracts.AiCertification
	if status := e.Call(t, http.MethodGet, "/v1/ai/certification", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /v1/ai/certification = %d, want 200", status)
	}
	if got.BindingState != "unbound" {
		t.Errorf("binding_state = %q, want unbound on an installation that has bound nothing", got.BindingState)
	}
	if len(got.Jobs) == 0 {
		t.Fatal("no jobs reported; the card would be empty rather than honest")
	}
	for _, job := range got.Jobs {
		if job.Result != "no_model" {
			t.Errorf("%s reads %q with nothing bound, want no_model", job.Task, job.Result)
		}
	}
	// The sample size travels per JOB, derived from that job's own counts — the
	// lane's repeat count is configurable per run, so one figure for the whole
	// page would be a claim about runs it never saw. With nothing bound there
	// are no counts to derive it from, and it is absent rather than guessed.
	for _, job := range got.Jobs {
		if job.RunsPerExample != nil {
			t.Errorf("%s reports a sample size with nothing bound: %d", job.Task, *job.RunsPerExample)
		}
	}
}

// The answer resolves against the binding stored through THIS surface. A
// certification page that read a seed file would describe a fresh install
// forever, while the installation served something else.
func TestAiCertificationFollowsTheStoredBinding(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// capture_classify's ladder leads at local_small, and its records were
	// measured on this model under eu_hosted — so binding it here is what turns
	// the job from unmeasured into a reading.
	binding := crmcontracts.AiRouting{
		Profile: "eu_hosted",
		// The whole document is replaced, so the embedding lane has to be
		// stated even though this test is about the chat tiers.
		Embeddings: crmcontracts.AiEmbeddingsBinding{
			Provider: "openai_compatible", Model: "openai/text-embedding-3-small",
			BaseUrl: strPtr("https://openrouter.ai/api"),
		},
		Tiers: map[string]crmcontracts.AiTierBinding{
			"local_small": {
				Provider: "openai_compatible", Model: "openai/gpt-oss-120b",
				BaseUrl: strPtr("https://openrouter.ai/api"),
			},
		},
	}
	// Decoded loosely on purpose: a refusal here is a problem document, not a
	// binding, and its detail names which field the store objected to — which is
	// the difference between fixing this test in one round and in five.
	stored := map[string]any{}
	if status := e.Call(t, http.MethodPut, "/v1/ai/routing", binding, nil, &stored); status != http.StatusOK {
		t.Fatalf("PUT /v1/ai/routing = %d, want 200: %v", status, stored)
	}

	var got crmcontracts.AiCertification
	if status := e.Call(t, http.MethodGet, "/v1/ai/certification", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /v1/ai/certification = %d, want 200", status)
	}
	if got.BindingState != "bound" {
		t.Fatalf("binding_state = %q after a binding was stored, want bound", got.BindingState)
	}

	classify := jobOf(t, got, "capture_classify")
	if classify.Model == nil || *classify.Model != "openai/gpt-oss-120b" {
		t.Errorf("model = %v, want the model just stored", classify.Model)
	}
	if classify.Result == "no_model" || classify.Result == "not_checked" {
		t.Errorf("capture_classify reads %q on a model its records measured", classify.Result)
	}
	// A job whose ladder this binding does not reach still says so, rather than
	// borrowing the bound rung of another job.
	docs := jobOf(t, got, "document_extract")
	if docs.Result != "no_model" {
		t.Errorf("document_extract reads %q, want no_model — its ladder is premium-only and nothing bound it",
			docs.Result)
	}

	// The grader is the instrument, not a job the product runs for a user.
	for _, job := range got.Jobs {
		if job.Task == "cert_judge" {
			t.Error("the certification grader is reported as a product job")
		}
	}
}
