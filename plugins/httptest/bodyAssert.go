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

	// Array containment assertions
	ContainsWhere map[string]interface{}
	AllMatch      map[string]interface{}
	NoneMatch     map[string]interface{}
}

// TestBodyAsserts is a map where keys are JSON paths and values are assertion specs
type TestBodyAsserts map[string]interface{}

func (t *TestBodyAsserts) UnmarshalYAML(node *yaml.Node) error {
	// Decode into a regular map
	var rawMap map[string]interface{}
	if err := node.Decode(&rawMap); err != nil {
		return fmt.Errorf("failed to decode body_asserts at line %d: %w", node.Line, err)
	}

	*t = rawMap
	return nil
}

// ToTestBodyAssertList converts the map-based format to a list of TestBodyAssert
func (t TestBodyAsserts) ToTestBodyAssertList() ([]TestBodyAssert, error) {
	var asserts []TestBodyAssert

	for path, value := range t {
		assert, err := parseBodyAssertValue(path, value)
		if err != nil {
			return nil, fmt.Errorf("error parsing assertion for path %q: %w", path, err)
		}
		asserts = append(asserts, assert)
	}

	return asserts, nil
}

// parseBodyAssertValue converts a value (string or map) into a TestBodyAssert
func parseBodyAssertValue(path string, value interface{}) (TestBodyAssert, error) {
	assert := TestBodyAssert{Path: path}

	// Handle shorthand values (string, int, float, bool, null)
	switch v := value.(type) {
	case string:
		assert.Equals = v
		return assert, nil
	case int:
		assert.Equals = fmt.Sprintf("%d", v)
		return assert, nil
	case int64:
		assert.Equals = fmt.Sprintf("%d", v)
		return assert, nil
	case float64:
		assert.Equals = fmt.Sprintf("%v", v)
		return assert, nil
	case bool:
		assert.Equals = fmt.Sprintf("%t", v)
		return assert, nil
	case nil:
		assert.Equals = "null"
		return assert, nil
	}

	// If value is a map, parse the assertion specs
	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return TestBodyAssert{}, fmt.Errorf("assertion value must be either a string, number, boolean, null, or an object, got %T", value)
	}

	// Track which assertions are set for conflict detection
	setAssertions := []string{}

	// Parse string assertions
	if v, ok := mapValue["contains"].(string); ok {
		assert.Contains = v
		setAssertions = append(setAssertions, "contains")
	}
	if v, ok := mapValue["not_contains"].(string); ok {
		assert.NotContains = v
		setAssertions = append(setAssertions, "not_contains")
	}
	// Handle equals with multiple types
	if v, ok := mapValue["equals"]; ok {
		switch val := v.(type) {
		case string:
			assert.Equals = val
		case int, int64:
			assert.Equals = fmt.Sprintf("%d", val)
		case float64:
			assert.Equals = fmt.Sprintf("%v", val)
		case bool:
			assert.Equals = fmt.Sprintf("%t", val)
		case nil:
			assert.Equals = "null"
		default:
			return TestBodyAssert{}, fmt.Errorf("equals value must be a string, number, boolean, or null, got %T", v)
		}
		setAssertions = append(setAssertions, "equals")
	}
	// Handle not_equals with multiple types
	if v, ok := mapValue["not_equals"]; ok {
		switch val := v.(type) {
		case string:
			assert.NotEquals = val
		case int, int64:
			assert.NotEquals = fmt.Sprintf("%d", val)
		case float64:
			assert.NotEquals = fmt.Sprintf("%v", val)
		case bool:
			assert.NotEquals = fmt.Sprintf("%t", val)
		case nil:
			assert.NotEquals = "null"
		default:
			return TestBodyAssert{}, fmt.Errorf("not_equals value must be a string, number, boolean, or null, got %T", v)
		}
		setAssertions = append(setAssertions, "not_equals")
	}
	if v, ok := mapValue["matches"].(string); ok {
		assert.Matches = v
		setAssertions = append(setAssertions, "matches")
	}
	if v, ok := mapValue["not_matches"].(string); ok {
		assert.NotMatches = v
		setAssertions = append(setAssertions, "not_matches")
	}

	// Parse symbol operators
	if v, ok := mapValue[">"].(int); ok {
		assert.GreaterThan = v
		setAssertions = append(setAssertions, ">")
	}
	if v, ok := mapValue["<"].(int); ok {
		assert.LessThan = v
		setAssertions = append(setAssertions, "<")
	}
	// Aliases
	if v, ok := mapValue["greater_than"].(int); ok {
		assert.GreaterThan = v
		setAssertions = append(setAssertions, ">")
	}
	if v, ok := mapValue["less_than"].(int); ok {
		assert.LessThan = v
		setAssertions = append(setAssertions, "<")
	}

	// Parse boolean pointer
	if v, ok := mapValue["present"].(bool); ok {
		assert.Present = &v
		setAssertions = append(setAssertions, "present")
	}

	// Parse size/length
	if v, ok := mapValue["length"].(int); ok {
		assert.Size = v
		setAssertions = append(setAssertions, "length")
	}
	if v, ok := mapValue["size"].(int); ok {
		assert.Size = v
		setAssertions = append(setAssertions, "length")
	}

	// Parse type
	if v, ok := mapValue["type"].(string); ok {
		assert.Type = BodyFieldType(v)
		setAssertions = append(setAssertions, "type")
		if !assert.Type.IsValid() {
			return TestBodyAssert{}, fmt.Errorf("invalid type %q for path %q", v, path)
		}
	}

	// Parse array containment assertions
	if v, ok := mapValue["contains_where"].(map[string]interface{}); ok {
		assert.ContainsWhere = v
		setAssertions = append(setAssertions, "contains_where")
	}
	if v, ok := mapValue["all_match"].(map[string]interface{}); ok {
		assert.AllMatch = v
		setAssertions = append(setAssertions, "all_match")
	}
	if v, ok := mapValue["none_match"].(map[string]interface{}); ok {
		assert.NoneMatch = v
		setAssertions = append(setAssertions, "none_match")
	}

	// Check if at least one assertion is provided
	if len(setAssertions) == 0 {
		return TestBodyAssert{}, fmt.Errorf("at least one assertion must be provided for path %q", path)
	}

	// Validate assertion compatibility
	if err := validateAssertionCompatibility(setAssertions, path); err != nil {
		return TestBodyAssert{}, err
	}

	return assert, nil
}

