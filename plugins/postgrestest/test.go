package postgrestest

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/exapsy/ene/e2eframe"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

const (
	Kind e2eframe.TestSuiteTestKind = "postgres"
)

type TestSuiteTest struct {
	TestName         string
	TestKind         string
	Query            string                `yaml:"query"`
	Expect           *PostgresExpectations `yaml:"expect"`
	PostgresEndpoint string
	testSuite        e2eframe.TestSuite
}

type PostgresExpectations struct {
	RowCount     *int                     `yaml:"row_count"`
	Rows         []map[string]interface{} `yaml:"rows"`
	TableExists  string                   `yaml:"table_exists"`
	ColumnValues map[string]interface{}   `yaml:"column_values"`
	NoRows       bool                     `yaml:"no_rows"`
	MinRowCount  *int                     `yaml:"min_row_count"`
	MaxRowCount  *int                     `yaml:"max_row_count"`
	Contains     []map[string]interface{} `yaml:"contains"`
	NotContains  []map[string]interface{} `yaml:"not_contains"`
}

func init() {
	e2eframe.RegisterTestSuiteTestUnmarshaler(
		Kind,
		func(node *yaml.Node) (e2eframe.TestSuiteTest, error) {
			test := &TestSuiteTest{}
			if err := test.UnmarshalYAML(node); err != nil {
				return nil, err
			}
			return test, nil
		},
	)
}

func (t *TestSuiteTest) Name() string {
	return t.TestName
}

func (t *TestSuiteTest) Kind() string {
	return t.TestKind
}

func (t *TestSuiteTest) Initialize(testSuite e2eframe.TestSuite) error {
	t.testSuite = testSuite

	// Find the target unit and get Postgres connection details
	target := testSuite.Target()
	if target == nil {
		return fmt.Errorf("target unit not found")
	}

	// Get Postgres connection details from the target unit
	dsn, err := target.Get("dsn")
	if err != nil {
		return fmt.Errorf("failed to get postgres DSN: %w", err)
	}
	t.PostgresEndpoint = dsn

	return nil
}

func (t *TestSuiteTest) Run(ctx context.Context, opts *e2eframe.TestSuiteTestRunOptions) (*e2eframe.TestResult, error) {
	startTime := time.Now()

	// Connect to Postgres
	db, err := sql.Open("postgres", t.PostgresEndpoint)
	if err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect to Postgres: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}
	defer db.Close()

	// Set connection timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Failed to ping Postgres: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}

	// Run expectations
	if err := t.runExpectations(ctx, db, opts); err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Expectation failed: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}

	return &e2eframe.TestResult{
		TestName: t.TestName,
		Passed:   true,
		Message:  "All expectations passed",
		Duration: time.Since(startTime),
	}, nil
}

