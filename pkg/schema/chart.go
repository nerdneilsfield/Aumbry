package schema

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed chart_schema.json
var chartSchemaSource []byte

var chartSchemas = sync.OnceValue(func() map[string]*jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(chartSchemaSource)))
	if err != nil {
		panic("schema: chart_schema.json: " + err.Error())
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("chart.json", doc); err != nil {
		panic("schema: chart_schema.json: " + err.Error())
	}
	out := make(map[string]*jsonschema.Schema, len(chartTypes))
	for _, typ := range chartTypes {
		sch, err := compiler.Compile("chart.json#/$defs/" + typ)
		if err != nil {
			panic("schema: compile chart def " + typ + ": " + err.Error())
		}
		out[typ] = sch
	}
	return out
})

func validateChart(line int, chartType, raw string) []ValidationError {
	if !contains(chartTypes, chartType) {
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart.type",
			Message: fmt.Sprintf("chart type must be bar, line, pie or scatter, got %q", chartType),
			Hint:    `set type="bar", "line", "pie" or "scatter"`,
		}}
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/",
			Message: "at /: chart body must be a JSON object",
			Hint:    `wrap valid JSON data in <![CDATA[ ... ]]>`,
		}}
	}

	val, err := jsonschema.UnmarshalJSON(strings.NewReader(trimmed))
	if err != nil {
		path := jsonSyntaxPath(trimmed)
		detail := err.Error()
		if tok := nonFiniteTokenNearSyntaxErr(trimmed, err); tok != "" {
			detail = fmt.Sprintf("non-finite number %s is not valid JSON", tok)
		}
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   chartField(path),
			Message: fmt.Sprintf("at %s: %s", path, detail),
			Hint:    "use valid finite JSON numbers inside <chart>",
		}}
	}

	sch := chartSchemas()[chartType]
	if err := sch.Validate(val); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			return flattenChartSchemaErrors(line, verr)
		}
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/",
			Message: fmt.Sprintf("at /: %v", err),
		}}
	}

	return validateChartSemantics(line, chartType, val)
}

func chartField(path string) string {
	if path == "" || path == "/" {
		return "chart/"
	}
	return "chart" + path
}

func flattenChartSchemaErrors(line int, verr *jsonschema.ValidationError) []ValidationError {
	out := verr.BasicOutput()
	var units []jsonschema.OutputUnit
	collectLeafUnits(out, &units)
	if len(units) == 0 {
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/",
			Message: "at /: " + verr.Error(),
		}}
	}
	var issues []ValidationError
	for _, u := range units {
		loc := u.InstanceLocation
		if loc == "" {
			loc = "/"
		}
		msg := "invalid chart value"
		if u.Error != nil {
			msg = u.Error.String()
		}
		issues = append(issues, ValidationError{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   chartField(loc),
			Message: fmt.Sprintf("at %s: %s", loc, msg),
			Hint:    "check <chart> JSON against schema requirements",
		})
	}
	return issues
}

func collectLeafUnits(u *jsonschema.OutputUnit, dst *[]jsonschema.OutputUnit) {
	if len(u.Errors) == 0 {
		if u.Error != nil {
			*dst = append(*dst, *u)
		}
		return
	}
	for i := range u.Errors {
		collectLeafUnits(&u.Errors[i], dst)
	}
}

