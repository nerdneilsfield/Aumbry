package schema

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	maxSectionDepth     = 3
	maxMermaidBlocks    = 10
	maxChartBlocks      = 10
	maxStagesGroups     = 8
	maxTotalStages      = 40
	maxStagesPerGroup   = 8
	minStepsInteractive = 2
	maxMetricsCount     = 12
	maxEquationBytes    = 4 * 1024
	maxMermaidBytes     = 20 * 1024
	maxCodeBlockBytes   = 32 * 1024
	maxTLDRPoints       = 7
	maxTLDRRunes        = 300
	maxNewsSummaryRunes = 600
	maxNewsDetailBytes  = 8 * 1024
	maxQuizOptions      = 8
	minQuizOptions      = 2
	maxNextStepsOptions = 5
	minFigureGroupItems = 2
	maxFigureGroupItems = 6
	maxChartSeries      = 20
	maxChartPoints      = 2000
)

var (
	anchorIDRe      = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	targetRe        = regexp.MustCompile(`^#[a-z0-9-]+$`)
	itemIDRe        = regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)
	doiRe           = regexp.MustCompile(`^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$`)
	highlightPartRe = regexp.MustCompile(`^(\d+)(?:-(\d+))?$`)
)

var (
	claimLevels      = []string{"claim", "evidence", "inference", "unverified"}
	cellTags         = []string{"claim", "evidence", "inference", "na", "unreported", "approx"}
	readDepths       = []string{"full", "abstract", "snippet"}
	metricTrends     = []string{"up", "down", "flat"}
	checkStatuses    = []string{"done", "todo", "blocked", "risk"}
	newsImportances  = []string{"high", "normal"}
	tldrPlacements   = []string{"top", "bottom"}
	chartTypes       = []string{"bar", "line", "pie", "scatter"}
	figureGroupCols  = []string{"2", "3"}
	boolStrings      = []string{"true", "false"}
	searchLogAllowed = []string{"criteria", "query", "excluded", "gap", "stop"}
	quizAllowed      = []string{"question", "option", "explain"}
)

var anchorPrefixes = map[string]string{
	"section":  "s-",
	"stages":   "stages-",
	"stage":    "stage-",
	"quiz":     "quiz-",
	"figure":   "fig-",
	"equation": "eq-",
	"ref":      "ref-",
}

func isValidYMD(raw string) bool {
	s := strings.TrimSpace(raw)
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func isValidTimelineDate(raw string) bool {
	s := strings.TrimSpace(raw)
	switch len(s) {
	case 4:
		_, err := time.Parse("2006", s)
		return err == nil
	case 7:
		_, err := time.Parse("2006-01", s)
		return err == nil
	case 10:
		return isValidYMD(s)
	default:
		return false
	}
}

func isValidNewsPublished(raw string) bool {
	s := strings.TrimSpace(raw)
	if isValidYMD(s) {
		return true
	}
	if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return true
	}
	return false
}

func contains(list []string, v string) bool {
	return slices.Contains(list, v)
}
