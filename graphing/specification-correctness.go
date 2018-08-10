package graphing

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	graph "github.com/johnnadratowski/golang-neo4j-bolt-driver/structures/graph"
	fi "github.com/numbleroot/nemo/faultinjectors"
)

// Structs.

// CorrectionsPair
type CorrectionsPair struct {
	Rule *fi.Rule
	Goal *fi.Goal
}

// Dependency
type Dependency struct {
	Rule string
	Time uint
}

// Functions.

// findAsyncEvents
func (n *Neo4J) findAsyncEvents(failedRun uint, msgs []*fi.Message) ([]*CorrectionsPair, []*CorrectionsPair, error) {

	diffRunID := 2000 + failedRun

	// Determine if there is non-triviality (i.e., async events)
	// in the failed run's precondition provenance.
	stmtPreAsync, err := n.Conn1.PrepareNeo(`
		MATCH path = (root {run: {run}, condition: "pre"})-[*0..]->(r1:Rule {run: {run}, condition: "pre", type: "async"})-[*0..]->(r2:Rule {run: {run}, condition: "pre", type: "async"})
		WHERE NOT ()-->(root)
		WITH r2, collect(r1) AS history

		MATCH (g:Goal)-[*1]->(r2)
		WITH r2, g, history
		RETURN r2 AS rule, g AS goal, filter(_ IN history WHERE size(history) = 1) AS history;
    `)
	if err != nil {
		return nil, nil, err
	}

	preAsyncsRaw, err := stmtPreAsync.QueryNeo(map[string]interface{}{
		"run": failedRun,
	})
	if err != nil {
		return nil, nil, err
	}

	preAsyncs := make([]*CorrectionsPair, 0, 5)

	for err == nil {

		var preAsync []interface{}

		preAsync, _, err = preAsyncsRaw.NextNeo()
		if err != nil && err != io.EOF {
			return nil, nil, err
		} else if err == nil {

			history := preAsync[2].([]interface{})

			// We only consider results that are not subsumed
			// by other results.
			if len(history) == 1 {

				var sender string

				// Type-assert the two nodes into well-defined struct.
				rule := preAsync[0].(graph.Node)
				goal := preAsync[1].(graph.Node)

				// Provide raw name excluding "_provX" ending.
				ruleLabelParts := strings.Split(rule.Properties["label"].(string), "_")
				rule.Properties["label"] = strings.Join(ruleLabelParts[:(len(ruleLabelParts)-1)], "_")

				// Parse parts that make up label of goal.
				goalLabel := strings.TrimLeft(goal.Properties["label"].(string), goal.Properties["table"].(string))
				goalLabel = strings.Trim(goalLabel, "()")
				goalLabelParts := strings.Split(goalLabel, ", ")

				for m := range msgs {

					t, err := strconv.ParseUint(goal.Properties["time"].(string), 10, 32)
					if err != nil {
						return nil, nil, err
					}

					if (msgs[m].Content == rule.Properties["label"].(string)) &&
						(msgs[m].RecvNode == goalLabelParts[0]) &&
						(msgs[m].RecvTime == uint(t)) {
						sender = msgs[m].SendNode
					}
				}

				// Append to slice of correction pairs.
				preAsyncs = append(preAsyncs, &CorrectionsPair{
					Rule: &fi.Rule{
						ID:    rule.Properties["id"].(string),
						Label: rule.Properties["label"].(string),
						Table: rule.Properties["table"].(string),
						Type:  rule.Properties["type"].(string),
					},
					Goal: &fi.Goal{
						ID:        goal.Properties["id"].(string),
						Label:     goal.Properties["label"].(string),
						Table:     goal.Properties["table"].(string),
						Time:      goal.Properties["time"].(string),
						CondHolds: goal.Properties["condition_holds"].(bool),
						Sender:    sender,
						Receiver:  goalLabelParts[0],
					},
				})
			}
		}
	}

	err = preAsyncsRaw.Close()
	if err != nil {
		return nil, nil, err
	}

	err = stmtPreAsync.Close()
	if err != nil {
		return nil, nil, err
	}

	// Determine if there is non-triviality (i.e., async events)
	// in differential postcondition provenance.
	stmtDiffAsync, err := n.Conn1.PrepareNeo(`
		MATCH path = (root {run: {run}, condition: "post"})-[*0..]->(r1:Rule {run: {run}, condition: "post", type: "async"})-[*0..]->(r2:Rule {run: {run}, condition: "post", type: "async"})
		WHERE NOT ()-->(root)
		WITH r2, collect(r1) AS history

		MATCH (g:Goal)-[*1]->(r2)
		WITH r2, g, history
		RETURN r2 AS rule, g AS goal, filter(_ IN history WHERE size(history) = 1) AS history;
    `)
	if err != nil {
		return nil, nil, err
	}

	diffAsyncsRaw, err := stmtDiffAsync.QueryNeo(map[string]interface{}{
		"run": diffRunID,
	})
	if err != nil {
		return nil, nil, err
	}

	diffAsyncs := make([]*CorrectionsPair, 0, 5)

	for err == nil {

		var diffAsync []interface{}

		diffAsync, _, err = diffAsyncsRaw.NextNeo()
		if err != nil && err != io.EOF {
			return nil, nil, err
		} else if err == nil {

			history := diffAsync[2].([]interface{})

			if len(history) == 1 {

				// Type-assert the two nodes into well-defined struct.
				rule := diffAsync[0].(graph.Node)
				goal := diffAsync[1].(graph.Node)

				// Provide raw name excluding "_provX" ending.
				ruleLabelParts := strings.Split(rule.Properties["label"].(string), "_")
				rule.Properties["label"] = strings.Join(ruleLabelParts[:(len(ruleLabelParts)-1)], "_")

				// Parse parts that make up label of goal.
				goalLabel := strings.TrimLeft(goal.Properties["label"].(string), goal.Properties["table"].(string))
				goalLabel = strings.Trim(goalLabel, "()")
				goalLabelParts := strings.Split(goalLabel, ", ")

				// Append to slice of correction pairs.
				diffAsyncs = append(diffAsyncs, &CorrectionsPair{
					Rule: &fi.Rule{
						ID:    rule.Properties["id"].(string),
						Label: rule.Properties["label"].(string),
						Table: rule.Properties["table"].(string),
						Type:  rule.Properties["type"].(string),
					},
					Goal: &fi.Goal{
						ID:        goal.Properties["id"].(string),
						Label:     goal.Properties["label"].(string),
						Table:     goal.Properties["table"].(string),
						Time:      goal.Properties["time"].(string),
						CondHolds: goal.Properties["condition_holds"].(bool),
						Receiver:  goalLabelParts[0],
					},
				})
			}
		}
	}

	err = diffAsyncsRaw.Close()
	if err != nil {
		return nil, nil, err
	}

	err = stmtDiffAsync.Close()
	if err != nil {
		return nil, nil, err
	}

	return preAsyncs, diffAsyncs, nil
}

