package httptest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/exapsy/ene/e2eframe"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

type TestBodyAssert struct {
	Path        string
	Contains    string
	NotContains string
	Equals      string
	NotEquals   string
	Matches     string
	NotMatches  string
	Present     *bool
	Size        int
	GreaterThan int
	LessThan    int
	Type        BodyFieldType
}

func (t *TestBodyAssert) UnmarshalYAML(node *yaml.Node) error {
	// Create a temporary map to safely extract values
	var rawData map[string]interface{}
	if err := node.Decode(&rawData); err != nil {
		return fmt.Errorf("failed to decode body assert at line %d: %w", node.Line, err)
	}

	// Use the safe NewTestBodyAssert function for validation and type checking
	bodyAssert, err := NewTestBodyAssert(rawData)
	if err != nil {
		// If it's already a BodyAssertError, just return it
		if userErr, ok := err.(*e2eframe.BodyAssertError); ok {
			userErr.Line = node.Line
			return userErr
		}
		return err
	}

	// Copy the validated values to this instance
	*t = bodyAssert
	return nil
}

func NewTestBodyAssert(cfg map[string]any) (TestBodyAssert, error) {
	assert := TestBodyAssert{}

	// Safely extract path (required)
	if path, ok := cfg["path"].(string); ok {
		assert.Path = path
	} else {
		return TestBodyAssert{}, e2eframe.NewBodyAssertValidationError("missing_path", "", nil, "", 0)
	}

	// Safely extract optional string fields
	if contains, ok := cfg["contains"].(string); ok {
		assert.Contains = contains
	}
	if notContains, ok := cfg["not_contains"].(string); ok {
		assert.NotContains = notContains
	}
	if equals, ok := cfg["equals"].(string); ok {
		assert.Equals = equals
	}
	if notEquals, ok := cfg["not_equals"].(string); ok {
		assert.NotEquals = notEquals
	}
	if matches, ok := cfg["matches"].(string); ok {
		assert.Matches = matches
	}
	if notMatches, ok := cfg["not_matches"].(string); ok {
		assert.NotMatches = notMatches
	}
	if typeStr, ok := cfg["type"].(string); ok {
		assert.Type = BodyFieldType(typeStr)
	}

	// Safely extract optional bool pointer
	if present, ok := cfg["present"].(bool); ok {
		assert.Present = &present
	}

	// Safely extract optional int fields
	if size, ok := cfg["length"].(int); ok {
		assert.Size = size
	}
	if greaterThan, ok := cfg["greater_than"].(int); ok {
		assert.GreaterThan = greaterThan
	}
	if lessThan, ok := cfg["less_than"].(int); ok {
		assert.LessThan = lessThan
	}

	// Validate path first
	if assert.Path == "" {
		return TestBodyAssert{}, e2eframe.NewBodyAssertValidationError("empty_path", assert.Path, nil, "", 0)
	}

	// Check if at least one assertion condition is provided
	hasCondition := assert.Contains != "" || assert.NotContains != "" || assert.Equals != "" || assert.NotEquals != "" ||
		assert.Matches != "" || assert.NotMatches != "" || assert.Present != nil ||
		assert.Size > 0 || assert.GreaterThan > 0 || assert.LessThan > 0

	if !hasCondition {
		return TestBodyAssert{}, e2eframe.NewBodyAssertValidationError("no_conditions", assert.Path, nil, "", 0)
	}

	// Validate type if set
	if assert.Type != "" && !assert.Type.IsValid() {
		return TestBodyAssert{}, e2eframe.NewBodyAssertValidationError("invalid_type", assert.Path, string(assert.Type), "", 0)
	}

	return assert, nil
}

func (e TestBodyAssert) IsValid() bool {
	if e.Path == "" {
		return false
	}

	// At least one assertion condition must be provided
	hasCondition := e.Contains != "" || e.NotContains != "" || e.Equals != "" || e.NotEquals != "" ||
		e.Matches != "" || e.NotMatches != "" || e.Present != nil ||
		e.Size > 0 || e.GreaterThan > 0 || e.LessThan > 0

	if !hasCondition {
		return false
	}

	// Only validate Type if it's set
	if e.Type != "" && !e.Type.IsValid() {
		return false
	}

	return true
}

