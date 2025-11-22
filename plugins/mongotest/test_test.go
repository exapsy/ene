package mongotest

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
			name: "valid test with filter",
			yaml: `
name: test1
kind: mongo
collection: users
filter: '{"status": "active"}'
expect:
  document_count: 5
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.TestName != "test1" {
					t.Errorf("expected name 'test1', got '%s'", test.TestName)
				}
				if test.Collection != "users" {
					t.Errorf("expected collection 'users', got '%s'", test.Collection)
				}
				if test.Filter != `{"status": "active"}` {
					t.Errorf("expected filter, got %v", test.Filter)
				}
				if test.Expect.DocumentCount == nil || *test.Expect.DocumentCount != 5 {
					t.Errorf("expected document_count 5")
				}
			},
		},
		{
			name: "valid test with pipeline",
			yaml: `
name: test2
kind: mongo
collection: orders
pipeline:
  - $group:
      _id: "$status"
      count: { $sum: 1 }
expect:
  min_document_count: 1
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.TestName != "test2" {
					t.Errorf("expected name 'test2', got '%s'", test.TestName)
				}
				if test.Pipeline == nil {
					t.Errorf("expected pipeline to be set")
				}
			},
		},
		{
			name: "collection_exists only",
			yaml: `
name: test3
kind: mongo
expect:
  collection_exists: users
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect.CollectionExists != "users" {
					t.Errorf("expected collection_exists 'users', got '%s'", test.Expect.CollectionExists)
				}
			},
		},
		{
			name: "no_documents expectation",
			yaml: `
name: test4
kind: mongo
collection: deleted_users
filter: '{}'
expect:
  no_documents: true
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if !test.Expect.NoDocuments {
					t.Errorf("expected no_documents to be true")
				}
			},
		},
		{
			name: "exact documents match",
			yaml: `
name: test5
kind: mongo
collection: users
filter: '{"id": 1}'
expect:
  document_count: 1
  documents:
    - id: 1
      name: Alice
      email: alice@example.com
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if len(test.Expect.Documents) != 1 {
					t.Errorf("expected 1 document in expectations")
				}
			},
		},
		{
			name: "field_values expectation",
			yaml: `
name: test6
kind: mongo
collection: stats
filter: '{}'
expect:
  field_values:
    total: 100
    active: 50
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if len(test.Expect.FieldValues) != 2 {
					t.Errorf("expected 2 field values")
				}
			},
		},
		{
			name: "contains expectation",
			yaml: `
name: test7
kind: mongo
collection: users
filter: '{"role": "admin"}'
expect:
  contains:
    - email: admin@example.com
      role: admin
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if len(test.Expect.Contains) != 1 {
					t.Errorf("expected 1 contains expectation")
				}
			},
		},
		{
			name: "not_contains expectation",
			yaml: `
name: test8
kind: mongo
collection: users
filter: '{}'
expect:
  not_contains:
    - email: deleted@example.com
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if len(test.Expect.NotContains) != 1 {
					t.Errorf("expected 1 not_contains expectation")
				}
			},
		},
		{
			name: "min and max document count",
			yaml: `
name: test9
kind: mongo
collection: users
filter: '{}'
expect:
  min_document_count: 5
  max_document_count: 100
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.Expect.MinDocumentCount == nil || *test.Expect.MinDocumentCount != 5 {
					t.Errorf("expected min_document_count 5")
				}
				if test.Expect.MaxDocumentCount == nil || *test.Expect.MaxDocumentCount != 100 {
					t.Errorf("expected max_document_count 100")
				}
			},
		},
		{
			name: "per-test target override",
			yaml: `
name: test10
kind: mongo
target: db2
collection: users
filter: '{}'
expect:
  document_count: 1
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if test.TestTarget != "db2" {
					t.Errorf("expected target 'db2', got '%s'", test.TestTarget)
				}
			},
		},
		{
			name: "debug flag",
			yaml: `
name: test11
kind: mongo
debug: true
collection: users
filter: '{}'
expect:
  document_count: 1
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				if !test.Debug {
					t.Errorf("expected debug to be true")
				}
			},
		},
		{
			name: "missing name",
			yaml: `
kind: mongo
collection: users
filter: '{}'
expect:
  document_count: 1
`,
			wantErr: true,
		},
		{
			name: "missing expect",
			yaml: `
name: test12
kind: mongo
collection: users
filter: '{}'
`,
			wantErr: true,
		},
		{
			name: "no expectations provided",
			yaml: `
name: test13
kind: mongo
collection: users
filter: '{}'
expect: {}
`,
			wantErr: true,
		},
		{
			name: "YAML filter structure",
			yaml: `
name: test14
kind: mongo
collection: users
filter:
  status: active
  age:
    $gte: 18
expect:
  min_document_count: 1
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				filterMap, ok := test.Filter.(map[string]interface{})
				if !ok {
					t.Errorf("expected filter to be a map")
				}
				if filterMap["status"] != "active" {
					t.Errorf("expected status to be 'active'")
				}
			},
		},
		{
			name: "YAML pipeline structure",
			yaml: `
name: test15
kind: mongo
collection: users
pipeline:
  - $match:
      status: active
  - $group:
      _id: "$role"
      count: { $sum: 1 }
expect:
  min_document_count: 1
`,
			wantErr: false,
			check: func(t *testing.T, test *TestSuiteTest) {
				pipelineArray, ok := test.Pipeline.([]interface{})
				if !ok {
					t.Errorf("expected pipeline to be an array")
				}
				if len(pipelineArray) != 2 {
					t.Errorf("expected 2 pipeline stages, got %d", len(pipelineArray))
				}
			},
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
		{"equal strings", "hello", "hello", true},
		{"unequal strings", "hello", "world", false},
		{"equal ints", 42, 42, true},
		{"equal int types", int32(42), int64(42), true},
		{"equal floats", 3.14, 3.14, true},
		{"int and float equal", 42, 42.0, true},
		{"equal bools", true, true, true},
		{"unequal bools", true, false, false},
		{"nil values", nil, nil, true},
		{"nil vs value", nil, "hello", false},
		{"value vs nil", "hello", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := test.valuesEqual(tt.actual, tt.expected)
			if got != tt.want {
				t.Errorf("valuesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDocumentMatches(t *testing.T) {
	test := &TestSuiteTest{}

	tests := []struct {
		name        string
		actualDoc   map[string]interface{}
		expectedDoc map[string]interface{}
		want        bool
	}{
		{
			name:        "exact match",
			actualDoc:   map[string]interface{}{"id": 1, "name": "Alice"},
			expectedDoc: map[string]interface{}{"id": 1, "name": "Alice"},
			want:        true,
		},
		{
			name:        "partial match - expected subset",
			actualDoc:   map[string]interface{}{"id": 1, "name": "Alice", "email": "alice@example.com"},
			expectedDoc: map[string]interface{}{"id": 1, "name": "Alice"},
			want:        true,
		},
		{
			name:        "no match - different values",
			actualDoc:   map[string]interface{}{"id": 1, "name": "Alice"},
			expectedDoc: map[string]interface{}{"id": 1, "name": "Bob"},
			want:        false,
		},
		{
			name:        "no match - missing field",
			actualDoc:   map[string]interface{}{"id": 1},
			expectedDoc: map[string]interface{}{"id": 1, "name": "Alice"},
			want:        false,
		},
		{
			name:        "match with numeric types",
			actualDoc:   map[string]interface{}{"id": int64(1), "count": int32(5)},
			expectedDoc: map[string]interface{}{"id": 1, "count": 5},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := test.documentMatches(tt.actualDoc, tt.expectedDoc)
			if got != tt.want {
				t.Errorf("documentMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindRegistration(t *testing.T) {
	if Kind != "mongo" {
		t.Errorf("expected Kind to be 'mongo', got '%s'", Kind)
	}
}

func TestName(t *testing.T) {
	test := &TestSuiteTest{TestName: "my-test"}
	if test.Name() != "my-test" {
		t.Errorf("expected Name() to return 'my-test', got '%s'", test.Name())
	}
}

func TestKind(t *testing.T) {
	test := &TestSuiteTest{TestKind: "mongo"}
	if test.Kind() != "mongo" {
		t.Errorf("expected Kind() to return 'mongo', got '%s'", test.Kind())
	}
}

func TestVerifyExpectations(t *testing.T) {
	test := &TestSuiteTest{}

	tests := []struct {
		name        string
		results     []map[string]interface{}
		expect      *MongoExpectations
		wantErr     bool
		errContains string
	}{
		{
			name:    "exact document count - pass",
			results: []map[string]interface{}{{"id": 1}, {"id": 2}},
			expect: &MongoExpectations{
				DocumentCount: intPtr(2),
			},
			wantErr: false,
		},
		{
			name:    "exact document count - fail",
			results: []map[string]interface{}{{"id": 1}},
			expect: &MongoExpectations{
				DocumentCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected 2 documents, but got 1 documents",
		},
		{
			name:    "min document count - pass",
			results: []map[string]interface{}{{"id": 1}, {"id": 2}, {"id": 3}},
			expect: &MongoExpectations{
				MinDocumentCount: intPtr(2),
			},
			wantErr: false,
		},
		{
			name:    "min document count - fail",
			results: []map[string]interface{}{{"id": 1}},
			expect: &MongoExpectations{
				MinDocumentCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected at least 2 documents",
		},
		{
			name:    "max document count - pass",
			results: []map[string]interface{}{{"id": 1}},
			expect: &MongoExpectations{
				MaxDocumentCount: intPtr(5),
			},
			wantErr: false,
		},
		{
			name:    "max document count - fail",
			results: []map[string]interface{}{{"id": 1}, {"id": 2}, {"id": 3}},
			expect: &MongoExpectations{
				MaxDocumentCount: intPtr(2),
			},
			wantErr:     true,
			errContains: "expected at most 2 documents",
		},
		{
			name:    "no documents - pass",
			results: []map[string]interface{}{},
			expect: &MongoExpectations{
				NoDocuments: true,
			},
			wantErr: false,
		},
		{
			name:    "no documents - fail",
			results: []map[string]interface{}{{"id": 1}},
			expect: &MongoExpectations{
				NoDocuments: true,
			},
			wantErr:     true,
			errContains: "expected no documents, but got 1 documents",
		},
		{
			name: "field values - pass",
			results: []map[string]interface{}{
				{"count": int64(5), "total": 100.0},
			},
			expect: &MongoExpectations{
				FieldValues: map[string]interface{}{
					"count": 5,
					"total": 100.0,
				},
			},
			wantErr: false,
		},
		{
			name: "field values - fail missing field",
			results: []map[string]interface{}{
				{"count": 5},
			},
			expect: &MongoExpectations{
				FieldValues: map[string]interface{}{
					"total": 100,
				},
			},
			wantErr:     true,
			errContains: "field 'total' not found",
		},
		{
			name: "field values - requires single document",
			results: []map[string]interface{}{
				{"count": 5},
				{"count": 10},
			},
			expect: &MongoExpectations{
				FieldValues: map[string]interface{}{
					"count": 5,
				},
			},
			wantErr:     true,
			errContains: "field_values assertion requires exactly 1 document",
		},
		{
			name: "contains - pass",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
			},
			expect: &MongoExpectations{
				Contains: []map[string]interface{}{
					{"id": 1, "name": "Alice"},
				},
			},
			wantErr: false,
		},
		{
			name: "contains - fail",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expect: &MongoExpectations{
				Contains: []map[string]interface{}{
					{"id": 2, "name": "Bob"},
				},
			},
			wantErr:     true,
			errContains: "expected document not found",
		},
		{
			name: "not_contains - pass",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expect: &MongoExpectations{
				NotContains: []map[string]interface{}{
					{"id": 2, "name": "Bob"},
				},
			},
			wantErr: false,
		},
		{
			name: "not_contains - fail",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
			},
			expect: &MongoExpectations{
				NotContains: []map[string]interface{}{
					{"id": 2, "name": "Bob"},
				},
			},
			wantErr:     true,
			errContains: "found document that should not exist",
		},
		{
			name: "exact documents - pass",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expect: &MongoExpectations{
				Documents: []map[string]interface{}{
					{"id": 1, "name": "Alice"},
				},
			},
			wantErr: false,
		},
		{
			name: "exact documents - fail count mismatch",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expect: &MongoExpectations{
				Documents: []map[string]interface{}{
					{"id": 1, "name": "Alice"},
					{"id": 2, "name": "Bob"},
				},
			},
			wantErr:     true,
			errContains: "expected 2 documents, but got 1 documents",
		},
		{
			name: "exact documents - fail value mismatch",
			results: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expect: &MongoExpectations{
				Documents: []map[string]interface{}{
					{"id": 1, "name": "Bob"},
				},
			},
			wantErr:     true,
			errContains: "expected Bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test.Expect = tt.expect
			err := test.verifyExpectations(tt.results)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain '%s', got '%s'", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func contains(s, substr string) bool {
	return indexOfSubstring(s, substr) >= 0
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestInterpolateExpectationDocuments(t *testing.T) {
	test := &TestSuiteTest{}

	fixtures := []e2eframe.Fixture{
		&e2eframe.FixtureV1{
			FixtureName:  "user_id",
			FixtureValue: "123",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "user_name",
			FixtureValue: "Alice",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "count",
			FixtureValue: "5",
		},
	}

	tests := []struct {
		name     string
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name: "interpolate string values",
			input: []map[string]interface{}{
				{"id": "{{ user_id }}", "name": "{{ user_name }}"},
			},
			expected: []map[string]interface{}{
				{"id": "123", "name": "Alice"},
			},
		},
		{
			name: "interpolate numeric values",
			input: []map[string]interface{}{
				{"count": "{{ count }}"},
			},
			expected: []map[string]interface{}{
				{"count": "5"},
			},
		},
		{
			name: "no interpolation needed",
			input: []map[string]interface{}{
				{"id": "1", "name": "Bob"},
			},
			expected: []map[string]interface{}{
				{"id": "1", "name": "Bob"},
			},
		},
		{
			name:     "empty input",
			input:    []map[string]interface{}{},
			expected: []map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := test.interpolateExpectationDocuments(tt.input, fixtures)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d documents, got %d", len(tt.expected), len(result))
				return
			}
			for i, doc := range result {
				expectedDoc := tt.expected[i]
				for key, expectedVal := range expectedDoc {
					actualVal, exists := doc[key]
					if !exists {
						t.Errorf("document %d missing key '%s'", i, key)
						continue
					}
					if actualVal != expectedVal {
						t.Errorf("document %d key '%s': expected %v, got %v", i, key, expectedVal, actualVal)
					}
				}
			}
		})
	}
}

func TestInterpolateFieldValues(t *testing.T) {
	test := &TestSuiteTest{}

	fixtures := []e2eframe.Fixture{
		&e2eframe.FixtureV1{
			FixtureName:  "expected_count",
			FixtureValue: "42",
		},
		&e2eframe.FixtureV1{
			FixtureName:  "expected_total",
			FixtureValue: "100",
		},
	}

	input := map[string]interface{}{
		"count": "{{ expected_count }}",
		"total": "{{ expected_total }}",
		"name":  "static",
	}

	result := test.interpolateFieldValues(input, fixtures)

	if result["count"] != "42" {
		t.Errorf("expected count to be '42', got '%v'", result["count"])
	}
	if result["total"] != "100" {
		t.Errorf("expected total to be '100', got '%v'", result["total"])
	}
	if result["name"] != "static" {
		t.Errorf("expected name to be 'static', got '%v'", result["name"])
	}
}

func TestInterpolateExpectationDocumentsEmptyFixtures(t *testing.T) {
	test := &TestSuiteTest{}

	input := []map[string]interface{}{
		{"id": "{{ user_id }}", "name": "Alice"},
	}

	result := test.interpolateExpectationDocuments(input, []e2eframe.Fixture{})

	if len(result) != 1 {
		t.Errorf("expected 1 document, got %d", len(result))
		return
	}

	// Should remain unchanged
	if result[0]["id"] != "{{ user_id }}" {
		t.Errorf("expected id to remain '{{ user_id }}', got '%v'", result[0]["id"])
	}
}

func TestNormalizeValue(t *testing.T) {
	test := &TestSuiteTest{}

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{"nil value", nil, nil},
		{"string value", "hello", "hello"},
		{"int32 to int64", int32(42), int64(42)},
		{"uint64 to int64", uint64(100), int64(100)},
		{"float32 to float64", float32(3.14), float64(float32(3.14))},
		{"bool value", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := test.normalizeValue(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeValue(%v) = %v (type %T), want %v (type %T)",
					tt.input, result, result, tt.expected, tt.expected)
			}
		})
	}
}
