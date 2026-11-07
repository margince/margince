// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

// A saved revision records each metric's definition version. A metric whose
// definition changed since is withheld with a reason. It is never read under
// the new definition as if it were the one the report was saved with.

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// MetricVersions maps a metric to the definition version a revision saved it with.
type MetricVersions map[crmcontracts.ReportingMetricID]string

var definitionChangedWords = langcopy.Phrase{
	En: "How this figure is counted changed after the report was saved. Save the report again to read it under the new definition.",
	De: "Wie diese Zahl gezählt wird, hat sich geändert, nachdem der Bericht gespeichert wurde. Speichere den Bericht erneut, um sie nach der neuen Definition zu lesen.",
	Vi: "Cách tính số liệu này đã thay đổi sau khi báo cáo được lưu. Hãy lưu lại báo cáo để đọc số liệu theo định nghĩa mới.",
}

// revisionMetricVersions reads what one revision recorded. A revision saved
// before versions were kept recorded none, so nothing of it is compared.
func (s *Service) revisionMetricVersions(ctx context.Context, tx pgx.Tx, report ids.UUID, revision int64) (MetricVersions, error) {
	var raw []byte
	var b bindings
	query := "SELECT metric_versions FROM report_definition_revision WHERE report_id=" + b.add(report) + " AND revision=" + b.add(revision)
	if err := tx.QueryRow(ctx, query, b.values...).Scan(&raw); err != nil {
		return nil, storedError(err)
	}
	if raw == nil {
		return MetricVersions{}, nil
	}
	var out MetricVersions
	err := json.Unmarshal(raw, &out)
	return out, err
}

// withdrawChangedDefinitions empties every metric, chart and fact whose
// definition version differs from the one the revision recorded, and says why.
func withdrawChangedDefinitions(out *Evaluation, recorded MetricVersions) error {
	changed := []crmcontracts.ReportingMetricID{}
	reason := ExplanationCopy(out.Result.Context).Say(definitionChangedWords)
	coverage := crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusUnsupported, Reason: &reason}
	for i, metric := range out.Result.Metrics {
		if saved, ok := recorded[metric.Id]; !ok || saved == metric.Version {
			continue
		}
		changed = append(changed, metric.Id)
		out.Result.Metrics[i] = crmcontracts.ReportingMetric{Id: metric.Id, Version: metric.Version, Unit: metric.Unit, Coverage: coverage, Evidence: metric.Evidence}
	}
	if len(changed) == 0 {
		return nil
	}
	for i, chart := range out.Result.Charts {
		if slices.Contains(changed, chart.Metric) {
			out.Result.Charts[i] = crmcontracts.ReportingChart{Kind: chart.Kind, Metric: chart.Metric, ContextId: chart.ContextId, Unit: chart.Unit, Coverage: coverage, Points: []crmcontracts.ReportingPoint{}}
		}
	}
	out.Facts = slices.DeleteFunc(out.Facts, func(fact Fact) bool { return slices.Contains(changed, fact.Metric) })
	var err error
	out.Result.EvaluationKey, err = evaluationKey(*out)
	return err
}
