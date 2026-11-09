// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Runs scored concurrently must build the record runs scored one after another
// build. The models here answer from the request alone, so which worker asks
// first cannot change any answer, and only the harness's own ordering can.

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// requestKeyedClient answers each request from its content, so a run's answer
// does not depend on the order concurrent runs reach it.
type requestKeyedClient struct {
	model.NoHealth
	answer func(req model.Request) string
}

func (requestKeyedClient) Caps() model.Capabilities { return model.Capabilities{} }

//nolint:ireturn // the port's own signature
func (requestKeyedClient) Stream(context.Context, model.Request) (model.TokenStream, error) {
	return nil, errors.New("the test client completes only")
}

func (requestKeyedClient) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	return model.Embeddings{}, model.ErrEmbeddingsUnsupported
}

func (c requestKeyedClient) Complete(_ context.Context, req model.Request) (model.Response, error) {
	return model.Response{Text: c.answer(req)}, nil
}

// requestText is every text a request carries, for a client to key its answer on.
func requestText(req model.Request) string {
	parts := []string{req.System}
	for _, m := range req.Messages {
		parts = append(parts, m.Content)
	}
	return strings.Join(parts, "\n")
}

// mixedCorpus is a task whose cases settle differently. One passes and is
// extended, one fails every run, one passes with a score near a band.
func mixedCorpus() []Scenario {
	scenarios := make([]Scenario, 0, 3)
	for _, subject := range []string{"alpha", "beta", "gamma"} {
		sc := testScenario(subject, wideBands)
		sc.Fixture = JSONValue(`{"subject":"a widget ` + subject + `"}`)
		scenarios = append(scenarios, sc)
	}
	return scenarios
}

func mixedClients() (candidate, judge requestKeyedClient) {
	candidate.answer = func(req model.Request) string {
		if strings.Contains(requestText(req), "beta") {
			return "something off topic"
		}
		return "the " + containsWidget + " described: " + requestText(req)
	}
	judge.answer = func(req model.Request) string {
		if strings.Contains(requestText(req), "gamma") {
			return scoreJSON(72)
		}
		return scoreJSON(90)
	}
	return candidate, judge
}

// certifyMixed certifies mixedCorpus with parallel runs in flight.
func certifyMixed(t *testing.T, parallel int) Record {
	t.Helper()
	withFixedNow(t, fixedResumeNow)
	candidate, judge := mixedClients()
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, mixedCorpus(), testCensus(t),
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "candidate"},
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "judge"}, ai.ProfileCloudFrontier, testRepeats, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, judge)},
			parallel:      parallel,
		})
	if err != nil {
		t.Fatalf("certifying with %d run(s) in flight: %v", parallel, err)
	}
	// Latency is the one number measured off a real clock; everything else
	// the record holds must be equal.
	rec.LatencyP50, rec.LatencyP95 = 0, 0
	return rec
}

func TestRunsInFlightTogetherBuildTheRecordRunsInSequenceBuild(t *testing.T) {
	sequential := certifyMixed(t, 1)
	if sequential.Runs <= len(mixedCorpus())*testRepeats {
		t.Fatalf("the sequential record holds %d runs; the corpus must extend a case for this test to cover a second round", sequential.Runs)
	}
	if sequential.Passed == sequential.Runs || sequential.Passed == 0 {
		t.Fatalf("the sequential record passed %d of %d runs; the corpus must mix passes and failures", sequential.Passed, sequential.Runs)
	}
	for _, parallel := range []int{2, 4, 16} {
		if got := certifyMixed(t, parallel); !reflect.DeepEqual(got, sequential) {
			t.Fatalf("with %d runs in flight the record is\n %+v\nwant the sequential record\n %+v", parallel, got, sequential)
		}
	}
}

