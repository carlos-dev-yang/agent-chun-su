package reception

import (
	"context"
	"errors"

	"chunsu/internal/chatlanguage"
	"chunsu/internal/chatstyle"
	"chunsu/internal/executor"
)

const Acknowledgment = "Received."

// FailureMessage is transport-neutral public copy. It never returns a raw
// host, executor, or service-manager error.
func FailureMessage(err error) string {
	var replyLanguage *chatlanguage.Error
	if errors.As(err, &replyLanguage) {
		return "Saved reply-language settings are unavailable. Use /language reset to restore automatic reply language."
	}
	var style *chatstyle.Error
	if errors.As(err, &style) {
		return "Saved tone settings are unavailable. Use /tone reset to restore the default tone."
	}
	if errors.Is(err, ErrUncertain) {
		return "The previous request or action result could not be confirmed. It was not retried automatically. Check /jobs and /errors."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The reply reached its time limit and was not completed. Check /jobs for work that may already have started."
	}
	var action *ActionError
	if errors.As(err, &action) {
		return "An internal action could not be completed. It was not retried automatically. Check /errors and /status."
	}
	var compatibility *executor.CompatibilityError
	if errors.As(err, &compatibility) {
		return "The reply could not be generated because of an AI executor compatibility problem. " + compatibility.Error() + " Run chunsu doctor on the host."
	}
	return "The request could not be completed. Check /errors for the cause and recovery guidance. /status and /help remain available."
}
