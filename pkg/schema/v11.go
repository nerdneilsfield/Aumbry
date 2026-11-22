package schema

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lestrrat-go/helium"
)

type pendingTarget struct {
	line  int
	field string
	value string
}

type v11State struct {
	issues []ValidationError

	mermaidCount int
	chartCount   int
	stagesCount  int
	totalStages  int

	seenIDs   map[string]bool
	anchors   map[string]bool
	equations map[string]bool

	pendingTargets []pendingTarget
	pendingEqRefs  []pendingTarget
}

func (d *Document) checkV11() []ValidationError {
	if d.Version != "1.1" {
		return nil
	}
	body := firstChildNamed(d.tree.DocumentElement(), "body")
	if body == nil {
		return nil
	}
	st := &v11State{
		seenIDs:   make(map[string]bool),
		anchors:   make(map[string]bool),
		equations: make(map[string]bool),
	}
	st.walk(body, 0, false)
	st.checkPendingRefs()
	return st.issues
}

func (s *v11State) addIssue(line int, code, field, msg, hint string) {
	s.issues = append(s.issues, ValidationError{
		Line:    line,
		Code:    code,
		Field:   field,
		Message: msg,
		Hint:    hint,
	})
}

func (s *v11State) registerID(line int, elem, id string) {
	if !anchorIDRe.MatchString(id) {
		s.addIssue(line, "INVALID_ID", elem+".id", fmt.Sprintf("id %q must match ^[a-z0-9-]{1,32}$", id), "use 1-32 lowercase letters, digits or hyphens")
		return
	}
	if elem == "equation" {
		s.equations[id] = true
	}
	if s.seenIDs[id] {
		s.addIssue(line, "DUPLICATE_ID", elem+".id", fmt.Sprintf("duplicate document id %q", id), "use a unique id across the entire document")
		return
	}
	s.seenIDs[id] = true
	if prefix, ok := anchorPrefixes[elem]; ok {
		s.anchors["#"+prefix+id] = true
	}
}

