package httptest

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/exapsy/ene/e2eframe"
	"gopkg.in/yaml.v3"
)

type TestHeaderAssert struct {
	Name        string
	Contains    string
	NotContains string
	Equals      string
	NotEquals   string
	Matches     string
	NotMatches  string
	Present     *bool
}

// TestHeaderAsserts is a map where keys are header names and values are assertion specs
type TestHeaderAsserts map[string]interface{}

func (t *TestHeaderAsserts) UnmarshalYAML(node *yaml.Node) error {
	// Decode into a regular map
	var rawMap map[string]interface{}
	if err := node.Decode(&rawMap); err != nil {
		return fmt.Errorf("failed to decode header_asserts at line %d: %w", node.Line, err)
	}

	*t = rawMap
	return nil
}

// ToTestHeaderAssertList converts the map-based format to a list of TestHeaderAssert
func (t TestHeaderAsserts) ToTestHeaderAssertList() ([]TestHeaderAssert, error) {
	var asserts []TestHeaderAssert

	for name, value := range t {
		assert, err := parseHeaderAssertValue(name, value)
		if err != nil {
			return nil, fmt.Errorf("error parsing assertion for header %q: %w", name, err)
		}
		asserts = append(asserts, assert)
	}

	return asserts, nil
}

// parseHeaderAssertValue converts a value (string or map) into a TestHeaderAssert
func parseHeaderAssertValue(name string, value interface{}) (TestHeaderAssert, error) {
	assert := TestHeaderAssert{Name: name}

	// If value is a string, it's shorthand for "equals"
	if strValue, ok := value.(string); ok {
		assert.Equals = strValue
		return assert, nil
	}

	// If value is a map, parse the assertion specs
	mapValue, ok := value.(map[string]interface{})
	if !ok {
		return TestHeaderAssert{}, fmt.Errorf("assertion value must be either a string or an object, got %T", value)
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
	if v, ok := mapValue["equals"].(string); ok {
		assert.Equals = v
		setAssertions = append(setAssertions, "equals")
	}
	if v, ok := mapValue["not_equals"].(string); ok {
		assert.NotEquals = v
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

	// Parse boolean pointer
	if v, ok := mapValue["present"].(bool); ok {
		assert.Present = &v
		setAssertions = append(setAssertions, "present")
	}

	// Check if at least one assertion is provided
	if len(setAssertions) == 0 {
		return TestHeaderAssert{}, fmt.Errorf("at least one assertion must be provided for header %q", name)
	}

	// Validate assertion compatibility
	if err := validateHeaderAssertionCompatibility(setAssertions, name); err != nil {
		return TestHeaderAssert{}, err
	}

	return assert, nil
}

// validateHeaderAssertionCompatibility checks for conflicting assertions
func validateHeaderAssertionCompatibility(assertions []string, name string) error {
	hasEquals := containsString(assertions, "equals")
	hasNotEquals := containsString(assertions, "not_equals")
	hasContains := containsString(assertions, "contains")
	hasNotContains := containsString(assertions, "not_contains")
	hasMatches := containsString(assertions, "matches")
	hasNotMatches := containsString(assertions, "not_matches")

	// equals conflicts with almost everything except present
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
		if len(conflicts) > 0 {
			return fmt.Errorf("header %q: 'equals' assertion conflicts with: %s", name, strings.Join(conflicts, ", "))
		}
	}

	// contains and not_contains conflict
	if hasContains && hasNotContains {
		return fmt.Errorf("header %q: 'contains' and 'not_contains' cannot be used together", name)
	}

	// matches and not_matches conflict
	if hasMatches && hasNotMatches {
		return fmt.Errorf("header %q: 'matches' and 'not_matches' cannot be used together", name)
	}

	return nil
}

// containsString is a helper to check if a string slice contains a value
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func NewTestHeaderAssert(cfg map[string]any) (TestHeaderAssert, error) {
	var (
		name        string
		contains    string
		notContains string
		equals      string
		notEquals   string
		matches     string
		notMatches  string
		present     *bool
	)

	if v, ok := cfg["name"].(string); ok {
		name = v
	}

	if v, ok := cfg["contains"].(string); ok {
		contains = v
	}

	if v, ok := cfg["not_contains"].(string); ok {
		notContains = v
	}

	if v, ok := cfg["equals"].(string); ok {
		equals = v
	}

	if v, ok := cfg["not_equals"].(string); ok {
		notEquals = v
	}

	if v, ok := cfg["matches"].(string); ok {
		matches = v
	}

	if v, ok := cfg["not_matches"].(string); ok {
		notMatches = v
	}

	if v, ok := cfg["present"].(*bool); ok {
		present = v
	}

	assert := TestHeaderAssert{
		Name:        name,
		Contains:    contains,
		NotContains: notContains,
		Equals:      equals,
		NotEquals:   notEquals,
		Matches:     matches,
		NotMatches:  notMatches,
		Present:     present,
	}
	if !assert.IsValid() {
		return TestHeaderAssert{}, fmt.Errorf("invalid header assert configuration")
	}

	return assert, nil
}

func (e TestHeaderAssert) IsValid() bool {
	if e.Name == "" {
		return false
	}

	if e.Contains == "" && e.NotContains == "" && e.Equals == "" && e.NotEquals == "" &&
		e.Matches == "" &&
		e.NotMatches == "" &&
		e.Present == nil {
		return false
	}

	return true
}

type TestHeaderAssertTestOptions struct {
	Fixtures []e2eframe.Fixture
}

func (e TestHeaderAssert) Test(resp *http.Response, opts *TestHeaderAssertTestOptions) error {
	if e.Present != nil && *e.Present && len(resp.Header[e.Name]) == 0 {
		return fmt.Errorf("header %q is missing", e.Name)
	}

	// Interpolate fixture values in assertion fields
	contains := interpolateFixtureValue(e.Contains, opts.Fixtures)
	notContains := interpolateFixtureValue(e.NotContains, opts.Fixtures)
	equals := interpolateFixtureValue(e.Equals, opts.Fixtures)
	notEquals := interpolateFixtureValue(e.NotEquals, opts.Fixtures)
	matches := interpolateFixtureValue(e.Matches, opts.Fixtures)
	notMatches := interpolateFixtureValue(e.NotMatches, opts.Fixtures)

	if contains != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if !headerContains(resp.Header.Get(e.Name), contains) {
			return fmt.Errorf("header %q does not contain %q", e.Name, contains)
		}
	}

	if notContains != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if headerContains(resp.Header.Get(e.Name), e.NotContains) {
			return fmt.Errorf("header %q contains %q", e.Name, notContains)
		}
	}

	if equals != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if resp.Header.Get(e.Name) != e.Equals {
			return fmt.Errorf("header %q does not equal %q", e.Name, equals)
		}
	}

	if notEquals != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if resp.Header.Get(e.Name) == e.NotEquals {
			return fmt.Errorf("header %q equals %q", e.Name, notEquals)
		}
	}

	if matches != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if !headerMatches(resp.Header.Get(e.Name), e.Matches) {
			return fmt.Errorf("header %q does not match %q", e.Name, matches)
		}
	}

	if notMatches != "" {
		if _, ok := resp.Header[e.Name]; !ok {
			return fmt.Errorf("header %q not found", e.Name)
		}

		if headerMatches(resp.Header.Get(e.Name), e.NotMatches) {
			return fmt.Errorf("header %q matches %q", e.Name, notMatches)
		}
	}

	return nil
}

func headerContains(header string, value string) bool {
	if header == "" {
		return false
	}

	return strings.Contains(header, value)
}

func headerMatches(header, pattern string) bool {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}

	return regex.MatchString(header)
}
