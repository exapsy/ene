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

func NewTestBodyAssert(cfg map[string]any) (TestBodyAssert, error) {
	assert := TestBodyAssert{
		Path:        cfg["path"].(string),
		Contains:    cfg["contains"].(string),
		NotContains: cfg["not_contains"].(string),
		Equals:      cfg["equals"].(string),
		NotEquals:   cfg["not_equals"].(string),
		Matches:     cfg["matches"].(string),
		NotMatches:  cfg["not_matches"].(string),
		Present:     cfg["present"].(*bool),
		Size:        cfg["length"].(int),
		GreaterThan: cfg["greater_than"].(int),
		LessThan:    cfg["less_than"].(int),
		Type:        BodyFieldType(cfg["type"].(string)),
	}

	if !assert.IsValid() {
		return TestBodyAssert{}, fmt.Errorf("invalid body assert configuration")
	}

	return assert, nil
}

func (e TestBodyAssert) IsValid() bool {
	if e.Path == "" {
		return false
	}

	if e.Contains == "" && e.NotContains == "" && e.Equals == "" && e.NotEquals == "" &&
		e.Matches == "" &&
		e.NotMatches == "" &&
		e.Present == nil &&
		e.Size == 0 &&
		e.GreaterThan == 0 &&
		e.LessThan == 0 {
		return false
	}

	if !e.Type.IsValid() {
		return false
	}

	return true
}

type TestBodyAssertTestOptions struct {
	Fixtures []e2eframe.Fixture
}

func (e TestBodyAssert) Test(r *http.Response, opts *TestBodyAssertTestOptions) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
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
		done, err := e.testRoot(body, contains, notContains, equals, notEquals, matches, notMatches)
		if done {
			return err
		}
	} else {
		err, done := e.testField(body, contains, notContains, equals, notEquals, matches, notMatches)
		if done {
			return err
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

	if contains != "" && !strings.Contains(value.String(), contains) {
		return fmt.Errorf("value does not contain: %s", e.Contains), true
	}

	if notContains != "" && strings.Contains(value.String(), notContains) {
		return fmt.Errorf("value contains: %s", notContains), true
	}

	if equals != "" && value.String() != equals {
		return fmt.Errorf("value does not equal: %s", equals), true
	}

	if notEquals != "" && value.String() == notEquals {
		return fmt.Errorf("value equals: %s", notEquals), true
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
	value := gjson.Get(string(body), e.Path)
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