func (s *v11State) walk(el *helium.Element, sectionDepth int, inStage bool) {
	tldrCount := 0
	for _, child := range childElements(el) {
		name := child.Name()
		if name == "tldr" {
			if el.Name() == "body" || el.Name() == "section" || el.Name() == "stage" {
				tldrCount++
				if tldrCount > 1 {
					s.addIssue(child.Line(), "INVALID_TLDR", "tldr", fmt.Sprintf("<%s> may contain at most one <tldr>", el.Name()), "keep at most one <tldr> per container")
				}
				s.checkTLDR(child)
			} else if el.Name() == "collapse" || el.Name() == "tab" || el.Name() == "step" {
				s.addIssue(child.Line(), "INVALID_TLDR", "tldr", "<tldr> may only be a direct child of <body>, <section> or <stage>", "move <tldr> directly under <body>, <section> or <stage>")
				s.checkTLDR(child)
			}
			continue
		}

		nextDepth := sectionDepth
		nextInStage := inStage

		switch name {
		case "section":
			if sectionDepth >= maxSectionDepth {
				s.addIssue(child.Line(), "SECTION_TOO_DEEP", "section", fmt.Sprintf("<section> nesting exceeds %d levels", maxSectionDepth), "flatten nested sections to at most 3 levels")
			}
			nextDepth++
			if id, has := child.GetAttribute("id"); has {
				s.registerID(child.Line(), "section", id)
			}
		case "stages":
			s.stagesCount++
			if s.stagesCount > maxStagesGroups {
				if s.stagesCount == maxStagesGroups+1 {
					s.addIssue(child.Line(), "TOO_MANY_BLOCKS", "stages", fmt.Sprintf("at most %d <stages> groups allowed per document", maxStagesGroups), "")
				}
			}
			if inStage {
				s.addIssue(child.Line(), "INVALID_STAGES", "stages", "<stages> cannot be nested inside a <stage>", "place <stages> outside any <stage>")
			}
			s.checkStages(child)
		case "stage":
			nextInStage = true
		case "figure":
			if id, has := child.GetAttribute("id"); has {
				s.registerID(child.Line(), "figure", id)
			}
		case "claim":
			level, _ := child.GetAttribute("level")
			if !contains(claimLevels, level) {
				s.addIssue(child.Line(), "INVALID_ATTR", "claim.level", "claim level must be claim, evidence, inference or unverified", "")
			}
		case "cell":
			if tag, has := child.GetAttribute("tag"); has && !contains(cellTags, tag) {
				s.addIssue(child.Line(), "INVALID_ATTR", "cell.tag", "cell tag must be claim, evidence, inference, na, unreported or approx", "")
			}
		case "paper-card":
			s.checkDOI(child, "paper-card")
			if rd, has := child.GetAttribute("read-depth"); has && !contains(readDepths, rd) {
				s.addIssue(child.Line(), "INVALID_ATTR", "paper-card.read-depth", "read-depth must be full, abstract or snippet", "")
			}
		case "equation":
			if id, has := child.GetAttribute("id"); has {
				s.registerID(child.Line(), "equation", id)
			}
			s.checkEquation(child)
		case "mermaid":
			s.mermaidCount++
			if s.mermaidCount == maxMermaidBlocks+1 {
				s.addIssue(child.Line(), "TOO_MANY_BLOCKS", "mermaid", fmt.Sprintf("at most %d <mermaid> blocks allowed per document", maxMermaidBlocks), "")
			}
			s.checkMermaid(child)
		case "chart":
			s.chartCount++
			if s.chartCount == maxChartBlocks+1 {
				s.addIssue(child.Line(), "TOO_MANY_BLOCKS", "chart", fmt.Sprintf("at most %d <chart> blocks allowed per document", maxChartBlocks), "")
			}
			typ, _ := child.GetAttribute("type")
			s.issues = append(s.issues, validateChart(child.Line(), typ, string(child.Content()))...)
		case "metrics":
			s.checkMetrics(child)
		case "steps":
			s.checkSteps(child)
		case "tabs":
			s.checkTabs(child)
		case "timeline":
			s.checkTimeline(child)
		case "glossary":
			s.checkGlossary(child)
		case "references":
			s.checkReferences(child)
		case "figure-group":
			s.checkFigureGroup(child)
		case "code-block":
			s.checkCodeBlock(child)
		case "checklist":
			s.checkChecklist(child)
		case "search-log":
			s.checkSearchLog(child)
		case "quiz":
			s.checkQuiz(child)
		case "next-steps":
			s.checkNextSteps(child)
		case "news-item":
			s.checkNewsItem(child)
		}

		// Recurse into containers that hold blocks or other nested containers.
		switch name {
		case "section", "collapse", "stages", "stage", "steps", "step", "tabs", "tab", "figure-group", "compare-table", "row":
			s.walk(child, nextDepth, nextInStage)
		}
	}
}

func (s *v11State) checkPendingRefs() {
	for _, pt := range s.pendingTargets {
		if !targetRe.MatchString(pt.value) || !s.anchors[pt.value] {
			s.addIssue(pt.line, "INVALID_TARGET", pt.field, fmt.Sprintf("target %q does not match an existing anchor id in this document", pt.value), "use a prefix anchor like #s-..., #stages-..., #stage-..., #quiz-..., #fig-..., #eq-... or #ref-...")
		}
	}
	for _, pe := range s.pendingEqRefs {
		if !s.equations[pe.value] {
			s.addIssue(pe.line, "INVALID_TARGET", pe.field, fmt.Sprintf("eq target %q does not match any <equation id>", pe.value), "reference an existing <equation id=\"...\"> raw id")
		}
	}
}

func (s *v11State) checkDOI(el *helium.Element, owner string) {
	doi, has := el.GetAttribute("doi")
	if !has {
		return
	}
	if !doiRe.MatchString(doi) {
		s.addIssue(el.Line(), "INVALID_DOI", owner+".doi", fmt.Sprintf("invalid DOI %q", doi), "pass bare DOI like 10.48550/arXiv.2611.01234 without https://doi.org/")
	}
}

func (s *v11State) checkEquation(el *helium.Element) {
	trimmed := strings.TrimSpace(string(el.Content()))
	if len(trimmed) == 0 {
		s.addIssue(el.Line(), "INVALID_EQUATION", "equation", "<equation> must not be empty", "")
		return
	}
	if len(trimmed) > maxEquationBytes {
		s.addIssue(el.Line(), "INVALID_EQUATION", "equation", fmt.Sprintf("<equation> exceeds %d bytes (%d bytes)", maxEquationBytes, len(trimmed)), "")
	}
}