// findTriggerEvents extracts the trigger events
// that mark the transition from specified condition
// being false to being true.
func (n *Neo4J) findTriggerEvents(run uint, condition string) ([]*fi.Rule, []*CorrectionsPair, error) {

	// TODO: Refine this query. We want:
	//       Distinct aggregate nodes with
	//       all children except clock facts.
	stmtTriggers, err := n.Conn1.PrepareNeo(`
		MATCH (a:Rule {run: {run}, condition: {condition}})-[*1]->(g:Goal {run: {run}, condition: {condition}, condition_holds: true})-[*1]->(r:Rule {run: {run}, condition: {condition}})
		WHERE (r)-[*1]->(:Goal {run: {run}, condition: {condition}, condition_holds: false})
		RETURN a AS aggregation, g AS goal, r AS rule;
    `)
	if err != nil {
		return nil, nil, err
	}

	triggersRaw, err := stmtTriggers.QueryNeo(map[string]interface{}{
		"run":       run,
		"condition": condition,
	})
	if err != nil {
		return nil, nil, err
	}

	aggregations := make([]*fi.Rule, 0, 5)
	triggers := make([]*CorrectionsPair, 0, 5)

	for err == nil {

		var trigger []interface{}

		trigger, _, err = triggersRaw.NextNeo()
		if err != nil && err != io.EOF {
			return nil, nil, err
		} else if err == nil {

			// Type-assert the three nodes into well-defined struct.
			agg := trigger[0].(graph.Node)
			goal := trigger[1].(graph.Node)
			rule := trigger[2].(graph.Node)

			// Provide raw name excluding "_provX" ending.
			aggLabelParts := strings.Split(agg.Properties["label"].(string), "_")
			agg.Properties["table"] = strings.Join(aggLabelParts[:(len(aggLabelParts)-1)], "_")

			// Parse parts that make up label of goal.
			goalLabel := strings.TrimLeft(goal.Properties["label"].(string), goal.Properties["table"].(string))
			goalLabel = strings.Trim(goalLabel, "()")
			goalLabelParts := strings.Split(goalLabel, ", ")

			// Provide raw name excluding "_provX" ending.
			ruleLabelParts := strings.Split(rule.Properties["label"].(string), "_")
			rule.Properties["table"] = strings.Join(ruleLabelParts[:(len(ruleLabelParts)-1)], "_")

			// Append condition-firing aggregation rule to slice.
			aggregations = append(aggregations, &fi.Rule{
				ID:    agg.Properties["id"].(string),
				Label: agg.Properties["label"].(string),
				Table: agg.Properties["table"].(string),
				Type:  agg.Properties["type"].(string),
			})

			// Append to slice of correction pairs.
			triggers = append(triggers, &CorrectionsPair{
				Goal: &fi.Goal{
					ID:        goal.Properties["id"].(string),
					Label:     goal.Properties["label"].(string),
					Table:     goal.Properties["table"].(string),
					Time:      goal.Properties["time"].(string),
					CondHolds: goal.Properties["condition_holds"].(bool),
					Receiver:  goalLabelParts[0],
				},
				Rule: &fi.Rule{
					ID:    rule.Properties["id"].(string),
					Label: rule.Properties["label"].(string),
					Table: rule.Properties["table"].(string),
					Type:  rule.Properties["type"].(string),
				},
			})
		}
	}

	err = triggersRaw.Close()
	if err != nil {
		return nil, nil, err
	}

	err = stmtTriggers.Close()
	if err != nil {
		return nil, nil, err
	}

	return aggregations, triggers, nil
}

