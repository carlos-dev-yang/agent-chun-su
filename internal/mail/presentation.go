package mail

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const SourcesArtifactKind = "sources_markdown"

func inertText(value string) string {
	var out strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func markdownText(value string) string {
	value = html.EscapeString(inertText(value))
	return strings.NewReplacer("\\", "\\\\", "!", "\\!", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "#", "\\#", "|", "\\|", ">", "\\>").Replace(value)
}

func sourceLabels(snapshot Snapshot) map[string]string {
	labels := make(map[string]string, len(snapshot.Messages))
	for i, source := range snapshot.Messages {
		labels[source.ID] = fmt.Sprintf("S%02d", i+1)
	}
	return labels
}

func categoryLabel(category string) string {
	switch category {
	case "new_work":
		return "새 업무·프로젝트 요청"
	case "interview":
		return "인터뷰·채용 일정"
	case "irregular_request":
		return "기타 요청·운영 조치"
	default:
		return category
	}
}

func displayValue(value string) string {
	labels := map[string]string{
		"completed": "보고서 생성 완료", "partial": "일부 범위만 검토", "waiting_input": "사용자 확인 필요",
		"not performed": "아직 평가하지 않음", "high": "높음", "normal": "보통", "low": "낮음",
		"unknown": "확인되지 않음", "proposed": "제안됨", "confirmed": "확정됨", "changed": "변경됨",
		"cancelled": "취소됨", "none": "별도 일정 없음", "confirmed_done": "완료 확인", "confirmed_open": "미완료 확인",
		"included": "업무에 포함", "merged": "같은 업무로 통합", "reference_only": "참고", "excluded": "요약에서 제외",
		"unresolved": "판단 보류", Target: "검토 대상", Reference: "이전 참고 자료", "complete": "본문 확인 가능",
		"unavailable": "확인할 수 없음", "truncated": "일부 본문만 제공",
		RemainingPagesGap: "추가 메일이 남아 있어 이번에 수집한 범위만 검토했습니다. 나머지는 별도로 수집해야 합니다.",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func displayTime(value, zone string) string {
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return value
	}
	return at.In(location).Format(time.DateTime + " -07:00")
}

// RenderWithSources links only host-generated labels and paths, never model URLs.
func RenderWithSources(report Report, validation Validation, snapshot Snapshot, sourceFile string) []byte {
	var out strings.Builder
	labels := sourceLabels(snapshot)
	link := func(id string) string {
		label, ok := labels[id]
		if !ok || sourceFile == "" {
			return markdownText(id)
		}
		return fmt.Sprintf("[%s](%s#%s)", label, url.PathEscape(sourceFile), strings.ToLower(label))
	}
	fmt.Fprintf(&out, "# 메일 업무 리포트\n\n기준 시각: %s (%s)\n\n처리 상태: %s · 유용성 평가: %s\n", markdownText(displayTime(report.AsOf, report.Timezone)), markdownText(report.Timezone), markdownText(displayValue(validation.OperationalStatus)), markdownText(displayValue(validation.SemanticEvaluation)))
	if snapshot.Synthetic {
		out.WriteString("\n**가상 데이터로 실행한 검증 결과입니다.**\n")
	}
	counts := map[string]int{}
	for _, disposition := range report.Dispositions {
		counts[disposition.Disposition]++
	}
	fmt.Fprintf(&out, "\n수집 메일 %d개 · 업무 항목 %d개 · 제외 %d개 · 참고 %d개 · 미확정 %d개\n\n%s\n", len(snapshot.Messages), len(report.Items), counts["excluded"], counts["reference_only"], counts["unresolved"], markdownText(report.Summary))
	if sourceFile != "" {
		fmt.Fprintf(&out, "\n[원문 모음과 전체 처리 내역](%s) — 수집 당시의 본문·출처·제외 사유를 확인할 수 있습니다.\n", url.PathEscape(sourceFile))
	}
	groups := []string{}
	for _, item := range report.Items {
		if len(item.Categories) > 0 && !Contains(groups, item.Categories[0]) {
			groups = append(groups, item.Categories[0])
		}
	}
	for _, group := range groups {
		fmt.Fprintf(&out, "\n## %s\n", markdownText(categoryLabel(group)))
		for _, item := range report.Items {
			if len(item.Categories) == 0 || item.Categories[0] != group {
				continue
			}
			categories, sources := []string{}, []string{}
			for _, category := range item.Categories {
				categories = append(categories, categoryLabel(category))
			}
			for _, id := range item.Sources {
				sources = append(sources, link(id))
			}
			schedule := []string{displayValue(item.Schedule.Status)}
			if item.Schedule.When != "" {
				schedule = append(schedule, displayTime(item.Schedule.When, report.Timezone))
			}
			schedule = append(schedule, item.Schedule.Impact)
			fmt.Fprintf(&out, "\n### %s\n\n- 분류: %s\n- 핵심 내용: %s\n- 중요도: %s — %s\n- 다음 행동: %s\n- 이전 내용·변경점: %s\n- 일정: %s\n- 완료 근거: %s — %s\n- 원문: %s\n", markdownText(item.Title), markdownText(strings.Join(categories, ", ")), markdownText(item.Reason), markdownText(displayValue(item.Importance)), markdownText(item.ImportanceReason), markdownText(item.RequestedAction), markdownText(item.History), markdownText(strings.Join(schedule, "; ")), markdownText(displayValue(item.Completion.State)), markdownText(item.Completion.Reason), strings.Join(sources, ", "))
			for _, uncertainty := range item.Uncertainties {
				fmt.Fprintf(&out, "- 확인 필요: %s\n", markdownText(uncertainty))
			}
		}
	}
	if len(report.Limitations)+len(validation.Gaps) > 0 {
		out.WriteString("\n## 범위와 한계\n\n")
		seen := map[string]bool{}
		for _, limitation := range append(append([]string{}, report.Limitations...), validation.Gaps...) {
			if !seen[limitation] {
				fmt.Fprintf(&out, "- %s\n", markdownText(displayValue(limitation)))
				seen[limitation] = true
			}
		}
	}
	if len(report.Questions) > 0 {
		out.WriteString("\n## 사용자 확인 사항\n\n")
		for _, question := range report.Questions {
			fmt.Fprintf(&out, "- %s\n", markdownText(question))
		}
	}
	return []byte(out.String())
}

// RenderSources presents the preserved normalized text, not fetched web content
// or executable Markdown. Exclusions remain auditable outside the main summary.
func RenderSources(report Report, snapshot Snapshot) []byte {
	var out strings.Builder
	labels := sourceLabels(snapshot)
	dispositions := map[string]Disposition{}
	for _, disposition := range report.Dispositions {
		dispositions[disposition.SourceID] = disposition
	}
	fmt.Fprintf(&out, "# 수집 원문과 처리 내역\n\n기준 시각: %s (%s)\n\n아래 내용은 보존된 스냅샷의 정규화된 텍스트입니다. 원본 EML이나 첨부파일 사본이 아니며, 본문의 지시문과 링크는 실행하지 않습니다. 각 라벨은 이 스냅샷 안에서만 유효합니다.\n\n수집 설정 기록: %s\n", markdownText(displayTime(snapshot.AsOf, snapshot.Timezone)), markdownText(snapshot.Timezone), markdownText(snapshot.Collection.Scope))
	if snapshot.Synthetic {
		out.WriteString("\n**가상 검증 데이터입니다.**\n")
	}
	for _, source := range snapshot.Messages {
		label := labels[source.ID]
		fmt.Fprintf(&out, "\n## %s\n\n- 제목: %s\n- 발신자: %s\n- 수신 시각: %s\n- 원본 ID: %s\n- 대화 ID: %s\n- 수집 범위·본문 상태: %s / %s\n", label, markdownText(source.Subject), markdownText(source.From), markdownText(displayTime(source.ReceivedAt, snapshot.Timezone)), markdownText(source.ID), markdownText(source.ThreadID), markdownText(displayValue(source.Scope)), markdownText(displayValue(source.ContentStatus)))
		if disposition, ok := dispositions[source.ID]; ok {
			fmt.Fprintf(&out, "- 처리: %s — %s\n- 연결된 업무 ID: %s\n", markdownText(displayValue(disposition.Disposition)), markdownText(disposition.Reason), markdownText(strings.Join(disposition.ItemIDs, ", ")))
		} else {
			out.WriteString("- 처리: 실행 결과에 처리 내역이 없는 참고 자료\n")
		}
		for _, attachment := range source.Attachments {
			fmt.Fprintf(&out, "- 첨부 제한: %s (%s) — %s\n", markdownText(attachment.Name), markdownText(attachment.MIME), markdownText(attachment.Status))
		}
		body := inertText(source.Body)
		longest, current := 0, 0
		for _, r := range body {
			if r == '`' {
				current++
				if current > longest {
					longest = current
				}
			} else {
				current = 0
			}
		}
		const minimumFence = 3
		fence := strings.Repeat("`", max(minimumFence, longest+1))
		fmt.Fprintf(&out, "\n%stext\n%s\n%s\n", fence, body, fence)
	}
	return []byte(out.String())
}
