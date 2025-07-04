package httptest

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/exapsy/ene/e2eframe"
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