// GenerateCorrections analyzes the available precondition
// provenance of the successful run and the postcondition
// provenance of the difference between successful and
// unsuccessful postcondition provenance. It then synthesizes
// correction suggestions based on making the precondition
// stricter by increasing dependencies.
func (n *Neo4J) GenerateCorrections() ([]string, error) {

	// TODO: Rework this, pretty shaky right now.

	recs := make([]string, 0, 6)
	considered := make(map[string]bool)

	// Collect all leaf nodes of the branches originating
	// in precondition root nodes of the good execution and
	// terminating one level after the precondition was
	// marked as achieved.
	preAgg, preTriggers, err := n.findTriggerEvents(0, "pre")
	if err != nil {
		return nil, err
	}

	var preTriggerRules string
	for i := range preTriggers {

		if preTriggerRules == "" {
			preTriggerRules = fmt.Sprintf("%s(...)", preTriggers[i].Rule.Table)
		} else {
			preTriggerRules = fmt.Sprintf(", %s(...)", preTriggers[i].Rule.Table)
		}

		// Mark as considered.
		considered[preTriggers[i].Rule.Table] = true
	}

	// Collect all leaf nodes of the differential
	// postcondition provenance (good - bad) originating
	// in the root nodes of the differential provenance
	// and terminating one level after the postcondition
	// was marked as achieved.
	_, postTriggers, err := n.findTriggerEvents(0, "post")
	if err != nil {
		return nil, err
	}

	var postTriggerRules string
	for i := range postTriggers {

		if !considered[postTriggers[i].Rule.Table] {

			if postTriggerRules == "" {
				postTriggerRules = fmt.Sprintf("%s(...)", postTriggers[i].Rule.Table)
			} else {
				postTriggerRules = fmt.Sprintf(", %s(...)", postTriggers[i].Rule.Table)
			}

			// Mark as considered.
			considered[postTriggers[i].Rule.Table] = true
		}
	}

	// Relate these two sets such that the precondition
	// leafs depend on having knowledge about the differential
	// nodes taking place.
	correction := fmt.Sprintf("<code>%s(...) := %s;</code> &nbsp; <i class = \"fas fa-long-arrow-alt-right\"></i> &nbsp; <code>%s(...) := %s, %s;</code>", preAgg[0].Table, preTriggerRules, preAgg[0].Table, preTriggerRules, postTriggerRules)

	recs = append(recs, "A fault occurred. Let's try making the protocol correct first. Change:")
	recs = append(recs, correction)

	return recs, nil
}