type TestBodyAssertTestOptions struct {
	Fixtures []e2eframe.Fixture
}

func (e TestBodyAssert) Test(r *http.Response, opts *TestBodyAssertTestOptions) (err error) {
	// Add panic recovery to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("body assertion panic: %v (path: %s)", r, e.Path)
		}
	}()

	if r == nil {
		return fmt.Errorf("http response is nil")
	}

	if r.Body == nil {
		return fmt.Errorf("http response body is nil")
	}

	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read body: %w", readErr)
	}

	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Interpolate fixture values in assertion fields
	contains := interpolateFixtureValue(e.Contains, opts.Fixtures)
	notContains := interpolateFixtureValue(e.NotContains, opts.Fixtures)
	equals := interpolateFixtureValue(e.Equals, opts.Fixtures)
	notEquals := interpolateFixtureValue(e.NotEquals, opts.Fixtures)
	matches := interpolateFixtureValue(e.Matches, opts.Fixtures)
	notMatches := interpolateFixtureValue(e.NotMatches, opts.Fixtures)

	// check whole body, $ = root
	if e.Path == "$" {
		done, testErr := e.testRoot(body, contains, notContains, equals, notEquals, matches, notMatches)
		if done {
			return testErr
		}
	} else {
		testErr, done := e.testField(body, contains, notContains, equals, notEquals, matches, notMatches)
		if done {
			return testErr
		}
	}

	return nil
}

func (e TestBodyAssert) testRoot(
	body []byte,
	contains string,
	notContains string,
	equals string,
	notEquals string,
	matches string,
	notMatches string,
) (bool, error) {
	if e.Present != nil && !*e.Present && len(body) > 0 {
		return false, fmt.Errorf("expected body to not be present")
	}

	if e.Present != nil && *e.Present && len(body) == 0 {
		return false, fmt.Errorf("expected body to be present but its empty")
	}

	// Check if the response body is valid JSON before parsing
	if !gjson.Valid(string(body)) {
		return false, fmt.Errorf("response body is not valid JSON for root path assertion")
	}

	gjsonResult := gjson.ParseBytes(body)

	if matches != "" {
		res, err := bodyMatches(matches, &gjsonResult)
		if err != nil {
			return false, fmt.Errorf("body does not match regex: %s, error: %w", matches, err)
		}

		if !res {
			return false, fmt.Errorf("body does not match regex: %s", matches)
		}
	}

	if notMatches != "" {
		res, err := bodyMatches(notMatches, &gjsonResult)
		if err != nil {
			return false, fmt.Errorf("body matches regex: %s, error: %w", notMatches, err)
		}

		if res {
			return false, fmt.Errorf("body matches regex: %s", notMatches)
		}
	}

	if contains != "" && !strings.Contains(gjsonResult.String(), contains) {
		return false, fmt.Errorf("body does not contain: %s", contains)
	}

	if notContains != "" && strings.Contains(gjsonResult.String(), notContains) {
		return false, fmt.Errorf("body contains: %s", notContains)
	}

	if equals != "" && gjsonResult.String() != equals {
		return false, fmt.Errorf("body does not equal: %s", equals)
	}

	if notEquals != "" && gjsonResult.String() == notEquals {
		return false, fmt.Errorf("body equals: %s", notEquals)
	}

	if e.Size != 0 {
		switch gjsonResult.Type {
		case gjson.String:
			if len(gjsonResult.String()) != e.Size {
				return false, fmt.Errorf("body size does not equal: %d", e.Size)
			}
		case gjson.JSON:
			if gjsonResult.IsArray() {
				arr := gjsonResult.Array()
				if len(arr) != e.Size {
					return false, fmt.Errorf("array size does not equal: %d", e.Size)
				}
			} else {
				return false, fmt.Errorf("body is not an array")
			}
		default:
			return false, fmt.Errorf("body is not a string or array")
		}
	}

	if e.GreaterThan != 0 {
		switch gjsonResult.Type {
		case gjson.Number:
			if gjsonResult.Int() <= int64(e.GreaterThan) {
				return false, fmt.Errorf("body is not greater than: %d", e.GreaterThan)
			}
		default:
			return false, fmt.Errorf("body is not a number")
		}
	}

	if e.LessThan != 0 {
		switch gjsonResult.Type {
		case gjson.Number:
			if gjsonResult.Int() >= int64(e.LessThan) {
				return false, fmt.Errorf("body is not less than: %d", e.LessThan)
			}
		default:
			return false, fmt.Errorf("body is not a number")
		}
	}

	if e.Type != "" {
		switch gjsonResult.Type {
		case gjson.String:
			if e.Type != BodyFieldTypeString {
				return false, fmt.Errorf("body is not a string")
			}
		case gjson.Number:
			if e.Type != BodyFieldTypeInt && e.Type != BodyFieldTypeFloat {
				return false, fmt.Errorf("body is not a number")
			}

			// gjson does not provide a way to check if a number is an integer or float
			if e.Type == BodyFieldTypeInt && strings.Contains(gjsonResult.String(), ".") {
				return false, fmt.Errorf("body is not an integer")
			}

			if e.Type == BodyFieldTypeFloat && !strings.Contains(gjsonResult.String(), ".") {
				return false, fmt.Errorf("body is not a float")
			}
		case gjson.JSON:
			if e.Type != BodyFieldTypeArray && e.Type != BodyFieldTypeObject {
				return false, fmt.Errorf("body is not an array or object")
			}
		case gjson.Null:
			if e.Present != nil && *e.Present {
				return false, fmt.Errorf("expected body to be present but it is null")
			}
		case gjson.True, gjson.False:
			if e.Type != BodyFieldTypeBool {
				return false, fmt.Errorf("body is not a boolean")
			}
		}
	}

	return true, nil
}