func (t *TestSuiteTest) runExpectations(ctx context.Context, db *sql.DB, opts *e2eframe.TestSuiteTestRunOptions) error {
	if t.Expect == nil {
		return fmt.Errorf("no expectations provided")
	}

	// Interpolate fixtures in table_exists if needed
	tableExists := t.Expect.TableExists
	if opts != nil && len(opts.Fixtures) > 0 && tableExists != "" {
		interpolationRegex := e2eframe.FixtureInterpolationRegex
		if interpolationRegex.MatchString(tableExists) {
			tableExists = e2eframe.InterpolateString(interpolationRegex, tableExists, opts.Fixtures)
		}
	}

	// Check table existence
	if tableExists != "" {
		if err := t.verifyTableExists(ctx, db, tableExists); err != nil {
			return err
		}
		// If only checking table existence, return early
		if t.Query == "" && t.Expect.RowCount == nil && len(t.Expect.Rows) == 0 {
			return nil
		}
	}

	// If no query provided but we have other expectations, error
	if t.Query == "" {
		return fmt.Errorf("no query provided for expectations")
	}

	// Interpolate fixtures in query if needed
	query := t.Query
	if opts != nil && len(opts.Fixtures) > 0 {
		interpolationRegex := e2eframe.FixtureInterpolationRegex
		if interpolationRegex.MatchString(query) {
			query = e2eframe.InterpolateString(interpolationRegex, query, opts.Fixtures)
		}
	}

	// Interpolate fixtures in expectations
	if opts != nil && len(opts.Fixtures) > 0 {
		// Interpolate rows expectations
		if len(t.Expect.Rows) > 0 {
			t.Expect.Rows = t.interpolateExpectationRows(t.Expect.Rows, opts.Fixtures)
		}
		// Interpolate column_values expectations
		if len(t.Expect.ColumnValues) > 0 {
			t.Expect.ColumnValues = t.interpolateColumnValues(t.Expect.ColumnValues, opts.Fixtures)
		}
		// Interpolate contains expectations
		if len(t.Expect.Contains) > 0 {
			t.Expect.Contains = t.interpolateExpectationRows(t.Expect.Contains, opts.Fixtures)
		}
		// Interpolate not_contains expectations
		if len(t.Expect.NotContains) > 0 {
			t.Expect.NotContains = t.interpolateExpectationRows(t.Expect.NotContains, opts.Fixtures)
		}
	}

	// Log query in verbose mode
	if opts != nil && opts.Verbose {
		fmt.Printf("\n=== Postgres Query ===\n")
		fmt.Printf("%s\n", query)
		fmt.Printf("======================\n\n")
	}

	// Execute query
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	// Collect all rows
	var results []map[string]interface{}
	for rows.Next() {
		// Create a slice of interface{} to hold each column value
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for easier comparison
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			rowMap[col] = val
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	// Log results in verbose mode
	if opts != nil && opts.Verbose {
		fmt.Printf("=== Query Results ===\n")
		fmt.Printf("Row count: %d\n", len(results))
		if len(results) > 0 {
			fmt.Printf("Columns: %v\n", columns)
			for i, row := range results {
				fmt.Printf("Row %d: %v\n", i+1, row)
			}
		}
		fmt.Printf("=====================\n\n")
	}

	// Verify expectations
	return t.verifyExpectations(results, columns)
}

func (t *TestSuiteTest) verifyTableExists(ctx context.Context, db *sql.DB, tableName string) error {
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)
	`
	var exists bool
	err := db.QueryRowContext(ctx, query, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("table '%s' does not exist", tableName)
	}

	return nil
}

func (t *TestSuiteTest) verifyExpectations(results []map[string]interface{}, columns []string) error {
	actualRowCount := len(results)

	// Check no_rows
	if t.Expect.NoRows {
		if actualRowCount > 0 {
			return fmt.Errorf("expected no rows, but got %d rows", actualRowCount)
		}
		return nil
	}

	// Check exact row count
	if t.Expect.RowCount != nil {
		if actualRowCount != *t.Expect.RowCount {
			return fmt.Errorf("expected %d rows, but got %d rows", *t.Expect.RowCount, actualRowCount)
		}
	}

	// Check min row count
	if t.Expect.MinRowCount != nil {
		if actualRowCount < *t.Expect.MinRowCount {
			return fmt.Errorf("expected at least %d rows, but got %d rows", *t.Expect.MinRowCount, actualRowCount)
		}
	}

	// Check max row count
	if t.Expect.MaxRowCount != nil {
		if actualRowCount > *t.Expect.MaxRowCount {
			return fmt.Errorf("expected at most %d rows, but got %d rows", *t.Expect.MaxRowCount, actualRowCount)
		}
	}

	// Check exact rows match
	if len(t.Expect.Rows) > 0 {
		if err := t.verifyExactRows(results, t.Expect.Rows); err != nil {
			return err
		}
	}

	// Check column values (for single row results)
	if len(t.Expect.ColumnValues) > 0 {
		if actualRowCount == 0 {
			return fmt.Errorf("expected column values but got no rows")
		}
		if actualRowCount > 1 {
			return fmt.Errorf("column_values assertion requires exactly 1 row, but got %d rows", actualRowCount)
		}
		if err := t.verifyColumnValues(results[0], t.Expect.ColumnValues); err != nil {
			return err
		}
	}

	// Check contains
	if len(t.Expect.Contains) > 0 {
		if err := t.verifyContains(results, t.Expect.Contains); err != nil {
			return err
		}
	}

	// Check not_contains
	if len(t.Expect.NotContains) > 0 {
		if err := t.verifyNotContains(results, t.Expect.NotContains); err != nil {
			return err
		}
	}

	return nil
}

// interpolateExpectationValue interpolates fixture values in expectation values
func (t *TestSuiteTest) interpolateExpectationValue(value interface{}, fixtures []e2eframe.Fixture) interface{} {
	if len(fixtures) == 0 {
		return value
	}

	// Handle string interpolation
	if strVal, ok := value.(string); ok {
		interpolationRegex := e2eframe.FixtureInterpolationRegex
		if interpolationRegex.MatchString(strVal) {
			return e2eframe.InterpolateString(interpolationRegex, strVal, fixtures)
		}
		return strVal
	}

	// Handle map interpolation (for row expectations)
	if mapVal, ok := value.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for k, v := range mapVal {
			result[k] = t.interpolateExpectationValue(v, fixtures)
		}
		return result
	}

	// Handle array interpolation
	if arrVal, ok := value.([]interface{}); ok {
		result := make([]interface{}, len(arrVal))
		for i, v := range arrVal {
			result[i] = t.interpolateExpectationValue(v, fixtures)
		}
		return result
	}

	// Return as-is for other types (int, bool, etc.)
	return value
}

// interpolateExpectationRows interpolates fixtures in row expectations
func (t *TestSuiteTest) interpolateExpectationRows(rows []map[string]interface{}, fixtures []e2eframe.Fixture) []map[string]interface{} {
	if len(fixtures) == 0 {
		return rows
	}

	result := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		interpolatedRow := make(map[string]interface{})
		for k, v := range row {
			interpolatedRow[k] = t.interpolateExpectationValue(v, fixtures)
		}
		result[i] = interpolatedRow
	}
	return result
}

// interpolateColumnValues interpolates fixtures in column value expectations
func (t *TestSuiteTest) interpolateColumnValues(colVals map[string]interface{}, fixtures []e2eframe.Fixture) map[string]interface{} {
	if len(fixtures) == 0 {
		return colVals
	}

	result := make(map[string]interface{})
	for k, v := range colVals {
		result[k] = t.interpolateExpectationValue(v, fixtures)
	}
	return result
}

func (t *TestSuiteTest) verifyExactRows(actual, expected []map[string]interface{}) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("expected %d rows, but got %d rows", len(expected), len(actual))
	}

	for i, expectedRow := range expected {
		actualRow := actual[i]
		if err := t.compareRow(actualRow, expectedRow, fmt.Sprintf("row %d", i+1)); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestSuiteTest) verifyColumnValues(row map[string]interface{}, expected map[string]interface{}) error {
	for column, expectedValue := range expected {
		actualValue, exists := row[column]
		if !exists {
			return fmt.Errorf("column '%s' not found in result", column)
		}

		if err := t.compareValue(actualValue, expectedValue, column); err != nil {
			return err
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyContains(results []map[string]interface{}, expected []map[string]interface{}) error {
	for _, expectedRow := range expected {
		found := false
		for _, actualRow := range results {
			if t.rowMatches(actualRow, expectedRow) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected row not found in results: %v", expectedRow)
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyNotContains(results []map[string]interface{}, notExpected []map[string]interface{}) error {
	for _, notExpectedRow := range notExpected {
		for _, actualRow := range results {
			if t.rowMatches(actualRow, notExpectedRow) {
				return fmt.Errorf("found row that should not exist: %v", notExpectedRow)
			}
		}
	}
	return nil
}

func (t *TestSuiteTest) rowMatches(actualRow, expectedRow map[string]interface{}) bool {
	// Check if all expected columns match
	for column, expectedValue := range expectedRow {
		actualValue, exists := actualRow[column]
		if !exists {
			return false
		}
		if !t.valuesEqual(actualValue, expectedValue) {
			return false
		}
	}
	return true
}

func (t *TestSuiteTest) compareRow(actual, expected map[string]interface{}, rowDesc string) error {
	for column, expectedValue := range expected {
		actualValue, exists := actual[column]
		if !exists {
			return fmt.Errorf("%s: column '%s' not found in result", rowDesc, column)
		}

		if err := t.compareValue(actualValue, expectedValue, fmt.Sprintf("%s, column '%s'", rowDesc, column)); err != nil {
			return err
		}
	}
	return nil
}

func (t *TestSuiteTest) compareValue(actual, expected interface{}, location string) error {
	if !t.valuesEqual(actual, expected) {
		return fmt.Errorf("%s: expected %v (%T), got %v (%T)", location, expected, expected, actual, actual)
	}
	return nil
}

func (t *TestSuiteTest) valuesEqual(actual, expected interface{}) bool {
	// Handle nil cases
	if actual == nil && expected == nil {
		return true
	}
	if actual == nil || expected == nil {
		return false
	}

	// Convert numeric types for comparison
	actualVal := reflect.ValueOf(actual)
	expectedVal := reflect.ValueOf(expected)

	// Handle numeric comparisons
	if t.isNumeric(actualVal) && t.isNumeric(expectedVal) {
		return t.compareNumeric(actualVal, expectedVal)
	}

	// Handle string comparisons
	actualStr, actualIsStr := actual.(string)
	expectedStr, expectedIsStr := expected.(string)
	if actualIsStr && expectedIsStr {
		return actualStr == expectedStr
	}

	// Handle bool comparisons
	actualBool, actualIsBool := actual.(bool)
	expectedBool, expectedIsBool := expected.(bool)
	if actualIsBool && expectedIsBool {
		return actualBool == expectedBool
	}

	// Fallback to deep equal
	return reflect.DeepEqual(actual, expected)
}

func (t *TestSuiteTest) isNumeric(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func (t *TestSuiteTest) compareNumeric(actual, expected reflect.Value) bool {
	actualFloat := t.toFloat64(actual)
	expectedFloat := t.toFloat64(expected)
	return actualFloat == expectedFloat
}

func (t *TestSuiteTest) toFloat64(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		return 0
	}
}

func (t *TestSuiteTest) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "name":
			if err := value.Decode(&t.TestName); err != nil {
				return fmt.Errorf("failed to decode test name: %w", err)
			}
		case "kind":
			if err := value.Decode(&t.TestKind); err != nil {
				return fmt.Errorf("failed to decode test kind: %w", err)
			}
		case "query":
			if err := value.Decode(&t.Query); err != nil {
				return fmt.Errorf("failed to decode query: %w", err)
			}
		case "expect":
			if err := value.Decode(&t.Expect); err != nil {
				return fmt.Errorf("failed to decode expect: %w", err)
			}
		default:
			// Ignore unknown fields for forward compatibility
			continue
		}
	}

	if t.TestName == "" {
		return fmt.Errorf("test name is required")
	}

	if t.TestKind == "" {
		t.TestKind = string(Kind)
	}

	if t.Expect == nil {
		return fmt.Errorf("expect is required")
	}

	// Validate that at least one expectation is provided
	hasExpectation := t.Expect.RowCount != nil ||
		t.Expect.MinRowCount != nil ||
		t.Expect.MaxRowCount != nil ||
		len(t.Expect.Rows) > 0 ||
		t.Expect.TableExists != "" ||
		len(t.Expect.ColumnValues) > 0 ||
		t.Expect.NoRows ||
		len(t.Expect.Contains) > 0 ||
		len(t.Expect.NotContains) > 0

	if !hasExpectation {
		return fmt.Errorf("at least one expectation must be provided")
	}

	return nil
}