// validateAssertionCompatibility checks for conflicting assertions
func validateAssertionCompatibility(assertions []string, path string) error {
	hasEquals := contains(assertions, "equals")
	hasNotEquals := contains(assertions, "not_equals")
	hasContains := contains(assertions, "contains")
	hasNotContains := contains(assertions, "not_contains")
	hasMatches := contains(assertions, "matches")
	hasNotMatches := contains(assertions, "not_matches")
	hasGreaterThan := contains(assertions, ">")
	hasLessThan := contains(assertions, "<")

	// equals conflicts with almost everything except present and type
	if hasEquals {
		conflicts := []string{}
		if hasNotEquals {
			conflicts = append(conflicts, "not_equals")
		}
		if hasContains {
			conflicts = append(conflicts, "contains")
		}
		if hasNotContains {
			conflicts = append(conflicts, "not_contains")
		}
		if hasMatches {
			conflicts = append(conflicts, "matches")
		}
		if hasNotMatches {
			conflicts = append(conflicts, "not_matches")
		}
		if hasGreaterThan {
			conflicts = append(conflicts, ">")
		}
		if hasLessThan {
			conflicts = append(conflicts, "<")
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("path %q: 'equals' assertion conflicts with: %s", path, strings.Join(conflicts, ", "))
		}
	}

	// contains and not_contains conflict
	if hasContains && hasNotContains {
		return fmt.Errorf("path %q: 'contains' and 'not_contains' cannot be used together", path)
	}

	// matches and not_matches conflict
	if hasMatches && hasNotMatches {
		return fmt.Errorf("path %q: 'matches' and 'not_matches' cannot be used together", path)
	}

	return nil
}

