package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

var ErrNotConfigured = errors.New("Jira live reader is not configured; select a provider and connect later, or supply saved responses")

type Request struct {
	Policy Policy
	Cursor string
}

type Page struct {
	Body       []byte
	CapturedAt string
	NextCursor string
	Complete   bool
}

// Reader is host-owned. Provider paths, authentication and continuation mechanics
// end here; normalization does not receive credentials or an HTTP client.
type Reader interface {
	ReadPage(context.Context, Request) (Page, error)
}

type DisconnectedReader struct{}

func (DisconnectedReader) ReadPage(context.Context, Request) (Page, error) {
	return Page{}, ErrNotConfigured
}

type SavedPage struct {
	Cursor     string          `json:"cursor"`
	CapturedAt string          `json:"captured_at"`
	Body       json.RawMessage `json:"body"`
}

type SavedInput struct {
	Version   int         `json:"version"`
	Provider  string      `json:"provider"`
	Format    string      `json:"format"`
	Synthetic bool        `json:"synthetic"`
	Policy    Policy      `json:"policy"`
	Pages     []SavedPage `json:"pages"`
}

type SavedReader struct {
	Input  SavedInput
	Digest string
}

func NewSavedReader(data []byte, limits config.Limits) (*SavedReader, error) {
	if limits.MaxSourceBytes <= 0 || limits.MaxArtifactBytes <= 0 || limits.MaxEvidenceBytes <= 0 {
		return nil, errors.New("Jira byte budgets must be positive")
	}
	if int64(len(data)) > limits.MaxEvidenceBytes {
		return nil, errors.New("saved Jira response bundle exceeds evidence budget")
	}
	var input SavedInput
	if err := decode(data, &input); err != nil {
		return nil, err
	}
	if input.Version != Version || input.Provider != Provider || (input.Format != CloudV3 && input.Format != RESTV2) || len(input.Pages) == 0 {
		return nil, errors.New("saved responses require version 1, provider jira, cloud-v3 or rest-v2 format, and at least one page")
	}
	if err := input.Policy.Validate(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, page := range input.Pages {
		if seen[page.Cursor] {
			return nil, errors.New("duplicate saved page cursor")
		}
		seen[page.Cursor] = true
		if _, err := timestamp(page.CapturedAt); err != nil {
			return nil, errors.New("each saved page requires its capture timestamp")
		}
		if int64(len(page.Body)) > limits.MaxArtifactBytes {
			return nil, errors.New("saved page exceeds response byte budget")
		}
		if _, _, err := pageContinuation(page.Body, input.Format); err != nil {
			return nil, err
		}
		if input.Format == RESTV2 {
			var offset struct {
				Start int `json:"startAt"`
			}
			_ = json.Unmarshal(page.Body, &offset)
			expected := strconv.Itoa(offset.Start)
			if offset.Start == 0 {
				expected = ""
			}
			if page.Cursor != expected {
				return nil, errors.New("saved rest-v2 cursor does not match startAt")
			}
		}
	}
	if !seen[""] {
		return nil, errors.New("saved responses require an initial empty cursor")
	}
	return &SavedReader{Input: input, Digest: files.Digest(data)}, nil
}

func (r *SavedReader) ReadPage(ctx context.Context, req Request) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if err := req.Policy.Validate(); err != nil {
		return Page{}, err
	}
	if req.Policy.ConnectionID != r.Input.Policy.ConnectionID {
		return Page{}, errors.New("saved reader connection mismatch")
	}
	for _, page := range r.Input.Pages {
		if page.Cursor == req.Cursor {
			next, complete, err := pageContinuation(page.Body, r.Input.Format)
			return Page{Body: append([]byte(nil), page.Body...), CapturedAt: page.CapturedAt, NextCursor: next, Complete: complete}, err
		}
	}
	return Page{}, errors.New("saved continuation is unavailable; the preserved source bundle must not be silently replaced")
}

func pageContinuation(data []byte, format string) (string, bool, error) {
	var page struct {
		Issues json.RawMessage `json:"issues"`
		Next   string          `json:"nextPageToken"`
		Last   *bool           `json:"isLast"`
		Start  *int            `json:"startAt"`
		Total  *int            `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return "", false, errors.New("invalid Jira page JSON")
	}
	var issues []json.RawMessage
	if len(page.Issues) == 0 || string(page.Issues) == "null" || json.Unmarshal(page.Issues, &issues) != nil {
		return "", false, errors.New("Jira page must contain an issues array")
	}
	switch format {
	case CloudV3:
		if page.Last != nil && *page.Last {
			if page.Next != "" {
				return "", false, errors.New("conflicting cloud page continuation")
			}
			return "", true, nil
		}
		return page.Next, false, nil
	case RESTV2:
		if page.Start == nil || page.Total == nil || *page.Start < 0 || *page.Total < 0 {
			return "", false, errors.New("rest-v2 pages require nonnegative startAt and total")
		}
		if *page.Start > *page.Total || len(issues) > *page.Total-*page.Start {
			return "", false, errors.New("rest-v2 page exceeds its declared total")
		}
		next := *page.Start + len(issues)
		if next >= *page.Total {
			return "", true, nil
		}
		if len(issues) == 0 {
			return "", false, nil
		}
		return strconv.Itoa(next), false, nil
	default:
		return "", false, fmt.Errorf("unsupported saved Jira response format")
	}
}