func validateChartSemantics(line int, chartType string, val any) []ValidationError {
	obj, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	var issues []ValidationError
	var categoriesLen int
	if cats, hasCats := obj["categories"].([]any); hasCats {
		categoriesLen = len(cats)
	}
	series, _ := obj["series"].([]any)
	if len(series) > maxChartSeries {
		issues = append(issues, ValidationError{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/series",
			Message: fmt.Sprintf("at /series: at most %d series allowed, got %d", maxChartSeries, len(series)),
			Hint:    fmt.Sprintf("keep at most %d series", maxChartSeries),
		})
	}
	if chartType == "pie" && len(series) != 1 {
		issues = append(issues, ValidationError{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/series",
			Message: fmt.Sprintf("at /series: pie chart requires exactly 1 series, got %d", len(series)),
			Hint:    "use exactly one series for a pie chart, or switch to type=\"bar\" or \"line\"",
		})
	}

	totalPoints := 0
	for i, rawSeries := range series {
		s, ok := rawSeries.(map[string]any)
		if !ok {
			continue
		}
		base := fmt.Sprintf("/series/%d", i)
		if name, _ := s["name"].(string); strings.TrimSpace(name) == "" {
			issues = append(issues, ValidationError{
				Line:    line,
				Code:    "INVALID_CHART",
				Field:   chartField(base + "/name"),
				Message: fmt.Sprintf("at %s/name: series name must not be empty", base),
				Hint:    "give every series a non-empty \"name\"",
			})
		}

		data, _ := s["data"].([]any)
		totalPoints += len(data)
		if chartType != "scatter" && len(data) != categoriesLen {
			issues = append(issues, ValidationError{
				Line:    line,
				Code:    "INVALID_CHART",
				Field:   chartField(base + "/data"),
				Message: fmt.Sprintf("at %s/data: data length (%d) must equal categories length (%d)", base, len(data), categoriesLen),
				Hint:    "make each series data array the same length as categories",
			})
		}

		for j, pt := range data {
			ptPath := fmt.Sprintf("%s/data/%d", base, j)
			switch v := pt.(type) {
			case []any:
				for k, coord := range v {
					if issue := checkFiniteJSONNumber(line, fmt.Sprintf("%s/%d", ptPath, k), coord); issue != nil {
						issues = append(issues, *issue)
					}
				}
			default:
				if issue := checkFiniteJSONNumber(line, ptPath, v); issue != nil {
					issues = append(issues, *issue)
				}
			}
		}

		if errs, hasErr := s["err"].([]any); hasErr {
			errBase := base + "/err"
			if chartType == "pie" || chartType == "scatter" {
				issues = append(issues, ValidationError{
					Line:    line,
					Code:    "INVALID_CHART",
					Field:   chartField(errBase),
					Message: fmt.Sprintf("at %s: %s chart does not support series[].err", errBase, chartType),
					Hint:    "remove \"err\" from this series, or use type=\"bar\" or \"line\"",
				})
				continue
			}
			if len(errs) != len(data) {
				issues = append(issues, ValidationError{
					Line:    line,
					Code:    "INVALID_CHART",
					Field:   chartField(errBase),
					Message: fmt.Sprintf("at %s: err length (%d) must equal data length (%d)", errBase, len(errs), len(data)),
					Hint:    "give err exactly one value per data point",
				})
			}
			for j, ev := range errs {
				ep := fmt.Sprintf("%s/%d", errBase, j)
				if issue := checkFiniteJSONNumber(line, ep, ev); issue != nil {
					issues = append(issues, *issue)
					continue
				}
				if f, ok := jsonNumberFloat(ev); ok && f < 0 {
					issues = append(issues, ValidationError{
						Line:    line,
						Code:    "INVALID_CHART",
						Field:   chartField(ep),
						Message: fmt.Sprintf("at %s: err must be non-negative", ep),
						Hint:    "use err values >= 0",
					})
				}
			}
		}
	}

	if totalPoints > maxChartPoints {
		issues = append(issues, ValidationError{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   "chart/series",
			Message: fmt.Sprintf("at /series: total data points (%d) exceeds limit (%d)", totalPoints, maxChartPoints),
			Hint:    "reduce the number of data points or split the data into several charts",
		})
	}
	return issues
}

func jsonNumberFloat(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil {
		return f, false
	}
	return f, true
}

func checkFiniteJSONNumber(line int, path string, v any) *ValidationError {
	n, ok := v.(json.Number)
	if !ok {
		return nil
	}
	f, err := n.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return &ValidationError{
			Line:    line,
			Code:    "INVALID_CHART",
			Field:   chartField(path),
			Message: fmt.Sprintf("at %s: number must be finite", path),
			Hint:    "use finite JSON numbers (no NaN or Infinity)",
		}
	}
	return nil
}

type jsonPathFrame struct {
	isObject  bool
	key       string
	expectKey bool
	index     int
}

func jsonSyntaxPath(raw string) string {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var stack []jsonPathFrame

	formatStack := func() string {
		if len(stack) == 0 {
			return "/"
		}
		var b strings.Builder
		for _, f := range stack {
			b.WriteByte('/')
			if f.isObject {
				if f.key == "" {
					return strings.TrimSuffix(b.String(), "/")
				}
				b.WriteString(strings.ReplaceAll(strings.ReplaceAll(f.key, "~", "~0"), "/", "~1"))
			} else {
				b.WriteString(strconv.Itoa(f.index))
			}
		}
		if b.Len() == 0 {
			return "/"
		}
		return b.String()
	}

	completeValue := func() {
		if len(stack) == 0 {
			return
		}
		top := &stack[len(stack)-1]
		if top.isObject {
			top.expectKey = true
		} else {
			top.index++
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "/"
		}
		if err != nil {
			return formatStack()
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, jsonPathFrame{isObject: true, expectKey: true})
			case '[':
				stack = append(stack, jsonPathFrame{isObject: false, index: 0})
			case '}', ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				completeValue()
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].isObject && stack[len(stack)-1].expectKey {
				stack[len(stack)-1].key = t
				stack[len(stack)-1].expectKey = false
			} else {
				completeValue()
			}
		default:
			completeValue()
		}
	}
}

func nonFiniteTokenNearSyntaxErr(raw string, err error) string {
	var se *json.SyntaxError
	if !errors.As(err, &se) || se.Offset <= 0 {
		return ""
	}
	idx := int(se.Offset) - 1
	if idx < 0 || idx >= len(raw) {
		return ""
	}
	rest := raw[idx:]
	for _, tok := range []string{"NaN", "Infinity", "-Infinity", "+Infinity"} {
		if strings.HasPrefix(rest, tok) {
			return tok
		}
	}
	return ""
}