func (s *v11State) checkMermaid(el *helium.Element) {
	trimmed := strings.TrimSpace(string(el.Content()))
	if len(trimmed) == 0 {
		s.addIssue(el.Line(), "INVALID_MERMAID", "mermaid", "<mermaid> must not be empty", "")
		return
	}
	if len(trimmed) > maxMermaidBytes {
		s.addIssue(el.Line(), "INVALID_MERMAID", "mermaid", fmt.Sprintf("<mermaid> exceeds %d bytes (%d bytes)", maxMermaidBytes, len(trimmed)), "")
	}
}

func (s *v11State) checkCodeBlock(el *helium.Element) {
	trimmed := strings.TrimSpace(string(el.Content()))
	if len(trimmed) == 0 {
		s.addIssue(el.Line(), "INVALID_CODE_BLOCK", "code-block", "<code-block> must not be empty", "wrap source code in <![CDATA[ ... ]]>")
	} else if len(trimmed) > maxCodeBlockBytes {
		s.addIssue(el.Line(), "INVALID_CODE_BLOCK", "code-block", fmt.Sprintf("<code-block> exceeds %d bytes (%d bytes)", maxCodeBlockBytes, len(trimmed)), "")
	}
	if diff, has := el.GetAttribute("diff"); has && !contains(boolStrings, diff) {
		s.addIssue(el.Line(), "INVALID_CODE_BLOCK", "code-block.diff", `diff attribute must be "true" or "false"`, "")
	}
	if hl, has := el.GetAttribute("highlight"); has {
		if !isValidHighlightSpec(hl) {
			s.addIssue(el.Line(), "INVALID_CODE_BLOCK", "code-block.highlight", fmt.Sprintf("invalid highlight range %q", hl), `use line numbers or ranges like "3,5-7"`)
		}
	}
	if eq, has := el.GetAttribute("eq"); has {
		if strings.TrimSpace(eq) == "" {
			s.addIssue(el.Line(), "INVALID_TARGET", "code-block.eq", "eq attribute must not be empty", "")
		} else {
			s.pendingEqRefs = append(s.pendingEqRefs, pendingTarget{line: el.Line(), field: "code-block.eq", value: eq})
		}
	}
}

func isValidHighlightSpec(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		m := highlightPartRe.FindStringSubmatch(p)
		if m == nil {
			return false
		}
		start, err := strconv.Atoi(m[1])
		if err != nil || start < 1 {
			return false
		}
		if m[2] != "" {
			end, err := strconv.Atoi(m[2])
			if err != nil || end < start {
				return false
			}
		}
	}
	return true
}

func (s *v11State) checkMetrics(el *helium.Element) {
	s.rejectDirectText(el, "metrics")
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "metric" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "metrics", fmt.Sprintf("<%s> is not allowed inside <metrics>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "metric", "<metric> cannot contain child elements", "")
		}
		label, _ := child.GetAttribute("label")
		if strings.TrimSpace(label) == "" {
			s.addIssue(child.Line(), "INVALID_METRIC", "metric.label", `<metric> requires a non-empty "label" attribute`, "")
		}
		val, _ := child.GetAttribute("value")
		if strings.TrimSpace(val) == "" {
			s.addIssue(child.Line(), "INVALID_METRIC", "metric.value", `<metric> requires a non-empty "value" attribute`, "")
		}
		if trend, has := child.GetAttribute("trend"); has && !contains(metricTrends, trend) {
			s.addIssue(child.Line(), "INVALID_METRIC", "metric.trend", "metric trend must be up, down or flat", "")
		}
	}
	if count == 0 || count > maxMetricsCount {
		s.addIssue(el.Line(), "INVALID_METRIC", "metrics", fmt.Sprintf("<metrics> must contain 1-%d <metric> elements, got %d", maxMetricsCount, count), "")
	}
}