// GenerateCorrections
func (n *Neo4J) GenerateCorrectionsOld(failedRuns []uint, msgs [][]*fi.Message) ([][]string, error) {

	fmt.Printf("Running generation of suggestions for corrections (pre ~> post)... ")

	// Prepare final slices to return.
	allCorrections := make([][]string, len(failedRuns))

	for i := range failedRuns {

		// Create local slices.
		corrections := make([]string, 0, 6)

		// Retrieve non-trivial events in pre and diffprov post.
		preAsyncs, diffAsyncs, err := n.findAsyncEvents(failedRuns[i], msgs[i])
		if err != nil {
			return nil, err
		}

		if len(preAsyncs) < 1 {

			// No message passing events in precondition provenance.

			if len(diffAsyncs) < 1 {
				// No message passing events in differential postcondition provenance.
				// TODO: Discuss this and remove last correction appending.
				corrections = append(corrections, "<pre>[Precondition]</pre> No message passing events required.")
				corrections = append(corrections, "<pre>[Postcondition]</pre> No message passing events left.")
				corrections = append(corrections, "Yet we saw a fault occuring. <span style = \"font-weight: bold;\">Discuss: What are the use cases?</span>")
			} else {

				diffAsyncsLabel := fmt.Sprintf("<code>%s</code> @ <code>%s</code>", diffAsyncs[0].Rule.Label, diffAsyncs[0].Goal.Time)
				for j := 1; j < len(diffAsyncs); j++ {
					diffAsyncsLabel = fmt.Sprintf("%s, <code>%s</code> @ <code>%s</code>", diffAsyncsLabel, diffAsyncs[j].Rule.Label, diffAsyncs[j].Goal.Time)
				}

				// At least one message passing event in differential postcondition provenance.
				corrections = append(corrections, "<pre>[Precondition]</pre> No message passing events required.")
				corrections = append(corrections, fmt.Sprintf("<pre>[Postcondition]</pre> Latest message passing events still missing: %s", diffAsyncsLabel))
				corrections = append(corrections, "<span style = \"font-weight: bold;\">Suggestion: Introduce more fault-tolerance through replication and retries.</span>")
			}
		} else {

			// At least one message passing event in precondition provenance.

			preAsyncsLabel := fmt.Sprintf("<code>%s</code> @ <code>%s</code>", preAsyncs[0].Rule.Label, preAsyncs[0].Goal.Time)
			for j := 1; j < len(preAsyncs); j++ {
				preAsyncsLabel = fmt.Sprintf("%s, <code>%s</code> @ <code>%s</code>", preAsyncsLabel, preAsyncs[j].Rule.Label, preAsyncs[j].Goal.Time)
			}

			if len(diffAsyncs) < 1 {
				// No message passing events in differential postcondition provenance.
				// TODO: Discuss this and remove last correction appending.
				corrections = append(corrections, fmt.Sprintf("<pre>[Precondition]</pre> Latest message passing events required: %s", preAsyncsLabel))
				corrections = append(corrections, "<pre>[Postcondition]</pre> No message passing events left.")
				corrections = append(corrections, "Yet we saw a fault occuring. <span style = \"font-weight: bold;\">Discuss: What are the use cases?</span>")
			} else {

				diffAsyncsLabel := fmt.Sprintf("<code>%s</code> @ <code>%s</code>", diffAsyncs[0].Rule.Label, diffAsyncs[0].Goal.Time)
				for j := 1; j < len(diffAsyncs); j++ {
					diffAsyncsLabel = fmt.Sprintf("%s, <code>%s</code> @ <code>%s</code>", diffAsyncsLabel, diffAsyncs[j].Rule.Label, diffAsyncs[j].Goal.Time)
				}

				// At least one message passing event in differential postcondition provenance.
				corrections = append(corrections, fmt.Sprintf("<pre>[Precondition]</pre> Latest message passing events required: %s", preAsyncsLabel))
				corrections = append(corrections, fmt.Sprintf("<pre>[Postcondition]</pre> Latest message passing events still missing: %s", diffAsyncsLabel))

				for j := range preAsyncs {

					updDeps := make(map[string]Dependency)

					for d := range diffAsyncs {

						// Check if we have to suggest introduction of
						// intermediate round-trips because the sender of
						// the pre async event is a different one from the
						// diff-post one.
						if preAsyncs[j].Goal.Sender != diffAsyncs[d].Goal.Receiver {

							t, err := strconv.ParseUint(diffAsyncs[d].Goal.Time, 10, 32)
							if err != nil {
								return nil, err
							}

							// Create new internal rule.
							intAckRule := fmt.Sprintf("int_ack_%s(%s, node, ...)", diffAsyncs[d].Goal.Table, preAsyncs[j].Goal.Sender)

							// If so, add a suggestion for an internal
							// acknowledgement round to pre async receiver.
							corrections = append(corrections, fmt.Sprintf("<span style = \"font-weight: bold;\">Suggestion:</span> <code>%s</code> needs to know that <code>%s</code> received <code>%s</code>.<br /> &nbsp; &nbsp; &nbsp; &nbsp; Add internal acknowledgement: <code>%s@async :- %s(node, ...)</code> @ <code>%s</code>;", preAsyncs[j].Goal.Sender, diffAsyncs[d].Goal.Receiver, diffAsyncs[d].Goal.Label, intAckRule, diffAsyncs[d].Rule.Label, diffAsyncs[d].Goal.Time))

							// Add intermediate ack to dependencies.
							updDeps[diffAsyncs[d].Rule.Label] = Dependency{
								Rule: intAckRule,
								Time: (uint(t) + 1),
							}
						} else {

							t, err := strconv.ParseUint(diffAsyncs[d].Goal.Time, 10, 32)
							if err != nil {
								return nil, err
							}

							// Otherwise, add the original diffprov rule.
							updDeps[diffAsyncs[d].Rule.Label] = Dependency{
								Rule: diffAsyncs[d].Rule.Label,
								Time: uint(t),
							}
						}
					}

					var maxTime uint = 0
					var updDiffAsyncsLabel string
					for d := range updDeps {

						if updDeps[d].Time > maxTime {
							maxTime = updDeps[d].Time
						}

						if updDiffAsyncsLabel == "" {
							updDiffAsyncsLabel = fmt.Sprintf("<code>%s</code> @ <code>%d</code>", updDeps[d].Rule, updDeps[d].Time)
						} else {
							updDiffAsyncsLabel = fmt.Sprintf("%s, <code>%s</code> @ <code>%d</code>", updDiffAsyncsLabel, updDeps[d].Rule, updDeps[d].Time)
						}
					}

					// Determine dependency suggestions for identified pre rules.
					corrections = append(corrections, fmt.Sprintf("<span style = \"font-weight: bold;\">Suggestion:</span> Augment the conditions under which <code>%s</code> fires:<br /> &nbsp; &nbsp; &nbsp; &nbsp; <code>%s(%s, ...)@async :- </code>%s, &nbsp; <code>EXISTING_DEPENDENCIES</code>;", preAsyncs[j].Rule.Label, preAsyncs[j].Rule.Label, preAsyncs[j].Goal.Receiver, updDiffAsyncsLabel))

					// Determine the updated (delayed) time of victory declaration.
					corrections = append(corrections, fmt.Sprintf("<span style = \"font-weight: bold;\">Timing:</span> Earliest time for safely firing <code>%s</code>: <code>%d</code>", preAsyncs[j].Rule.Label, maxTime))
				}
			}
		}

		if len(corrections) == 0 {
			corrections = append(corrections, "No correction suggestions to make!")
		}

		allCorrections[i] = corrections
	}

	fmt.Printf("done\n\n")

	return allCorrections, nil
}
