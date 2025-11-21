package postgrestest

import (
	"testing"

	"github.com/exapsy/ene/e2eframe"
	"gopkg.in/yaml.v3"
)

func TestUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(*testing.T, *TestSuiteTest)
	}{
		{
			name: "basic query with row count",
			yaml: `
name: "test query"
kind: postgres
query: "SELECT * FROM users"
expect:
  row_count: 5
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.TestName != "test query" {
					t.Errorf("expected name 'test query', got '%s'", test.TestName)
				}
				if test.Query != "SELECT * FROM users" {
					t.Errorf("expected query 'SELECT * FROM users', got '%s'", test.Query)
				}
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if test.Expect.RowCount == nil || *test.Expect.RowCount != 5 {
					t.Errorf("expected row_count 5, got %v", test.Expect.RowCount)
				}
			},
		},
		{
			name: "table exists check",
			yaml: `
name: "check table exists"
kind: postgres
expect:
  table_exists: "users"
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if test.Expect.TableExists != "users" {
					t.Errorf("expected table_exists 'users', got '%s'", test.Expect.TableExists)
				}
			},
		},
		{
			name: "exact rows match",
			yaml: `
name: "verify user data"
kind: postgres
query: "SELECT id, email FROM users WHERE id = 1"
expect:
  row_count: 1
  rows:
    - id: 1
      email: "test@example.com"
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if len(test.Expect.Rows) != 1 {
					t.Fatalf("expected 1 row in expect.rows, got %d", len(test.Expect.Rows))
				}
				row := test.Expect.Rows[0]
				if row["id"] != 1 {
					t.Errorf("expected id 1, got %v", row["id"])
				}
				if row["email"] != "test@example.com" {
					t.Errorf("expected email 'test@example.com', got %v", row["email"])
				}
			},
		},
		{
			name: "no rows expectation",
			yaml: `
name: "verify no orphaned records"
kind: postgres
query: "SELECT * FROM orders WHERE user_id NOT IN (SELECT id FROM users)"
expect:
  no_rows: true
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if !test.Expect.NoRows {
					t.Errorf("expected no_rows true, got false")
				}
			},
		},
		{
			name: "min and max row count",
			yaml: `
name: "check user count range"
kind: postgres
query: "SELECT * FROM users"
expect:
  min_row_count: 1
  max_row_count: 100
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if test.Expect.MinRowCount == nil || *test.Expect.MinRowCount != 1 {
					t.Errorf("expected min_row_count 1, got %v", test.Expect.MinRowCount)
				}
				if test.Expect.MaxRowCount == nil || *test.Expect.MaxRowCount != 100 {
					t.Errorf("expected max_row_count 100, got %v", test.Expect.MaxRowCount)
				}
			},
		},
		{
			name: "column values assertion",
			yaml: `
name: "check specific column values"
kind: postgres
query: "SELECT COUNT(*) as total FROM users"
expect:
  column_values:
    total: 42
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if len(test.Expect.ColumnValues) != 1 {
					t.Fatalf("expected 1 column value, got %d", len(test.Expect.ColumnValues))
				}
				if test.Expect.ColumnValues["total"] != 42 {
					t.Errorf("expected total 42, got %v", test.Expect.ColumnValues["total"])
				}
			},
		},
		{
			name: "contains assertion",
			yaml: `
name: "verify user exists"
kind: postgres
query: "SELECT * FROM users"
expect:
  contains:
    - id: 1
      email: "admin@example.com"
    - id: 2
      email: "user@example.com"
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if len(test.Expect.Contains) != 2 {
					t.Fatalf("expected 2 rows in contains, got %d", len(test.Expect.Contains))
				}
			},
		},
		{
			name: "not_contains assertion",
			yaml: `
name: "verify deleted users"
kind: postgres
query: "SELECT * FROM users"
expect:
  not_contains:
    - email: "deleted@example.com"
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect == nil {
					t.Fatal("expect is nil")
				}
				if len(test.Expect.NotContains) != 1 {
					t.Fatalf("expected 1 row in not_contains, got %d", len(test.Expect.NotContains))
				}
			},
		},
		{
			name: "missing name",
			yaml: `
kind: postgres
query: "SELECT * FROM users"
expect:
  row_count: 5
`,
			wantErr: true,
		},
		{
			name: "missing expect",
			yaml: `
name: "test"
kind: postgres
query: "SELECT * FROM users"
`,
			wantErr: true,
		},
		{
			name: "empty expect",
			yaml: `
name: "test"
kind: postgres
query: "SELECT * FROM users"
expect: {}
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.yaml), &node); err != nil {
				t.Fatalf("failed to unmarshal yaml: %v", err)
			}

			// The unmarshaled node is a document node, we need the content
			if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
				t.Fatalf("expected document node with content")
			}

			test := &TestSuiteTest{}
			err := test.UnmarshalYAML(node.Content[0])

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.check != nil {
				tt.check(t, test)
			}
		})
	}
}

func TestValuesEqual(t *testing.T) {
	test := &TestSuiteTest{}

	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
		want     bool
	}{
		{"nil equals nil", nil, nil, true},
		{"nil not equals value", nil, "test", false},
		{"value not equals nil", "test", nil, false},
		{"string equals", "hello", "hello", true},
		{"string not equals", "hello", "world", false},
		{"int equals int", 42, 42, true},
		{"int not equals int", 42, 43, false},
		{"int64 equals int", int64(42), 42, true},
		{"float64 equals int", 42.0, 42, true},
		{"float64 equals float32", 42.0, float32(42.0), true},
		{"bool equals", true, true, true},
		{"bool not equals", true, false, false},
		{"different types not equal", "42", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := test.valuesEqual(tt.actual, tt.expected)
			if got != tt.want {
				t.Errorf("valuesEqual(%v, %v) = %v, want %v", tt.actual, tt.expected, got, tt.want)
			}
		})
	}
}

func TestRowMatches(t *testing.T) {
	test := &TestSuiteTest{}

	tests := []struct {
		name        string
		actualRow   map[string]interface{}
		expectedRow map[string]interface{}
		want        bool
	}{
		{
			name:        "exact match",
			actualRow:   map[string]interface{}{"id": 1, "name": "test"},
			expectedRow: map[string]interface{}{"id": 1, "name": "test"},
			want:        true,
		},
		{
			name:        "partial match (expected subset)",
			actualRow:   map[string]interface{}{"id": 1, "name": "test", "email": "test@example.com"},
			expectedRow: map[string]interface{}{"id": 1, "name": "test"},
			want:        true,
		},
		{
			name:        "no match - different value",
			actualRow:   map[string]interface{}{"id": 1, "name": "test"},
			expectedRow: map[string]interface{}{"id": 2, "name": "test"},
			want:        false,
		},
		{
			name:        "no match - missing column",
			actualRow:   map[string]interface{}{"id": 1},
			expectedRow: map[string]interface{}{"id": 1, "name": "test"},
			want:        false,
		},
		{
			name:        "numeric type compatibility",
			actualRow:   map[string]interface{}{"count": int64(42)},
			expectedRow: map[string]interface{}{"count": 42},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := test.rowMatches(tt.actualRow, tt.expectedRow)
			if got != tt.want {
				t.Errorf("rowMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindRegistration(t *testing.T) {
	// Test that the kind is properly registered
	if !e2eframe.TestSuiteTestKindExists(Kind) {
		t.Errorf("postgres test kind not registered")
	}
}

func TestName(t *testing.T) {
	test := &TestSuiteTest{TestName: "my test"}
	if test.Name() != "my test" {
		t.Errorf("expected name 'my test', got '%s'", test.Name())
	}
}

func TestKind(t *testing.T) {
	test := &TestSuiteTest{TestKind: "postgres"}
	if test.Kind() != "postgres" {
		t.Errorf("expected kind 'postgres', got '%s'", test.Kind())
	}
}

func TestVerifyExpectations(t *testing.T) {
	tests := []struct {
		name        string
		results     []map[string]interface{}
		expect      *PostgresExpectations
		wantErr     bool
		errContains string
	}{
		{
			name: "exact row count match",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "test1"},
				{"id": int64(2), "name": "test2"},
			},
			expect: &PostgresExpectations{
				RowCount: intPtr(2),
			},
			wantErr: false,
		},
		{
			name: "row count mismatch",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "test1"},
			},
			expect: &PostgresExpectations{
				RowCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected 2 rows, but got 1 rows",
		},
		{
			name:    "no_rows with empty results",
			results: []map[string]interface{}{},
			expect: &PostgresExpectations{
				NoRows: true,
			},
			wantErr: false,
		},
		{
			name: "no_rows with results fails",
			results: []map[string]interface{}{
				{"id": int64(1)},
			},
			expect: &PostgresExpectations{
				NoRows: true,
			},
			wantErr:     true,
			errContains: "expected no rows",
		},
		{
			name: "min_row_count pass",
			results: []map[string]interface{}{
				{"id": int64(1)},
				{"id": int64(2)},
			},
			expect: &PostgresExpectations{
				MinRowCount: intPtr(1),
			},
			wantErr: false,
		},
		{
			name: "min_row_count fail",
			results: []map[string]interface{}{
				{"id": int64(1)},
			},
			expect: &PostgresExpectations{
				MinRowCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected at least 2 rows",
		},
		{
			name: "max_row_count pass",
			results: []map[string]interface{}{
				{"id": int64(1)},
			},
			expect: &PostgresExpectations{
				MaxRowCount: intPtr(2),
			},
			wantErr: false,
		},
		{
			name: "max_row_count fail",
			results: []map[string]interface{}{
				{"id": int64(1)},
				{"id": int64(2)},
				{"id": int64(3)},
			},
			expect: &PostgresExpectations{
				MaxRowCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected at most 2 rows",
		},
		{
			name: "column_values match",
			results: []map[string]interface{}{
				{"count": int64(42), "status": "active"},
			},
			expect: &PostgresExpectations{
				ColumnValues: map[string]interface{}{
					"count":  42,
					"status": "active",
				},
			},
			wantErr: false,
		},
		{
			name: "column_values mismatch",
			results: []map[string]interface{}{
				{"count": int64(42)},
			},
			expect: &PostgresExpectations{
				ColumnValues: map[string]interface{}{
					"count": 43,
				},
			},
			wantErr:     true,
			errContains: "expected 43",
		},
		{
			name: "contains match",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "alice"},
				{"id": int64(2), "name": "bob"},
				{"id": int64(3), "name": "charlie"},
			},
			expect: &PostgresExpectations{
				Contains: []map[string]interface{}{
					{"id": 1, "name": "alice"},
					{"id": 3, "name": "charlie"},
				},
			},
			wantErr: false,
		},
		{
			name: "contains not found",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "alice"},
			},
			expect: &PostgresExpectations{
				Contains: []map[string]interface{}{
					{"id": 2, "name": "bob"},
				},
			},
			wantErr:     true,
			errContains: "expected row not found",
		},
		{
			name: "not_contains pass",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "alice"},
			},
			expect: &PostgresExpectations{
				NotContains: []map[string]interface{}{
					{"id": 2, "name": "bob"},
				},
			},
			wantErr: false,
		},
		{
			name: "not_contains fail",
			results: []map[string]interface{}{
				{"id": int64(1), "name": "alice"},
				{"id": int64(2), "name": "bob"},
			},
			expect: &PostgresExpectations{
				NotContains: []map[string]interface{}{
					{"id": 2, "name": "bob"},
				},
			},
			wantErr:     true,
			errContains: "found row that should not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := &TestSuiteTest{
				Expect: tt.expect,
			}

			err := test.verifyExpectations(tt.results, []string{})

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Helper functions

func intPtr(i int) *int {
	return &i
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestFixtureInterpolation(t *testing.T) {
	test := &TestSuiteTest{}

	fixtures := []e2eframe.Fixture{
		&e2eframe.FixtureV1{
			FixtureName:  "user_id",
			FixtureValue: "123",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "user_email",
			FixtureValue: "test@example.com",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "count",
			FixtureValue: "42",
		},
	}

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "string with fixture",
			input:    "user_{{ user_id }}",
			expected: "user_123",
		},
		{
			name:     "string without fixture",
			input:    "no fixtures here",
			expected: "no fixtures here",
		},
		{
			name:     "multiple fixtures in string",
			input:    "{{ user_email }} - {{ user_id }}",
			expected: "test@example.com - 123",
		},
		{
			name:     "integer value",
			input:    42,
			expected: 42,
		},
		{
			name:     "boolean value",
			input:    true,
			expected: true,
		},
		{
			name: "map with fixtures",
			input: map[string]interface{}{
				"id":    "{{ user_id }}",
				"email": "{{ user_email }}",
				"count": 10,
			},
			expected: map[string]interface{}{
				"id":    "123",
				"email": "test@example.com",
				"count": 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := test.interpolateExpectationValue(tt.input, fixtures)

			// For maps, we need deep comparison
			if mapResult, ok := result.(map[string]interface{}); ok {
				expectedMap := tt.expected.(map[string]interface{})
				for k, v := range expectedMap {
					if mapResult[k] != v {
						t.Errorf("map[%s] = %v, want %v", k, mapResult[k], v)
					}
				}
			} else {
				if result != tt.expected {
					t.Errorf("got %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestInterpolateExpectationRows(t *testing.T) {
	test := &TestSuiteTest{}

	fixtures := []e2eframe.Fixture{
		&e2eframe.FixtureV1{
			FixtureName:  "admin_email",
			FixtureValue: "admin@example.com",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "user_role",
			FixtureValue: "admin",
		},
	}

	input := []map[string]interface{}{
		{
			"email": "{{ admin_email }}",
			"role":  "{{ user_role }}",
			"id":    1,
		},
		{
			"email": "user@example.com",
			"role":  "user",
			"id":    2,
		},
	}

	result := test.interpolateExpectationRows(input, fixtures)

	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}

	// Check first row - should be interpolated
	if result[0]["email"] != "admin@example.com" {
		t.Errorf("row 0 email: got %v, want admin@example.com", result[0]["email"])
	}
	if result[0]["role"] != "admin" {
		t.Errorf("row 0 role: got %v, want admin", result[0]["role"])
	}
	if result[0]["id"] != 1 {
		t.Errorf("row 0 id: got %v, want 1", result[0]["id"])
	}

	// Check second row - should be unchanged
	if result[1]["email"] != "user@example.com" {
		t.Errorf("row 1 email: got %v, want user@example.com", result[1]["email"])
	}
	if result[1]["role"] != "user" {
		t.Errorf("row 1 role: got %v, want user", result[1]["role"])
	}
}

func TestInterpolateColumnValues(t *testing.T) {
	test := &TestSuiteTest{}

	fixtures := []e2eframe.Fixture{
		&e2eframe.FixtureV1{
			FixtureName:  "expected_count",
			FixtureValue: "100",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "status",
			FixtureValue: "active",
		},
	}

	input := map[string]interface{}{
		"count":  "{{ expected_count }}",
		"status": "{{ status }}",
		"other":  42,
	}

	result := test.interpolateColumnValues(input, fixtures)

	if result["count"] != "100" {
		t.Errorf("count: got %v, want 100", result["count"])
	}
	if result["status"] != "active" {
		t.Errorf("status: got %v, want active", result["status"])
	}
	if result["other"] != 42 {
		t.Errorf("other: got %v, want 42", result["other"])
	}
}

func TestInterpolateExpectationRowsEmptyFixtures(t *testing.T) {
	test := &TestSuiteTest{}

	input := []map[string]interface{}{
		{
			"email": "{{ admin_email }}",
			"role":  "admin",
		},
	}

	// With no fixtures, should return unchanged
	result := test.interpolateExpectationRows(input, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}

	// Should be unchanged since no fixtures provided
	if result[0]["email"] != "{{ admin_email }}" {
		t.Errorf("email should be unchanged, got %v", result[0]["email"])
	}
}