func (s *v11State) checkSteps(el *helium.Element) {
	s.rejectDirectText(el, "steps")
	if inter, has := el.GetAttribute("interactive"); has && !contains(boolStrings, inter) {
		s.addIssue(el.Line(), "INVALID_ATTR", "steps.interactive", `interactive must be "true" or "false"`, "")
	}
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "step" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "steps", fmt.Sprintf("<%s> is not allowed inside <steps>", child.Name()), "")
			continue
		}
		count++
	}
	inter, _ := el.GetAttribute("interactive")
	if inter == "true" {
		if count < minStepsInteractive {
			s.addIssue(el.Line(), "INVALID_ATTR", "steps.interactive", fmt.Sprintf(`<steps interactive="true"> requires at least %d <step> elements, got %d`, minStepsInteractive, count), "")
		}
	} else if count == 0 {
		s.addIssue(el.Line(), "INVALID_ATTR", "steps", "<steps> requires at least 1 <step> element", "")
	}
}

func (s *v11State) checkTabs(el *helium.Element) {
	s.rejectDirectText(el, "tabs")
	seenTitles := make(map[string]bool)
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "tab" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "tabs", fmt.Sprintf("<%s> is not allowed inside <tabs>", child.Name()), "")
			continue
		}
		count++
		title, _ := child.GetAttribute("title")
		trimmed := strings.TrimSpace(title)
		if trimmed != "" {
			if seenTitles[trimmed] {
				s.addIssue(child.Line(), "DUPLICATE_TAB", "tab.title", fmt.Sprintf("duplicate tab title %q inside <tabs>", trimmed), "give each <tab> a unique title")
			}
			seenTitles[trimmed] = true
		}
	}
	if count < 2 {
		s.addIssue(el.Line(), "INVALID_ATTR", "tabs", fmt.Sprintf("<tabs> requires at least 2 <tab> elements, got %d", count), "")
	}
}

func (s *v11State) checkTimeline(el *helium.Element) {
	s.rejectDirectText(el, "timeline")
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "event" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "timeline", fmt.Sprintf("<%s> is not allowed inside <timeline>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "event", "<event> cannot contain child elements", "")
		}
		title, _ := child.GetAttribute("title")
		if strings.TrimSpace(title) == "" {
			s.addIssue(child.Line(), "MISSING_ATTR", "event.title", `<event> requires the "title" attribute`, "")
		}
		dateVal, _ := child.GetAttribute("date")
		if !isValidTimelineDate(dateVal) {
			s.addIssue(child.Line(), "INVALID_DATE", "event.date", fmt.Sprintf("event date %q must use YYYY, YYYY-MM or YYYY-MM-DD format", dateVal), "")
		}
	}
	if count == 0 {
		s.addIssue(el.Line(), "INVALID_ATTR", "timeline", "<timeline> requires at least 1 <event> element", "")
	}
}

func (s *v11State) checkGlossary(el *helium.Element) {
	s.rejectDirectText(el, "glossary")
	seenNames := make(map[string]bool)
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "term" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "glossary", fmt.Sprintf("<%s> is not allowed inside <glossary>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "term", "<term> cannot contain child elements", "")
		}
		name, _ := child.GetAttribute("name")
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			s.addIssue(child.Line(), "MISSING_ATTR", "term.name", `<term> requires the "name" attribute`, "")
			continue
		}
		if seenNames[trimmed] {
			s.addIssue(child.Line(), "INVALID_ATTR", "term.name", fmt.Sprintf("duplicate glossary term %q", trimmed), "")
		}
		seenNames[trimmed] = true
	}
	if count == 0 {
		s.addIssue(el.Line(), "INVALID_ATTR", "glossary", "<glossary> requires at least 1 <term> element", "")
	}
}

func (s *v11State) checkReferences(el *helium.Element) {
	s.rejectDirectText(el, "references")
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "ref" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "references", fmt.Sprintf("<%s> is not allowed inside <references>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "ref", "<ref> cannot contain child elements", "")
		}
		id, hasID := child.GetAttribute("id")
		if !hasID || strings.TrimSpace(id) == "" {
			s.addIssue(child.Line(), "INVALID_ID", "ref.id", `<ref> requires the "id" attribute`, "")
		} else {
			s.registerID(child.Line(), "ref", id)
		}
		if title, _ := child.GetAttribute("title"); strings.TrimSpace(title) == "" {
			s.addIssue(child.Line(), "MISSING_ATTR", "ref.title", `<ref> requires the "title" attribute`, "")
		}
		if rd, has := child.GetAttribute("read-depth"); has && !contains(readDepths, rd) {
			s.addIssue(child.Line(), "INVALID_ATTR", "ref.read-depth", "read-depth must be full, abstract or snippet", "")
		}
		s.checkDOI(child, "ref")
	}
	if count == 0 {
		s.addIssue(el.Line(), "INVALID_ATTR", "references", "<references> requires at least 1 <ref> element", "")
	}
}

