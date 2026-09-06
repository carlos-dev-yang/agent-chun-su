package gmail

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ResolveQuery gives relative date operators explicit calendar semantics in the
// report timezone. Persist its result once; pagination must not move the window.
// Quoted literal text is preserved, and unsupported relative syntax is refused.
func ResolveQuery(query string, asOf time.Time, zone string) (string, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	quoted, escaped := false, false
	runes := []rune(query)
	boundary := func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("(){}", r) }
	for i := 0; i < len(runes); {
		r := runes[i]
		if escaped {
			out.WriteRune(r)
			escaped = false
			i++
			continue
		}
		if r == '\\' {
			out.WriteRune(r)
			escaped = true
			i++
			continue
		}
		if r == '"' {
			quoted = !quoted
			out.WriteRune(r)
			i++
			continue
		}
		start := i == 0 || boundary(runes[i-1]) || runes[i-1] == '-'
		if !quoted && start {
			end := i
			for end < len(runes) && !boundary(runes[end]) {
				end++
			}
			token := string(runes[i:end])
			lower := strings.ToLower(token)
			operator := ""
			prefix := ""
			if strings.HasPrefix(lower, "newer_than:") {
				operator = "after:"
				prefix = "newer_than:"
			}
			if strings.HasPrefix(lower, "older_than:") {
				operator = "before:"
				prefix = "older_than:"
			}
			if operator != "" {
				value := strings.TrimPrefix(lower, prefix)
				if len(value) < 2 {
					return "", errors.New("relative Gmail dates need a positive count and d, m or y; otherwise use explicit epoch dates")
				}
				count, e := strconv.ParseInt(value[:len(value)-1], 10, 32)
				if e != nil || count <= 0 {
					return "", errors.New("relative Gmail date count is unsupported")
				}
				var at time.Time
				switch value[len(value)-1] {
				case 'd':
					at = asOf.In(location).AddDate(0, 0, -int(count))
				case 'm':
					at = asOf.In(location).AddDate(0, -int(count), 0)
				case 'y':
					at = asOf.In(location).AddDate(-int(count), 0, 0)
				default:
					return "", errors.New("relative Gmail dates support d, m and y only")
				}
				if at.Unix() < 0 || !at.Before(asOf) {
					return "", errors.New("relative Gmail window must begin after the Unix epoch and before as-of")
				}
				out.WriteString(operator + strconv.FormatInt(at.Unix(), 10))
				i = end
				continue
			}
		}
		out.WriteRune(r)
		i++
	}
	if quoted || escaped {
		return "", errors.New("Gmail query has an unfinished quote or escape")
	}
	return "(" + out.String() + ") before:" + strconv.FormatInt(asOf.Unix(), 10), nil
}