func (e TestBodyAssert) testField(
	body []byte,
	contains string,
	notContains string,
	equals string,
	notEquals string,
	matches string,
	notMatches string,
) (error, bool) {
	value, err := e.getValueFromPathWithBody(body)
	if err != nil && e.Present != nil && *e.Present {
		return fmt.Errorf(
			"expected value to be present at path: %s, but it was not found, error: %w",
			e.Path,
			err,
		), true
	}

	if err == nil && e.Present != nil && !*e.Present {
		return fmt.Errorf("expected value to NOT be present at path: %s", e.Path), false
	}

	// If there was an error getting the value or value is nil, we can't perform the remaining assertions
	if err != nil || value == nil {
		return nil, false
	}

	// Additional safety check to ensure value pointer is valid
	if value == nil {
		return fmt.Errorf("nil value returned for path: %s", e.Path), true
	}

	if contains != "" {
		if !strings.Contains(value.String(), contains) {
			return fmt.Errorf("value does not contain: %s", contains), true
		}
	}

	if notContains != "" {
		if strings.Contains(value.String(), notContains) {
			return fmt.Errorf("value contains: %s", notContains), true
		}
	}

	if equals != "" {
		if value.String() != equals {
			return fmt.Errorf("value does not equal: %s", equals), true
		}
	}

	if notEquals != "" {
		if value.String() == notEquals {
			return fmt.Errorf("value equals: %s", notEquals), true
		}
	}

	if matches != "" {
		res, err := bodyMatches(matches, value)
		if err != nil {
			return fmt.Errorf("value does not match regex: %s, error: %w", matches, err), true
		}

		if !res {
			return fmt.Errorf("value does not match regex: %s", matches), true
		}
	}

	if notMatches != "" {
		res, err := bodyMatches(notMatches, value)
		if err != nil {
			return fmt.Errorf("value matches regex: %s, error: %w", notMatches, err), true
		}

		if res {
			return fmt.Errorf("value matches regex: %s", notMatches), true
		}
	}

	if e.Size != 0 {
		switch value.Type {
		case gjson.String:
			if len(value.String()) != e.Size {
				return fmt.Errorf("value size does not equal: %d", e.Size), true
			}
		case gjson.JSON:
			if value.IsArray() {
				arr := value.Array()
				if len(arr) != e.Size {
					return fmt.Errorf("array size does not equal: %d", e.Size), true
				}
			} else {
				return fmt.Errorf("value is not an array: %s", e.Path), true
			}
		default:
			return fmt.Errorf("value is not a string or array: %s", e.Path), true
		}
	}

	if e.GreaterThan != 0 {
		switch value.Type {
		case gjson.Number:
			if value.Int() <= int64(e.GreaterThan) {
				return fmt.Errorf("value is not greater than: %d", e.GreaterThan), true
			}
		default:
			return fmt.Errorf("value is not a number: %s", e.Path), true
		}
	}

	if e.LessThan != 0 {
		switch value.Type {
		case gjson.Number:
			if value.Int() >= int64(e.LessThan) {
				return fmt.Errorf("value is not less than: %d", e.LessThan), true
			}
		default:
			return fmt.Errorf("value is not a number: %s", e.Path), true
		}
	}

	if e.Type != "" {
		switch e.Type {
		case BodyFieldTypeString:
			if value.Type != gjson.String {
				return fmt.Errorf("value is not a string: %s", e.Path), true
			}
		case BodyFieldTypeInt:
			if value.Type != gjson.Number {
				return fmt.Errorf("value is not a number: %s", e.Path), true
			}

			// gjson does not provide a way to check if a number is an integer
			if !strings.Contains(value.String(), ".") {
				if value.Type != gjson.Number {
					return fmt.Errorf("value is not an integer: %s", e.Path), true
				}
			} else {
				return fmt.Errorf("value is not an integer: %s", e.Path), true
			}
		case BodyFieldTypeFloat:
			if value.Type != gjson.Number {
				return fmt.Errorf("value is not a float: %s", e.Path), true
			}

			// gjson does not provide a way to check if a number is a float
			if strings.Contains(value.String(), ".") {
				if value.Type != gjson.Number {
					return fmt.Errorf("value is not a float: %s", e.Path), true
				}
			} else {
				return fmt.Errorf("value is not a float: %s", e.Path), true
			}
		case BodyFieldTypeBool:
			if value.Type != gjson.True && value.Type != gjson.False {
				return fmt.Errorf("value is not a boolean: %s", e.Path), true
			}
		case BodyFieldTypeArray:
			if !value.IsArray() {
				return fmt.Errorf("value is not an array: %s", e.Path), true
			}
		case BodyFieldTypeObject:
			if !value.IsObject() {
				return fmt.Errorf("value is not an object: %s", e.Path), true
			}
		default:
			return fmt.Errorf("invalid type: %s", e.Type), true
		}
	}

	return nil, false
}