func (s *v11State) checkFigureGroup(el *helium.Element) {
	s.rejectDirectText(el, "figure-group")
	cols, has := el.GetAttribute("cols")
	if !has {
		s.addIssue(el.Line(), "MISSING_ATTR", "figure-group.cols", `<figure-group> requires the "cols" attribute`, "")
	} else if !contains(figureGroupCols, cols) {
		s.addIssue(el.Line(), "INVALID_ATTR", "figure-group.cols", `figure-group cols must be "2" or "3"`, "")
	}
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "figure" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "figure-group", fmt.Sprintf("<%s> is not allowed inside <figure-group>", child.Name()), "")
			continue
		}
		count++
	}
	if count < minFigureGroupItems || count > maxFigureGroupItems {
		s.addIssue(el.Line(), "INVALID_ATTR", "figure-group", fmt.Sprintf("<figure-group> requires %d-%d <figure> elements, got %d", minFigureGroupItems, maxFigureGroupItems, count), "")
	}
}

func (s *v11State) checkChecklist(el *helium.Element) {
	s.rejectDirectText(el, "checklist")
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "check" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "checklist", fmt.Sprintf("<%s> is not allowed inside <checklist>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "check", "<check> cannot contain child elements", "")
		}
		status, _ := child.GetAttribute("status")
		if !contains(checkStatuses, status) {
			s.addIssue(child.Line(), "INVALID_CHECK", "check.status", "check status must be done, todo, blocked or risk", `use status="done", "todo", "blocked" or "risk"`)
		}
	}
	if count == 0 {
		s.addIssue(el.Line(), "INVALID_CHECK", "checklist", "<checklist> requires at least 1 <check> element", "")
	}
}

func (s *v11State) checkSearchLog(el *helium.Element) {
	s.rejectDirectText(el, "search-log")
	if d, has := el.GetAttribute("date"); has && !isValidYMD(d) {
		s.addIssue(el.Line(), "INVALID_DATE", "search-log.date", fmt.Sprintf("search-log date %q must use YYYY-MM-DD format", d), "")
	}
	criteriaCount := 0
	queryCount := 0
	stopCount := 0
	for _, child := range childElements(el) {
		name := child.Name()
		if !contains(searchLogAllowed, name) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "search-log", fmt.Sprintf("<%s> is not allowed inside <search-log>", name), "")
			continue
		}
		switch name {
		case "criteria":
			criteriaCount++
			if criteriaCount > 1 {
				s.addIssue(child.Line(), "INVALID_SEARCH_LOG", "search-log.criteria", "<search-log> may contain at most 1 <criteria>", "")
			}
			s.rejectDirectText(child, "search-log.criteria")
			for _, sub := range childElements(child) {
				if sub.Name() != "include" && sub.Name() != "exclude" {
					s.addIssue(sub.Line(), "UNKNOWN_TAG", "search-log.criteria", fmt.Sprintf("<%s> is not allowed inside <criteria>", sub.Name()), "")
					continue
				}
				if hasChildElements(sub) {
					s.addIssue(sub.Line(), "UNKNOWN_TAG", sub.Name(), fmt.Sprintf("<%s> cannot contain child elements", sub.Name()), "")
				}
			}
		case "query":
			queryCount++
			if hasChildElements(child) {
				s.addIssue(child.Line(), "UNKNOWN_TAG", "query", "<query> cannot contain child elements", "")
			}
			if d, has := child.GetAttribute("date"); has && !isValidYMD(d) {
				s.addIssue(child.Line(), "INVALID_DATE", "query.date", fmt.Sprintf("query date %q must use YYYY-MM-DD format", d), "")
			}
			for _, attr := range []string{"hits", "kept"} {
				if raw, has := child.GetAttribute(attr); has {
					n, err := strconv.Atoi(raw)
					if err != nil || n < 0 {
						s.addIssue(child.Line(), "INVALID_SEARCH_LOG", "query."+attr, fmt.Sprintf("query %s must be a non-negative integer, got %q", attr, raw), "")
					}
				}
			}
		case "stop":
			stopCount++
			if stopCount > 1 {
				s.addIssue(child.Line(), "INVALID_SEARCH_LOG", "search-log.stop", "<search-log> may contain at most 1 <stop>", "")
			}
			if hasChildElements(child) {
				s.addIssue(child.Line(), "UNKNOWN_TAG", "stop", "<stop> cannot contain child elements", "")
			}
		case "excluded", "gap":
			if hasChildElements(child) {
				s.addIssue(child.Line(), "UNKNOWN_TAG", name, fmt.Sprintf("<%s> cannot contain child elements", name), "")
			}
		}
	}
	if queryCount == 0 {
		s.addIssue(el.Line(), "INVALID_SEARCH_LOG", "search-log.query", "<search-log> requires at least 1 <query>", "add at least one <query> inside <search-log>")
	}
}

