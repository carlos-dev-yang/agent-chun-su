package reception

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/store"
)

func (h Host) jobCommand(ctx context.Context, parts []string) (string, bool, error) {
	usage := "Usage: /job ID | /job results ID | /job pause ID | /job resume ID | /job approve STEP_ID | /job retry STEP_ID ATTEMPT_ID"
	if len(parts) < 2 || len(parts) > 4 {
		return usage, true, nil
	}
	request := control.Request{}
	switch {
	case len(parts) == 2 && files.ValidID(parts[1]):
		request.Operation, request.JobID = "staged_steps", parts[1]
	case len(parts) == 3 && parts[1] == "results" && files.ValidID(parts[2]):
		request.Operation, request.JobID = "staged_results", parts[2]
	case len(parts) == 3 && (parts[1] == "pause" || parts[1] == "resume") && files.ValidID(parts[2]):
		request.Operation, request.JobID = "staged_"+parts[1], parts[2]
	case len(parts) == 3 && parts[1] == "approve" && files.ValidID(parts[2]):
		request.Operation, request.StepID = "staged_approve", parts[2]
	case len(parts) == 4 && parts[1] == "retry" && files.ValidID(parts[2]) && files.ValidID(parts[3]):
		request.Operation, request.StepID, request.AttemptID = "staged_retry", parts[2], parts[3]
	default:
		return usage, true, nil
	}
	raw, handled, err := control.Call(ctx, h.Root, time.Duration(h.Config.Limits.LockWaitSeconds)*time.Second, h.Config.Limits.MaxArtifactBytes, request)
	if err != nil {
		return "Staged job control could not be completed. Inspect the controller and job state.", true, err
	}
	if !handled {
		return "The controller is unavailable. Start it to inspect or change a staged job; chat remains available.", true, nil
	}
	switch request.Operation {
	case "staged_steps":
		var steps []store.Step
		if err = json.Unmarshal(raw, &steps); err != nil {
			return "", true, err
		}
		var lines []string
		for _, step := range steps {
			lines = append(lines, fmt.Sprintf("%s: %s · step %s · attempt %s · output %s", step.Stage, step.State, step.ID, step.CurrentAttemptID, step.OutputArtifactID))
		}
		if len(lines) == 0 {
			return "No staged steps found.", true, nil
		}
		return strings.Join(lines, "\n"), true, nil
	case "staged_results":
		var events []store.ResultEvent
		if err = json.Unmarshal(raw, &events); err != nil {
			return "", true, err
		}
		if len(events) == 0 {
			return "No result events are available yet.", true, nil
		}
		lines := []string{}
		for _, event := range events {
			lines = append(lines, fmt.Sprintf("Result %s: %s · delivery %s · artifact %s", event.ID, event.Status, event.DeliveryStatus, event.ArtifactID))
			if len(lines) >= h.Config.Limits.MaxMessages {
				break
			}
		}
		return strings.Join(lines, "\n"), true, nil
	case "staged_pause":
		return "The job is paused before its next step. Its current step may finish.", true, nil
	case "staged_resume":
		return "The job may advance when its dependencies and approvals are ready.", true, nil
	case "staged_approve":
		return "The manual checkpoint is approved for this step.", true, nil
	case "staged_retry":
		return "The failed step is ready for an explicit new attempt.", true, nil
	}
	return "", true, nil
}
