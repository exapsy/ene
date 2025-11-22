package mongotest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/yaml.v3"
)

const (
	Kind e2eframe.TestSuiteTestKind = "mongo"
)

type TestSuiteTest struct {
	TestName      string
	TestKind      string
	TestTarget    string             `yaml:"target"` // Optional: override suite-level target
	Debug         bool               `yaml:"debug"`  // Optional: enable debug output for this test
	Collection    string             `yaml:"collection"`
	Filter        interface{}        `yaml:"filter"`   // Can be string (JSON) or map (YAML)
	Pipeline      interface{}        `yaml:"pipeline"` // Can be string (JSON) or array (YAML)
	Expect        *MongoExpectations `yaml:"expect"`
	MongoEndpoint string
	testSuite     e2eframe.TestSuite
}

type MongoExpectations struct {
	DocumentCount    *int                     `yaml:"document_count"`
	Documents        []map[string]interface{} `yaml:"documents"`
	CollectionExists string                   `yaml:"collection_exists"`
	FieldValues      map[string]interface{}   `yaml:"field_values"`
	NoDocuments      bool                     `yaml:"no_documents"`
	MinDocumentCount *int                     `yaml:"min_document_count"`
	MaxDocumentCount *int                     `yaml:"max_document_count"`
	Contains         []map[string]interface{} `yaml:"contains"`
	NotContains      []map[string]interface{} `yaml:"not_contains"`
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

	var target e2eframe.Unit

	// Use per-test target if specified, otherwise use suite-level target
	if t.TestTarget != "" {
		// Find the target unit in the test suite
		units := testSuite.Units()
		found := false
		for _, unit := range units {
			if unit.Name() == t.TestTarget {
				target = unit
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("target unit '%s' not found in suite", t.TestTarget)
		}
	} else {
		target = testSuite.Target()
	}

	// Target is required
	if target == nil {
		return fmt.Errorf("target unit not found (hint: set 'target' at suite level or test level)")
	}

	// Get MongoDB connection details from the target unit
	dsn, err := target.Get("dsn")
	if err != nil {
		return fmt.Errorf("failed to get mongodb DSN from unit '%s': %w (hint: target must be a mongo unit)", target.Name(), err)
	}
	t.MongoEndpoint = dsn

	return nil
}

func (t *TestSuiteTest) Run(ctx context.Context, opts *e2eframe.TestSuiteTestRunOptions) (*e2eframe.TestResult, error) {
	startTime := time.Now()

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(t.MongoEndpoint)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Failed to connect to MongoDB: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}
	defer func() {
		if err := client.Disconnect(ctx); err != nil {
			fmt.Printf("Warning: failed to disconnect from MongoDB: %v\n", err)
		}
	}()

	// Set connection timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Failed to ping MongoDB: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}

	// Run expectations
	if err := t.runExpectations(ctx, client, opts); err != nil {
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

func (t *TestSuiteTest) runExpectations(ctx context.Context, client *mongo.Client, opts *e2eframe.TestSuiteTestRunOptions) error {
	if t.Expect == nil {
		return fmt.Errorf("no expectations provided")
	}

	// Interpolate fixtures in collection_exists if needed
	collectionExists := t.Expect.CollectionExists
	if opts != nil && len(opts.Fixtures) > 0 && collectionExists != "" {
		interpolationRegex := e2eframe.FixtureInterpolationRegex
		if interpolationRegex.MatchString(collectionExists) {
			collectionExists = e2eframe.InterpolateString(interpolationRegex, collectionExists, opts.Fixtures)
		}
	}

	// Check collection existence
	if collectionExists != "" {
		if err := t.verifyCollectionExists(ctx, client, collectionExists); err != nil {
			return err
		}
		// If only checking collection existence, return early
		if t.Collection == "" && t.Filter == nil && t.Pipeline == nil {
			return nil
		}
	}

	// If no collection provided but we have other expectations, error
	if t.Collection == "" {
		return fmt.Errorf("no collection provided for expectations")
	}

	// Interpolate fixtures in collection name if needed
	collection := t.Collection
	if opts != nil && len(opts.Fixtures) > 0 {
		interpolationRegex := e2eframe.FixtureInterpolationRegex
		if interpolationRegex.MatchString(collection) {
			collection = e2eframe.InterpolateString(interpolationRegex, collection, opts.Fixtures)
		}
	}

	// Parse and interpolate filter/pipeline
	var filter interface{}
	var pipeline interface{}
	var err error

	if t.Filter != nil {
		filter, err = t.parseAndInterpolateQuery(t.Filter, opts)
		if err != nil {
			return fmt.Errorf("failed to parse filter: %w", err)
		}
	}

	if t.Pipeline != nil {
		pipeline, err = t.parseAndInterpolateQuery(t.Pipeline, opts)
		if err != nil {
			return fmt.Errorf("failed to parse pipeline: %w", err)
		}
	}

	// Interpolate fixtures in expectations
	if opts != nil && len(opts.Fixtures) > 0 {
		// Interpolate documents expectations
		if len(t.Expect.Documents) > 0 {
			t.Expect.Documents = t.interpolateExpectationDocuments(t.Expect.Documents, opts.Fixtures)
		}
		// Interpolate field_values expectations
		if len(t.Expect.FieldValues) > 0 {
			t.Expect.FieldValues = t.interpolateFieldValues(t.Expect.FieldValues, opts.Fixtures)
		}
		// Interpolate contains expectations
		if len(t.Expect.Contains) > 0 {
			t.Expect.Contains = t.interpolateExpectationDocuments(t.Expect.Contains, opts.Fixtures)
		}
		// Interpolate not_contains expectations
		if len(t.Expect.NotContains) > 0 {
			t.Expect.NotContains = t.interpolateExpectationDocuments(t.Expect.NotContains, opts.Fixtures)
		}
	}

	// Log query in verbose or debug mode
	if opts != nil && (opts.Verbose || opts.Debug || t.Debug) {
		fmt.Printf("\n=== MongoDB Query ===\n")
		fmt.Printf("Collection: %s\n", collection)
		if filter != nil {
			filterJSON, _ := json.MarshalIndent(filter, "", "  ")
			fmt.Printf("Filter: %s\n", string(filterJSON))
		}
		if pipeline != nil {
			pipelineJSON, _ := json.MarshalIndent(pipeline, "", "  ")
			fmt.Printf("Pipeline: %s\n", string(pipelineJSON))
		}
		fmt.Printf("=====================\n\n")
	}

	// Get database from connection string
	db := client.Database("test") // Default database
	coll := db.Collection(collection)

	// Execute query
	var results []map[string]interface{}
	if pipeline != nil {
		// Use aggregation
		pipelineArray, ok := pipeline.([]interface{})
		if !ok {
			return fmt.Errorf("pipeline must be an array")
		}
		cursor, err := coll.Aggregate(ctx, pipelineArray)
		if err != nil {
			return fmt.Errorf("aggregation execution failed: %w", err)
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &results); err != nil {
			return fmt.Errorf("failed to decode aggregation results: %w", err)
		}
	} else {
		// Use find
		var findFilter interface{} = bson.D{}
		if filter != nil {
			findFilter = filter
		}

		cursor, err := coll.Find(ctx, findFilter)
		if err != nil {
			return fmt.Errorf("find execution failed: %w", err)
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &results); err != nil {
			return fmt.Errorf("failed to decode find results: %w", err)
		}
	}

	// Convert bson types to comparable types
	results = t.normalizeResults(results)

	// Log results in verbose or debug mode
	if opts != nil && (opts.Verbose || opts.Debug || t.Debug) {
		fmt.Printf("=== Query Results ===\n")
		fmt.Printf("Document count: %d\n", len(results))
		if len(results) > 0 {
			for i, doc := range results {
				fmt.Printf("Document %d: %v\n", i+1, doc)
			}
		}
		fmt.Printf("=====================\n\n")
	}

	// Verify expectations
	return t.verifyExpectations(results)
}

func (t *TestSuiteTest) parseAndInterpolateQuery(query interface{}, opts *e2eframe.TestSuiteTestRunOptions) (interface{}, error) {
	// Handle string (JSON format)
	if strQuery, ok := query.(string); ok {
		// Interpolate fixtures in string first
		if opts != nil && len(opts.Fixtures) > 0 {
			interpolationRegex := e2eframe.FixtureInterpolationRegex
			if interpolationRegex.MatchString(strQuery) {
				strQuery = e2eframe.InterpolateString(interpolationRegex, strQuery, opts.Fixtures)
			}
		}

		// Parse JSON string to bson
		var parsed interface{}
		if err := json.Unmarshal([]byte(strQuery), &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse JSON query: %w", err)
		}
		return parsed, nil
	}

	// Handle YAML structure (already parsed as map or array)
	// Interpolate fixtures in the structure
	if opts != nil && len(opts.Fixtures) > 0 {
		return t.interpolateExpectationValue(query, opts.Fixtures), nil
	}

	return query, nil
}

func (t *TestSuiteTest) verifyCollectionExists(ctx context.Context, client *mongo.Client, collectionName string) error {
	db := client.Database("test") // Default database
	names, err := db.ListCollectionNames(ctx, bson.D{{Key: "name", Value: collectionName}})
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if len(names) == 0 {
		return fmt.Errorf("collection '%s' does not exist", collectionName)
	}

	return nil
}

func (t *TestSuiteTest) verifyExpectations(results []map[string]interface{}) error {
	actualDocumentCount := len(results)

	// Check no_documents
	if t.Expect.NoDocuments {
		if actualDocumentCount > 0 {
			return fmt.Errorf("expected no documents, but got %d documents", actualDocumentCount)
		}
		return nil
	}

	// Check exact document count
	if t.Expect.DocumentCount != nil {
		if actualDocumentCount != *t.Expect.DocumentCount {
			return fmt.Errorf("expected %d documents, but got %d documents", *t.Expect.DocumentCount, actualDocumentCount)
		}
	}

	// Check min document count
	if t.Expect.MinDocumentCount != nil {
		if actualDocumentCount < *t.Expect.MinDocumentCount {
			return fmt.Errorf("expected at least %d documents, but got %d documents", *t.Expect.MinDocumentCount, actualDocumentCount)
		}
	}

	// Check max document count
	if t.Expect.MaxDocumentCount != nil {
		if actualDocumentCount > *t.Expect.MaxDocumentCount {
			return fmt.Errorf("expected at most %d documents, but got %d documents", *t.Expect.MaxDocumentCount, actualDocumentCount)
		}
	}

	// Check exact documents match
	if len(t.Expect.Documents) > 0 {
		if err := t.verifyExactDocuments(results, t.Expect.Documents); err != nil {
			return err
		}
	}

	// Check field values (for single document results)
	if len(t.Expect.FieldValues) > 0 {
		if actualDocumentCount == 0 {
			return fmt.Errorf("expected field values but got no documents")
		}
		if actualDocumentCount > 1 {
			return fmt.Errorf("field_values assertion requires exactly 1 document, but got %d documents", actualDocumentCount)
		}
		if err := t.verifyFieldValues(results[0], t.Expect.FieldValues); err != nil {
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

func (t *TestSuiteTest) normalizeResults(results []map[string]interface{}) []map[string]interface{} {
	normalized := make([]map[string]interface{}, len(results))
	for i, doc := range results {
		normalized[i] = t.normalizeDocument(doc)
	}
	return normalized
}

func (t *TestSuiteTest) normalizeDocument(doc map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range doc {
		result[key] = t.normalizeValue(value)
	}
	return result
}

func (t *TestSuiteTest) normalizeValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Map:
		m := make(map[string]interface{})
		iter := v.MapRange()
		for iter.Next() {
			key := fmt.Sprintf("%v", iter.Key().Interface())
			m[key] = t.normalizeValue(iter.Value().Interface())
		}
		return m
	case reflect.Slice, reflect.Array:
		arr := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			arr[i] = t.normalizeValue(v.Index(i).Interface())
		}
		return arr
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		return value
	}
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

	// Handle map interpolation (for document expectations)
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

// interpolateExpectationDocuments interpolates fixtures in document expectations
func (t *TestSuiteTest) interpolateExpectationDocuments(documents []map[string]interface{}, fixtures []e2eframe.Fixture) []map[string]interface{} {
	if len(fixtures) == 0 {
		return documents
	}

	result := make([]map[string]interface{}, len(documents))
	for i, doc := range documents {
		interpolatedDoc := make(map[string]interface{})
		for k, v := range doc {
			interpolatedDoc[k] = t.interpolateExpectationValue(v, fixtures)
		}
		result[i] = interpolatedDoc
	}
	return result
}

// interpolateFieldValues interpolates fixtures in field value expectations
func (t *TestSuiteTest) interpolateFieldValues(fieldVals map[string]interface{}, fixtures []e2eframe.Fixture) map[string]interface{} {
	if len(fixtures) == 0 {
		return fieldVals
	}

	result := make(map[string]interface{})
	for k, v := range fieldVals {
		result[k] = t.interpolateExpectationValue(v, fixtures)
	}
	return result
}

func (t *TestSuiteTest) verifyExactDocuments(actual, expected []map[string]interface{}) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("expected %d documents, but got %d documents", len(expected), len(actual))
	}

	for i, expectedDoc := range expected {
		actualDoc := actual[i]
		if err := t.compareDocument(actualDoc, expectedDoc, fmt.Sprintf("document %d", i+1)); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestSuiteTest) verifyFieldValues(doc map[string]interface{}, expected map[string]interface{}) error {
	for field, expectedValue := range expected {
		actualValue, exists := doc[field]
		if !exists {
			return fmt.Errorf("field '%s' not found in result", field)
		}

		if err := t.compareValue(actualValue, expectedValue, field); err != nil {
			return err
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyContains(results []map[string]interface{}, expected []map[string]interface{}) error {
	for _, expectedDoc := range expected {
		found := false
		for _, actualDoc := range results {
			if t.documentMatches(actualDoc, expectedDoc) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected document not found in results: %v", expectedDoc)
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyNotContains(results []map[string]interface{}, notExpected []map[string]interface{}) error {
	for _, notExpectedDoc := range notExpected {
		for _, actualDoc := range results {
			if t.documentMatches(actualDoc, notExpectedDoc) {
				return fmt.Errorf("found document that should not exist: %v", notExpectedDoc)
			}
		}
	}
	return nil
}

func (t *TestSuiteTest) documentMatches(actualDoc, expectedDoc map[string]interface{}) bool {
	// Check if all expected fields match
	for field, expectedValue := range expectedDoc {
		actualValue, exists := actualDoc[field]
		if !exists {
			return false
		}
		if !t.valuesEqual(actualValue, expectedValue) {
			return false
		}
	}
	return true
}

func (t *TestSuiteTest) compareDocument(actual, expected map[string]interface{}, docDesc string) error {
	for field, expectedValue := range expected {
		actualValue, exists := actual[field]
		if !exists {
			return fmt.Errorf("%s: field '%s' not found in result", docDesc, field)
		}

		if err := t.compareValue(actualValue, expectedValue, fmt.Sprintf("%s, field '%s'", docDesc, field)); err != nil {
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
		case "target":
			if err := value.Decode(&t.TestTarget); err != nil {
				return fmt.Errorf("failed to decode target: %w", err)
			}
		case "debug":
			if err := value.Decode(&t.Debug); err != nil {
				return fmt.Errorf("failed to decode debug: %w", err)
			}
		case "collection":
			if err := value.Decode(&t.Collection); err != nil {
				return fmt.Errorf("failed to decode collection: %w", err)
			}
		case "filter":
			if err := value.Decode(&t.Filter); err != nil {
				return fmt.Errorf("failed to decode filter: %w", err)
			}
		case "pipeline":
			if err := value.Decode(&t.Pipeline); err != nil {
				return fmt.Errorf("failed to decode pipeline: %w", err)
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
	hasExpectation := t.Expect.DocumentCount != nil ||
		t.Expect.MinDocumentCount != nil ||
		t.Expect.MaxDocumentCount != nil ||
		len(t.Expect.Documents) > 0 ||
		t.Expect.CollectionExists != "" ||
		len(t.Expect.FieldValues) > 0 ||
		t.Expect.NoDocuments ||
		len(t.Expect.Contains) > 0 ||
		len(t.Expect.NotContains) > 0

	if !hasExpectation {
		return fmt.Errorf("at least one expectation must be provided")
	}

	return nil
}