func (s *v11State) checkStages(el *helium.Element) {
	s.rejectDirectText(el, "stages")
	id, hasID := el.GetAttribute("id")
	if !hasID || strings.TrimSpace(id) == "" {
		s.addIssue(el.Line(), "INVALID_STAGES", "stages.id", `<stages> requires the "id" attribute`, `add id="..." to <stages>`)
	} else {
		s.registerID(el.Line(), "stages", id)
	}
	if gated, has := el.GetAttribute("gated"); has && !contains(boolStrings, gated) {
		s.addIssue(el.Line(), "INVALID_STAGES", "stages.gated", `stages gated must be "true" or "false"`, "")
	}
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "stage" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "stages", fmt.Sprintf("<%s> is not allowed inside <stages>", child.Name()), "")
			continue
		}
		count++
		s.totalStages++
		if s.totalStages == maxTotalStages+1 {
			s.addIssue(child.Line(), "TOO_MANY_BLOCKS", "stage", fmt.Sprintf("at most %d <stage> elements allowed across document", maxTotalStages), "")
		}
		sid, hasSID := child.GetAttribute("id")
		if !hasSID || strings.TrimSpace(sid) == "" {
			s.addIssue(child.Line(), "INVALID_STAGES", "stage.id", `<stage> requires the "id" attribute`, `add id="..." to <stage>`)
		} else {
			s.registerID(child.Line(), "stage", sid)
		}
		if title, _ := child.GetAttribute("title"); strings.TrimSpace(title) == "" {
			s.addIssue(child.Line(), "INVALID_STAGES", "stage.title", `<stage> requires the "title" attribute`, `add title="..." to <stage>`)
		}
	}
	if count == 0 || count > maxStagesPerGroup {
		s.addIssue(el.Line(), "INVALID_STAGES", "stages", fmt.Sprintf("<stages> must contain 1-%d <stage> elements, got %d", maxStagesPerGroup, count), "")
	}
}

