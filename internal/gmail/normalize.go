package gmail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Normalized struct {
	Message  mail.Message `json:"message"`
	LabelIDs []string     `json:"label_ids"`
	Notes    []string     `json:"notes"`
}

func SourceID(connectionID, providerID string) string { return connectionID + ":" + providerID }
func header(headers []Header, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}
func decodedHeader(value string) string {
	d := mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
	result, err := d.DecodeHeader(value)
	if err != nil {
		return value
	}
	return result
}

func HTMLText(data string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(data))
	var out strings.Builder
	ignored := 0
	for {
		token := tokenizer.Next()
		if token == html.ErrorToken {
			break
		}
		switch token {
		case html.StartTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == "style" || tag == "head" {
				ignored++
			}
			if ignored == 0 && (tag == "p" || tag == "br" || tag == "div" || tag == "li" || tag == "tr") {
				out.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if (tag == "script" || tag == "style" || tag == "head") && ignored > 0 {
				ignored--
			}
			if ignored == 0 && (tag == "p" || tag == "div" || tag == "li" || tag == "tr") {
				out.WriteByte('\n')
			}
		case html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			if ignored == 0 && string(name) == "br" {
				out.WriteByte('\n')
			}
		case html.TextToken:
			if ignored == 0 {
				out.Write(tokenizer.Text())
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func decodeBody(part Part, limit int64) (string, bool, error) {
	encoding := base64.RawURLEncoding
	if strings.HasSuffix(part.Body.Data, "=") {
		encoding = base64.URLEncoding
	}
	var reader io.Reader = base64.NewDecoder(encoding, strings.NewReader(part.Body.Data))
	_, params, err := mime.ParseMediaType(header(part.Headers, "Content-Type"))
	if err == nil && params["charset"] != "" {
		reader, err = charset.NewReaderLabel(params["charset"], reader)
		if err != nil {
			return "", false, errors.New("unsupported body charset")
		}
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return "", false, errors.New("body encoding is unavailable")
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
		for removed := 0; removed < utf8.UTFMax && len(data) > 0 && !utf8.Valid(data); removed++ {
			data = data[:len(data)-1]
		}
	}
	if !utf8.Valid(data) {
		return "", false, errors.New("body text is not valid UTF-8 after decoding")
	}
	return string(data), truncated, nil
}

func Normalize(connectionID string, raw APIMessage, scope string, asOf time.Time, limits config.Limits) (Normalized, error) {
	out := Normalized{LabelIDs: raw.LabelIDs, Notes: []string{}}
	if !files.ValidID(connectionID) || !providerID(raw.ID) || !providerID(raw.ThreadID) {
		return out, errors.New("invalid normalized source identity")
	}
	stamp, err := strconv.ParseInt(raw.InternalDate, 10, 64)
	if err != nil {
		return out, errors.New("Gmail message has no usable received timestamp")
	}
	received := time.UnixMilli(stamp)
	if received.After(asOf) {
		return out, errors.New("Gmail message is newer than the pinned as-of time")
	}
	m := mail.Message{ID: SourceID(connectionID, raw.ID), ThreadID: SourceID(connectionID, raw.ThreadID), ReceivedAt: received.UTC().Format(time.RFC3339Nano), Scope: scope, Channel: "gmail", From: decodedHeader(header(raw.Payload.Headers, "From")), Subject: decodedHeader(header(raw.Payload.Headers, "Subject")), ContentStatus: "complete", Attachments: []mail.Attachment{}}
	plain, rich := []string{}, []string{}
	count := 0
	unavailable, truncated := false, false
	var visit func(Part, int, string)
	visit = func(part Part, depth int, path string) {
		count++
		if depth > DefaultMaxMIMEDepth || count > DefaultMaxMIMEParts {
			unavailable = true
			out.Notes = append(out.Notes, "MIME structure exceeds the supported depth or part count")
			return
		}
		disposition := strings.ToLower(header(part.Headers, "Content-Disposition"))
		attachment := part.Filename != "" || strings.HasPrefix(disposition, "attachment")
		if attachment || (!strings.HasPrefix(part.MIME, "multipart/") && part.MIME != "text/plain" && part.MIME != "text/html" && part.MIME != "") {
			id := part.Body.AttachmentID
			if id == "" {
				id = files.Digest([]byte(path + "/" + part.PartID + "/" + part.Filename))
			}
			m.Attachments = append(m.Attachments, mail.Attachment{ID: id, Name: part.Filename, MIME: part.MIME, Status: "unsupported"})
			return
		}
		if part.MIME == "text/plain" || part.MIME == "text/html" {
			if part.Body.AttachmentID != "" && part.Body.Data == "" {
				unavailable = true
				out.Notes = append(out.Notes, "text body requires an attachment download outside the current policy")
				return
			}
			text, cut, e := decodeBody(part, limits.MaxSourceBytes)
			if e != nil {
				unavailable = true
				out.Notes = append(out.Notes, e.Error())
				return
			}
			truncated = truncated || cut
			if part.MIME == "text/plain" {
				plain = append(plain, text)
			} else {
				rich = append(rich, HTMLText(text))
			}
		}
		for i, p := range part.Parts {
			visit(p, depth+1, fmt.Sprintf("%s/%d", path, i))
		}
	}
	visit(raw.Payload, 0, "root")
	if len(plain) > 0 {
		m.Body = strings.Join(plain, "\n\n")
	} else {
		m.Body = strings.Join(rich, "\n\n")
	}
	if int64(len(m.Body)) > limits.MaxSourceBytes {
		body := []byte(m.Body)[:limits.MaxSourceBytes]
		for removed := 0; removed < utf8.UTFMax && len(body) > 0 && !utf8.Valid(body); removed++ {
			body = body[:len(body)-1]
		}
		m.Body = string(body)
		truncated = true
	}
	if len(plain) == 0 && len(rich) == 0 {
		unavailable = true
		out.Notes = append(out.Notes, "no supported text body is available")
	}
	if truncated {
		m.ContentStatus = "truncated"
	}
	if unavailable {
		m.ContentStatus = "unavailable"
	}
	out.Message = m
	return out, nil
}
