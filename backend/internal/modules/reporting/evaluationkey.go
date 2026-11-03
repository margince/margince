// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Captions may change without changing figures, scope or source identity.
// Clone slices before clearing prose so the response retains its explanations.
func evaluationKey(evaluation Evaluation) (string, error) {
	evaluation.Result.EvaluationKey = ""
	evaluation.Result.Context.ProseLanguage = nil
	evaluation.Result.Metrics = slices.Clone(evaluation.Result.Metrics)
	for i := range evaluation.Result.Metrics {
		evaluation.Result.Metrics[i].Coverage.Reason = nil
	}
	evaluation.Result.Charts = slices.Clone(evaluation.Result.Charts)
	for i := range evaluation.Result.Charts {
		chart := &evaluation.Result.Charts[i]
		chart.Coverage.Reason = nil
		chart.Points = slices.Clone(chart.Points)
		for j := range chart.Points {
			chart.Points[j].Label = ""
		}
		if chart.CaptureStatus != nil {
			status := *chart.CaptureStatus
			if status.Failure != nil {
				blank := ""
				status.Failure = &blank
			}
			chart.CaptureStatus = &status
		}
	}
	result, err := canonicalResult(evaluation.Result)
	if err != nil {
		return "", err
	}
	// Facts are a set: a reader may return its rows in any order. A fact holds
	// no list, so its plain encoding is already canonical and stays cheap at
	// the fact limit; TestAFactHoldsNoListTheReceiptWouldHashTwoWays holds that.
	facts := make([][]byte, len(evaluation.Facts))
	for i, fact := range evaluation.Facts {
		fact.Row.Label, fact.StageLabel, fact.OwnerLabel = "", "", ""
		if facts[i], err = json.Marshal(fact); err != nil {
			return "", err
		}
	}
	slices.SortFunc(facts, bytes.Compare)
	digest := sha256.New()
	digest.Write(result)
	for _, fact := range facts {
		digest.Write([]byte{'\n'})
		digest.Write(fact)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// resultSets sorts the lists of a result whose order means nothing to the
// figures. The response keeps its own order, so a chart still draws in order.
func resultSets(tree map[string]any) error {
	if frame, ok := tree["context"].(map[string]any); ok {
		if err := sortSet(frame, "member_ids"); err != nil {
			return err
		}
	}
	charts, _ := tree["charts"].([]any)
	for _, chart := range charts {
		if chart, ok := chart.(map[string]any); ok {
			if err := sortSet(chart, "points"); err != nil {
				return err
			}
		}
	}
	return nil
}

// canonicalResult encodes a result so that a nil and an empty list, or a null
// and an absent field, read the same. encoding/json sorts the object keys.
func canonicalResult(result crmcontracts.ReportingEvaluation) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	tree = pruneEmpty(tree)
	if object, ok := tree.(map[string]any); ok {
		if err := resultSets(object); err != nil {
			return nil, err
		}
	}
	return json.Marshal(tree)
}

// pruneEmpty drops nulls and empty lists from a decoded JSON tree.
//
//craft:ignore naked-any a decoded JSON tree has no static type.
func pruneEmpty(node any) any {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			child = pruneEmpty(child)
			if list, ok := child.([]any); child == nil || (ok && len(list) == 0) {
				delete(value, key)
				continue
			}
			value[key] = child
		}
	case []any:
		for i := range value {
			value[i] = pruneEmpty(value[i])
		}
	}
	return node
}

// sortSet orders a list by each member's whole encoding, so two members that
// tie on an id still fall in one order.
func sortSet(object map[string]any, key string) error {
	list, ok := object[key].([]any)
	if !ok {
		return nil
	}
	type member struct {
		value   any
		encoded []byte
	}
	members := make([]member, len(list))
	for i, value := range list {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		members[i] = member{value, encoded}
	}
	slices.SortFunc(members, func(a, b member) int { return bytes.Compare(a.encoded, b.encoded) })
	for i := range members {
		list[i] = members[i].value
	}
	return nil
}