func (s *v11State) checkQuiz(el *helium.Element) {
	s.rejectDirectText(el, "quiz")
	id, hasID := el.GetAttribute("id")
	if !hasID || strings.TrimSpace(id) == "" {
		s.addIssue(el.Line(), "INVALID_QUIZ", "quiz.id", `<quiz> requires the "id" attribute`, `add id="..." to <quiz>`)
	} else {
		s.registerID(el.Line(), "quiz", id)
	}

	questions := 0
	options := 0
	correctCount := 0
	explains := 0
	for _, child := range childElements(el) {
		name := child.Name()
		if !contains(quizAllowed, name) {
			s.addIssue(child.Line(), "INVALID_QUIZ", "quiz", fmt.Sprintf("<%s> is not allowed inside <quiz>", name), "")
			continue
		}
		hasChildren := hasChildElements(child)
		if hasChildren {
			s.addIssue(child.Line(), "INVALID_QUIZ", "quiz."+name, fmt.Sprintf("<%s> cannot contain child elements", name), "escape tags inside CDATA or entity text")
		}
		switch name {
		case "question":
			questions++
			if !hasChildren && strings.TrimSpace(string(child.Content())) == "" {
				s.addIssue(child.Line(), "INVALID_QUIZ", "quiz.question", "<question> must not be empty", "")
			}
		case "option":
			options++
			if !hasChildren && strings.TrimSpace(string(child.Content())) == "" {
				s.addIssue(child.Line(), "INVALID_QUIZ", "quiz.option", "<option> must not be empty", "")
			}
			if corr, has := child.GetAttribute("correct"); has {
				if !contains(boolStrings, corr) {
					s.addIssue(child.Line(), "INVALID_QUIZ", "option.correct", `option correct must be "true" or "false"`, "")
				} else if corr == "true" {
					correctCount++
				}
			}
		case "explain":
			explains++
		}
	}
	if questions != 1 {
		s.addIssue(el.Line(), "INVALID_QUIZ", "quiz.question", fmt.Sprintf("<quiz> requires exactly 1 <question>, got %d", questions), "")
	}
	if options < minQuizOptions || options > maxQuizOptions {
		s.addIssue(el.Line(), "INVALID_QUIZ", "quiz.option", fmt.Sprintf("<quiz> requires %d-%d <option> elements, got %d", minQuizOptions, maxQuizOptions, options), "")
	}
	if options >= minQuizOptions && correctCount == 0 {
		s.addIssue(el.Line(), "INVALID_QUIZ", "quiz.option", `<quiz> requires at least 1 <option correct="true">`, "")
	}
	if explains > 1 {
		s.addIssue(el.Line(), "INVALID_QUIZ", "quiz.explain", "<quiz> may contain at most 1 <explain>", "")
	}
}

func (s *v11State) checkNextSteps(el *helium.Element) {
	s.rejectDirectText(el, "next-steps")
	count := 0
	for _, child := range childElements(el) {
		if child.Name() != "option" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "next-steps", fmt.Sprintf("<%s> is not allowed inside <next-steps>", child.Name()), "")
			continue
		}
		count++
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "next-steps.option", "<option> inside <next-steps> cannot contain child elements", "")
		}
		target, _ := child.GetAttribute("target")
		s.pendingTargets = append(s.pendingTargets, pendingTarget{
			line:  child.Line(),
			field: "next-steps.option.target",
			value: target,
		})
	}
	if count == 0 || count > maxNextStepsOptions {
		s.addIssue(el.Line(), "INVALID_ATTR", "next-steps", fmt.Sprintf("<next-steps> requires 1-%d <option> elements, got %d", maxNextStepsOptions, count), "")
	}
}

func (s *v11State) checkTLDR(el *helium.Element) {
	s.rejectDirectText(el, "tldr")
	if placement, has := el.GetAttribute("placement"); has && !contains(tldrPlacements, placement) {
		s.addIssue(el.Line(), "INVALID_TLDR", "tldr.placement", `tldr placement must be "top" or "bottom"`, "")
	}
	points := 0
	verdicts := 0
	for _, child := range childElements(el) {
		switch child.Name() {
		case "point":
			points++
			if hasChildElements(child) {
				s.addIssue(child.Line(), "INVALID_TLDR", "tldr.point", "<point> cannot contain child elements", "")
			}
			trimmed := strings.TrimSpace(string(child.Content()))
			if trimmed == "" || utf8.RuneCountInString(trimmed) > maxTLDRRunes {
				s.addIssue(child.Line(), "INVALID_TLDR", "tldr.point", fmt.Sprintf("<point> must be 1-%d Unicode runes", maxTLDRRunes), "")
			}
		case "verdict":
			verdicts++
			if hasChildElements(child) {
				s.addIssue(child.Line(), "INVALID_TLDR", "tldr.verdict", "<verdict> cannot contain child elements", "")
			}
			trimmed := strings.TrimSpace(string(child.Content()))
			if trimmed == "" || utf8.RuneCountInString(trimmed) > maxTLDRRunes {
				s.addIssue(child.Line(), "INVALID_TLDR", "tldr.verdict", fmt.Sprintf("<verdict> must be 1-%d Unicode runes", maxTLDRRunes), "")
			}
		default:
			s.addIssue(child.Line(), "INVALID_TLDR", "tldr", fmt.Sprintf("<%s> is not allowed inside <tldr>", child.Name()), "")
		}
	}
	if points == 0 || points > maxTLDRPoints {
		s.addIssue(el.Line(), "INVALID_TLDR", "tldr.point", fmt.Sprintf("<tldr> must contain 1-%d <point> elements, got %d", maxTLDRPoints, points), "")
	}
	if verdicts > 1 {
		s.addIssue(el.Line(), "INVALID_TLDR", "tldr.verdict", "<tldr> may contain at most 1 <verdict>", "")
	}
}