func TestScoreConcurrentlyFoldsInJobOrderWhateverOrderRunsFinish(t *testing.T) {
	jobs := make([]runJob, 8)
	for i := range jobs {
		jobs[i] = runJob{caseIndex: i, run: 1}
	}
	// The first job finishes last: it waits for every other job to be measured.
	var others sync.WaitGroup
	others.Add(len(jobs) - 1)
	var folded []int
	results, err := scoreConcurrently(context.Background(), jobs, len(jobs),
		func(_ context.Context, _ int, job runJob) (int, error) {
			if job.caseIndex == 0 {
				others.Wait()
			} else {
				others.Done()
			}
			return job.caseIndex * 10, nil
		},
		func(job runJob, measured int) (RunResult, error) {
			folded = append(folded, job.caseIndex)
			return RunResult{Score: measured}, nil
		})
	if err != nil {
		t.Fatalf("scoreConcurrently: %v", err)
	}
	if want := []int{0, 1, 2, 3, 4, 5, 6, 7}; !slices.Equal(folded, want) {
		t.Fatalf("folded in order %v, want job order %v", folded, want)
	}
	for i, r := range results {
		if r.Score != i*10 {
			t.Fatalf("result %d holds score %d, want %d", i, r.Score, i*10)
		}
	}
}

// numberedJobs is n jobs whose case index is their position.
func numberedJobs(n int) []runJob {
	jobs := make([]runJob, n)
	for i := range jobs {
		jobs[i] = runJob{caseIndex: i, run: 1}
	}
	return jobs
}

// A failure leaves every run before it folded and journaled, as a single
// worker scoring in order would. Those runs were paid for, and a restart replays them.
func TestScoreConcurrentlyKeepsEveryRunBeforeTheFailure(t *testing.T) {
	errRunFailed := errors.New("run 3 failed")
	jobThreeFailed := make(chan struct{})
	var journaled []int
	_, err := scoreConcurrently(context.Background(), numberedJobs(6), 4,
		func(_ context.Context, _ int, job runJob) (int, error) {
			switch job.caseIndex {
			case 0:
				<-jobThreeFailed // the slow run finishes after the failure
			case 3:
				close(jobThreeFailed)
				return 0, errRunFailed
			}
			return job.caseIndex, nil
		},
		func(job runJob, _ int) (RunResult, error) {
			journaled = append(journaled, job.caseIndex)
			return RunResult{}, nil
		})
	if !errors.Is(err, errRunFailed) {
		t.Fatalf("err = %v, want the failed run's own error", err)
	}
	if want := []int{0, 1, 2}; !slices.Equal(journaled, want) {
		t.Fatalf("journaled %v, want every run before the failure (%v) and none after it", journaled, want)
	}
}

// Two failures report the earlier run's, whichever arrives first, so the
// error is the one a single worker would have stopped at.
func TestScoreConcurrentlyReportsTheEarliestFailedRun(t *testing.T) {
	errEarly, errLate := errors.New("run 1 failed"), errors.New("run 3 failed")
	lateFailed := make(chan struct{})
	var journaled []int
	_, err := scoreConcurrently(context.Background(), numberedJobs(4), 4,
		func(_ context.Context, _ int, job runJob) (int, error) {
			switch job.caseIndex {
			case 1:
				<-lateFailed
				return 0, errEarly
			case 3:
				close(lateFailed)
				return 0, errLate
			}
			return job.caseIndex, nil
		},
		func(job runJob, _ int) (RunResult, error) {
			journaled = append(journaled, job.caseIndex)
			return RunResult{}, nil
		})
	if !errors.Is(err, errEarly) {
		t.Fatalf("err = %v, want run 1's, the earliest by job order", err)
	}
	if !slices.Equal(journaled, []int{0}) {
		t.Fatalf("journaled %v, want only the run before the earliest failure", journaled)
	}
}

func TestScoreConcurrentlyReportsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := scoreConcurrently(ctx, []runJob{{run: 1}, {run: 2}}, 2,
		func(ctx context.Context, _ int, _ runJob) (int, error) { return 0, ctx.Err() },
		func(runJob, int) (RunResult, error) { return RunResult{}, nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
}

func TestParallelOrDefault(t *testing.T) {
	for in, want := range map[int]int{0: defaultParallel, 1: 1, 8: 8} {
		if got, err := parallelOrDefault(in); err != nil || got != want {
			t.Errorf("parallelOrDefault(%d) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := parallelOrDefault(-1); err == nil {
		t.Error("parallelOrDefault(-1) accepted a negative count")
	}
}
