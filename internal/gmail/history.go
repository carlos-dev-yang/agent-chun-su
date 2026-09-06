package gmail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"chunsu/internal/mail"
	"chunsu/internal/store"
)

// PriorReports admits only earlier interpretations whose complete source set is
// already in the new envelope. It never widens Gmail scope to retrieve history.
func (c Collector) PriorReports(ctx context.Context, snapshot mail.Snapshot) ([]mail.Interpretation, []string, error) {
	out := []mail.Interpretation{}
	gaps := []string{}
	if snapshot.Origin == nil {
		return out, gaps, nil
	}
	sources := map[string]bool{}
	for _, m := range snapshot.Messages {
		sources[m.ID] = true
	}
	jobIDs := map[string]bool{}
	for _, message := range snapshot.Messages {
		var job string
		err := c.Store.DB.QueryRowContext(ctx, "SELECT job_id FROM source_coverage WHERE connection_id=? AND source_id=?", snapshot.Origin.ConnectionID, message.ID).Scan(&job)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, gaps, err
		}
		jobIDs[job] = true
	}
	asOf, err := time.Parse(time.RFC3339Nano, snapshot.AsOf)
	if err != nil {
		return out, gaps, err
	}
	var total int64
	ordered := make([]string, 0, len(jobIDs))
	for id := range jobIDs {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, jobID := range ordered {
		j, err := c.Store.Job(ctx, jobID)
		if err != nil {
			return out, gaps, err
		}
		if j.Status != store.Completed && j.Status != store.Partial {
			gaps = append(gaps, "previous report content is unavailable: "+jobID)
			continue
		}
		artifacts, err := c.Store.Artifacts(ctx, jobID)
		if err != nil {
			return out, gaps, err
		}
		var report mail.Report
		for _, art := range artifacts {
			if art.AttemptID == j.CurrentAttempt && art.Kind == "raw_result" {
				b, e := c.Store.ReadArtifact(art, c.Config.Limits.MaxArtifactBytes)
				if e != nil {
					return out, gaps, e
				}
				if e = mail.Decode(b, &report); e != nil {
					return out, gaps, e
				}
				break
			}
		}
		if report.Version != mail.Version {
			return out, gaps, errors.New("covered report has no supported preserved result")
		}
		previous, e := time.Parse(time.RFC3339Nano, report.AsOf)
		if e != nil {
			return out, gaps, e
		}
		if previous.After(asOf) {
			continue
		}
		for _, item := range report.Items {
			complete := len(item.Sources) > 0
			for _, source := range item.Sources {
				complete = complete && sources[source]
			}
			if !complete {
				continue
			}
			text, e := json.Marshal(item)
			if e != nil {
				return out, gaps, e
			}
			if int64(len(text)) > c.Config.Limits.MaxSourceBytes || int64(len(text)) > c.Config.Limits.MaxArtifactBytes-total || len(out) >= c.Config.Limits.MaxMessages {
				gaps = append(gaps, "prior interpretation history reached its configured limit")
				return out, gaps, nil
			}
			total += int64(len(text))
			out = append(out, mail.Interpretation{AsOf: report.AsOf, SourceIDs: item.Sources, Text: string(text), Kind: "previous_report"})
		}
	}
	return out, gaps, nil
}