// contains is a helper to check if a string slice contains a value
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

	// Test array containment assertions
	if e.ContainsWhere != nil {
		if !value.IsArray() {
			return fmt.Errorf("contains_where can only be used on arrays: %s", e.Path), true
		}
		if err := e.testArrayContainsWhere(value); err != nil {
			return err, true
		}
	}

	if e.AllMatch != nil {
		if !value.IsArray() {
			return fmt.Errorf("all_match can only be used on arrays: %s", e.Path), true
		}
		if err := e.testArrayAllMatch(value); err != nil {
			return err, true
		}
	}

	if e.NoneMatch != nil {
		if !value.IsArray() {
			return fmt.Errorf("none_match can only be used on arrays: %s", e.Path), true
		}
		if err := e.testArrayNoneMatch(value); err != nil {
			return err, true
		}
	}

	if contains != "" {
		actualValue := value.String()
		if !strings.Contains(actualValue, contains) {
			return fmt.Errorf("value does not contain %q (got: %q)", contains, actualValue), true
		}
	}

	if notContains != "" {
		actualValue := value.String()
		if strings.Contains(actualValue, notContains) {
			return fmt.Errorf("value contains %q (got: %q)", notContains, actualValue), true
		}
	}

	if equals != "" {
		actualValue := value.String()
		if actualValue != equals {
			return fmt.Errorf("expected %q but got %q", equals, actualValue), true
		}
	}

	if notEquals != "" {
		actualValue := value.String()
		if actualValue == notEquals {
			return fmt.Errorf("value equals %q (should not equal, but got: %q)", notEquals, actualValue), true
		}
	}

	if matches != "" {
		actualValue := value.String()
		res, err := bodyMatches(matches, value)
		if err != nil {
			return fmt.Errorf("value does not match regex %q, error: %w (got: %q)", matches, err, actualValue), true
		}

		if !res {
			return fmt.Errorf("value does not match regex %q (got: %q)", matches, actualValue), true
		}
	}

	if notMatches != "" {
		actualValue := value.String()
		res, err := bodyMatches(notMatches, value)
		if err != nil {
			return fmt.Errorf("value matches regex %q, error: %w (got: %q)", notMatches, err, actualValue), true
		}

		if res {
			return fmt.Errorf("value matches regex %q (should not match, but got: %q)", notMatches, actualValue), true
		}
	}

	if e.Size != 0 {
		switch value.Type {
		case gjson.String:
			actualSize := len(value.String())
			if actualSize != e.Size {
				return fmt.Errorf("expected size %d but got %d", e.Size, actualSize), true
			}
		case gjson.JSON:
			if value.IsArray() {
				arr := value.Array()
				actualSize := len(arr)
				if actualSize != e.Size {
					return fmt.Errorf("expected array size %d but got %d", e.Size, actualSize), true
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
			actualValue := value.Int()
			if actualValue <= int64(e.GreaterThan) {
				return fmt.Errorf("expected value > %d but got %d", e.GreaterThan, actualValue), true
			}
		default:
			return fmt.Errorf("value is not a number: %s", e.Path), true
		}
	}

	if e.LessThan != 0 {
		switch value.Type {
		case gjson.Number:
			actualValue := value.Int()
			if actualValue >= int64(e.LessThan) {
				return fmt.Errorf("expected value < %d but got %d", e.LessThan, actualValue), true
			}
		default:
			return fmt.Errorf("value is not a number: %s", e.Path), true
		}
	}

	if e.Type != "" {
		switch e.Type {
		case BodyFieldTypeString:
			if value.Type != gjson.String {
				return fmt.Errorf("expected type 'string' but got type '%s' at path: %s", value.Type.String(), e.Path), true
			}
		case BodyFieldTypeInt:
			if value.Type != gjson.Number {
				return fmt.Errorf("expected type 'int' but got type '%s' at path: %s (value: %q)", value.Type.String(), e.Path, value.String()), true
			}

			// gjson does not provide a way to check if a number is an integer
			if !strings.Contains(value.String(), ".") {
				if value.Type != gjson.Number {
					return fmt.Errorf("expected type 'int' but got type '%s' at path: %s (value: %q)", value.Type.String(), e.Path, value.String()), true
				}
			} else {
				return fmt.Errorf("expected type 'int' but got float at path: %s (value: %q)", e.Path, value.String()), true
			}
		case BodyFieldTypeFloat:
			if value.Type != gjson.Number {
				return fmt.Errorf("expected type 'float' but got type '%s' at path: %s (value: %q)", value.Type.String(), e.Path, value.String()), true
			}

			// gjson does not provide a way to check if a number is a float
			if strings.Contains(value.String(), ".") {
				if value.Type != gjson.Number {
					return fmt.Errorf("expected type 'float' but got type '%s' at path: %s (value: %q)", value.Type.String(), e.Path, value.String()), true
				}
			} else {
				return fmt.Errorf("expected type 'float' but got int at path: %s (value: %q)", e.Path, value.String()), true
			}
		case BodyFieldTypeBool:
			if value.Type != gjson.True && value.Type != gjson.False {
				return fmt.Errorf("expected type 'bool' but got type '%s' at path: %s (value: %q)", value.Type.String(), e.Path, value.String()), true
			}
		case BodyFieldTypeArray:
			if !value.IsArray() {
				return fmt.Errorf("expected type 'array' but got type '%s' at path: %s", value.Type.String(), e.Path), true
			}
		case BodyFieldTypeObject:
			if !value.IsObject() {
				return fmt.Errorf("expected type 'object' but got type '%s' at path: %s", value.Type.String(), e.Path), true
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

// testArrayContainsWhere checks if at least one array element matches all conditions
func (e TestBodyAssert) testArrayContainsWhere(value *gjson.Result) error {
	if !value.IsArray() {
		return fmt.Errorf("contains_where requires an array at path: %s", e.Path)
	}

	array := value.Array()
	if len(array) == 0 {
		return fmt.Errorf("array is empty, cannot match contains_where conditions at path: %s", e.Path)
	}

	for _, item := range array {
		if matchesConditions(&item, e.ContainsWhere) {
			return nil // Found at least one match
		}
	}

	return fmt.Errorf("no array element matches contains_where conditions at path: %s", e.Path)
}

// testArrayAllMatch checks if all array elements match the conditions
func (e TestBodyAssert) testArrayAllMatch(value *gjson.Result) error {
	if !value.IsArray() {
		return fmt.Errorf("all_match requires an array at path: %s", e.Path)
	}

	array := value.Array()
	if len(array) == 0 {
		return nil // Empty array vacuously satisfies all_match
	}

	for i, item := range array {
		if !matchesConditions(&item, e.AllMatch) {
			return fmt.Errorf("array element at index %d does not match all_match conditions at path: %s", i, e.Path)
		}
	}

	return nil
}

// testArrayNoneMatch checks if no array elements match the conditions
func (e TestBodyAssert) testArrayNoneMatch(value *gjson.Result) error {
	if !value.IsArray() {
		return fmt.Errorf("none_match requires an array at path: %s", e.Path)
	}

	array := value.Array()
	for i, item := range array {
		if matchesConditions(&item, e.NoneMatch) {
			return fmt.Errorf("array element at index %d matches none_match conditions (should not match) at path: %s", i, e.Path)
		}
	}

	return nil
}

// matchesConditions checks if a gjson.Result matches all conditions in the map
func matchesConditions(item *gjson.Result, conditions map[string]interface{}) bool {
	for key, expectedValue := range conditions {
		fieldValue := item.Get(key)

		// Handle different types of conditions
		switch v := expectedValue.(type) {
		case string:
			// Simple equality check
			if fieldValue.String() != v {
				return false
			}
		case int, int64, float64:
			// Numeric equality
			if fieldValue.Num != toFloat64(v) {
				return false
			}
		case bool:
			// Boolean equality
			if fieldValue.Bool() != v {
				return false
			}
		case map[string]interface{}:
			// Nested conditions (operators like >, <, etc.)
			if !matchesNestedConditions(&fieldValue, v) {
				return false
			}
		default:
			// Fallback to string comparison
			if fieldValue.String() != fmt.Sprint(v) {
				return false
			}
		}
	}
	return true
}

// matchesNestedConditions handles operator-based conditions like { ">": 100 }
func matchesNestedConditions(fieldValue *gjson.Result, conditions map[string]interface{}) bool {
	for operator, value := range conditions {
		numValue := toFloat64(value)
		fieldNum := fieldValue.Num

		switch operator {
		case ">", "greater_than":
			if fieldNum <= numValue {
				return false
			}
		case "<", "less_than":
			if fieldNum >= numValue {
				return false
			}
		case ">=":
			if fieldNum < numValue {
				return false
			}
		case "<=":
			if fieldNum > numValue {
				return false
			}
		case "equals":
			if strVal, ok := value.(string); ok {
				if fieldValue.String() != strVal {
					return false
				}
			} else {
				if fieldNum != numValue {
					return false
				}
			}
		case "not_equals":
			if strVal, ok := value.(string); ok {
				if fieldValue.String() == strVal {
					return false
				}
			} else {
				if fieldNum == numValue {
					return false
				}
			}
		case "contains":
			if strVal, ok := value.(string); ok {
				if !strings.Contains(fieldValue.String(), strVal) {
					return false
				}
			}
		case "matches":
			if strVal, ok := value.(string); ok {
				regex, err := regexp.Compile(strVal)
				if err != nil || !regex.MatchString(fieldValue.String()) {
					return false
				}
			}
		case "present":
			if boolVal, ok := value.(bool); ok {
				if boolVal && !fieldValue.Exists() {
					return false
				}
				if !boolVal && fieldValue.Exists() {
					return false
				}
			}
		case "type":
			if strVal, ok := value.(string); ok {
				if !matchesType(fieldValue, strVal) {
					return false
				}
			}
		}
	}
	return true
}

// matchesType checks if a value matches the expected type
func matchesType(value *gjson.Result, expectedType string) bool {
	switch expectedType {
	case "string":
		return value.Type == gjson.String
	case "int", "float", "number":
		return value.Type == gjson.Number
	case "bool", "boolean":
		return value.Type == gjson.True || value.Type == gjson.False
	case "array":
		return value.IsArray()
	case "object":
		return value.IsObject()
	case "null":
		return value.Type == gjson.Null
	default:
		return false
	}
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) float64 {
	switch num := v.(type) {
	case int:
		return float64(num)
	case int64:
		return float64(num)
	case float64:
		return num
	case float32:
		return float64(num)
	default:
		return 0
	}
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