func bodyMatches(matches string, value *gjson.Result) (result bool, err error) {
	if value == nil {
		return false, fmt.Errorf("cannot match regex against nil value")
	}

	regex, err := regexp.Compile(matches)
	if err != nil {
		return false, fmt.Errorf("invalid regex: %w", err)
	}

	if !regex.MatchString(value.String()) {
		return false, nil
	}

	return true, nil
}

func (e TestBodyAssert) getValueFromPathWithBody(body []byte) (*gjson.Result, error) {
	if body == nil {
		return nil, fmt.Errorf("cannot parse nil body")
	}

	if e.Path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	// Safely convert body to string and parse with gjson
	bodyStr := string(body)
	if bodyStr == "" {
		return nil, fmt.Errorf("empty response body")
	}

	// Check if the response body is valid JSON before parsing
	if !gjson.Valid(bodyStr) {
		return nil, fmt.Errorf("response body is not valid JSON (path: %s)", e.Path)
	}

	value := gjson.Get(bodyStr, e.Path)
	if !value.Exists() {
		return nil, fmt.Errorf("value not found at path: %q", e.Path)
	}

	return &value, nil
}

// interpolateFixtureValue replaces fixture references in the provided string.
func interpolateFixtureValue(value string, fixtures []e2eframe.Fixture) string {
	if value == "" || len(fixtures) == 0 {
		return value
	}

	result := value
	// Regex to match {{ fixture_name }} pattern
	interpolationRegex := e2eframe.FixtureInterpolationRegex

	if interpolationRegex.MatchString(result) {
		return e2eframe.InterpolateString(interpolationRegex, result, fixtures)
	}

	return result
}