func (s *v11State) checkNewsItem(el *helium.Element) {
	if title, _ := el.GetAttribute("title"); strings.TrimSpace(title) == "" {
		s.addIssue(el.Line(), "INVALID_NEWS", "news-item.title", `<news-item> requires a non-empty "title" attribute`, `add title="..." to <news-item>`)
	}
	if imp, has := el.GetAttribute("importance"); has && !contains(newsImportances, imp) {
		s.addIssue(el.Line(), "INVALID_NEWS", "news-item.importance", `importance must be "high" or "normal"`, "")
	}
	if pub, has := el.GetAttribute("published"); has && !isValidNewsPublished(pub) {
		s.addIssue(el.Line(), "INVALID_DATE", "news-item.published", fmt.Sprintf("published %q must be YYYY-MM-DD or RFC 3339", pub), "")
	}
	if did, has := el.GetAttribute("detail-id"); has && !itemIDRe.MatchString(did) {
		s.addIssue(el.Line(), "INVALID_ID", "news-item.detail-id", fmt.Sprintf("detail-id %q must be a 12-character item id", did), "")
	}

	var summary strings.Builder
	for cur := el.FirstChild(); cur != nil; cur = cur.NextSibling() {
		switch cur.Type() {
		case helium.TextNode, helium.CDATASectionNode:
			summary.Write(cur.Content())
		}
	}
	if runes := utf8.RuneCountInString(strings.TrimSpace(summary.String())); runes > maxNewsSummaryRunes {
		s.addIssue(el.Line(), "INVALID_NEWS", "news-item", fmt.Sprintf("<news-item> summary exceeds %d Unicode runes (%d)", maxNewsSummaryRunes, runes), "")
	}

	details := 0
	for _, child := range childElements(el) {
		if child.Name() != "detail" {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "news-item", fmt.Sprintf("<%s> is not allowed inside <news-item>", child.Name()), "")
			continue
		}
		details++
		if details > 1 {
			s.addIssue(child.Line(), "INVALID_NEWS", "news-item.detail", "<news-item> may contain at most 1 <detail>", "")
		}
		if hasChildElements(child) {
			s.addIssue(child.Line(), "UNKNOWN_TAG", "news-item.detail", "<detail> cannot contain child elements", "")
		}
		if n := len(strings.TrimSpace(string(child.Content()))); n > maxNewsDetailBytes {
			s.addIssue(child.Line(), "INVALID_NEWS", "news-item.detail", fmt.Sprintf("<detail> exceeds %d bytes (%d bytes)", maxNewsDetailBytes, n), "")
		}
	}
}

func (s *v11State) rejectDirectText(el *helium.Element, field string) {
	for cur := el.FirstChild(); cur != nil; cur = cur.NextSibling() {
		switch cur.Type() {
		case helium.TextNode, helium.CDATASectionNode:
			if strings.TrimSpace(string(cur.Content())) != "" {
				s.addIssue(el.Line(), "UNEXPECTED_TEXT", field, fmt.Sprintf("<%s> cannot contain text directly", el.Name()), "")
				return
			}
		}
	}
}

func firstChildNamed(parent helium.Node, name string) *helium.Element {
	if parent == nil {
		return nil
	}
	for cur := parent.FirstChild(); cur != nil; cur = cur.NextSibling() {
		if cur.Type() == helium.ElementNode && cur.Name() == name {
			if el, ok := cur.(*helium.Element); ok {
				return el
			}
		}
	}
	return nil
}

func childElements(parent helium.Node) []*helium.Element {
	var out []*helium.Element
	for cur := parent.FirstChild(); cur != nil; cur = cur.NextSibling() {
		if cur.Type() == helium.ElementNode {
			if el, ok := cur.(*helium.Element); ok {
				out = append(out, el)
			}
		}
	}
	return out
}

func hasChildElements(el helium.Node) bool {
	for cur := el.FirstChild(); cur != nil; cur = cur.NextSibling() {
		if cur.Type() == helium.ElementNode {
			return true
		}
	}
	return false
}
