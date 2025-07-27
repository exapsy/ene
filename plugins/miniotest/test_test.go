package miniotest

import (
	"fmt"
	"testing"

	"github.com/exapsy/ene/e2eframe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMinioTest_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, test *TestSuiteTest)
		err   string
	}{
		{
			name: "simple files_exist verification",
			yaml: `
name: test-files-exist
kind: minio
verify_state:
  files_exist:
    - "uploads/user123/profile.jpg"
    - "documents/report.pdf"
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-files-exist", test.TestName)
				assert.Equal(t, "minio", test.TestKind)
				require.NotNil(t, test.VerifyState)
				assert.Len(t, test.VerifyState.FilesExist, 2)
				assert.Equal(t, "uploads/user123/profile.jpg", test.VerifyState.FilesExist[0])
				assert.Equal(t, "documents/report.pdf", test.VerifyState.FilesExist[1])
			},
		},
		{
			name: "bucket counts verification",
			yaml: `
name: test-bucket-counts
kind: minio
verify_state:
  bucket_counts:
    uploads: 2
    processed: 1
    archived: 0
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-bucket-counts", test.TestName)
				require.NotNil(t, test.VerifyState)
				require.NotNil(t, test.VerifyState.BucketCounts)
				assert.Equal(t, 2, test.VerifyState.BucketCounts["uploads"])
				assert.Equal(t, 1, test.VerifyState.BucketCounts["processed"])
				assert.Equal(t, 0, test.VerifyState.BucketCounts["archived"])
			},
		},
		{
			name: "required state with file constraints",
			yaml: `
name: test-required-state
kind: minio
verify_state:
  required:
    buckets:
      uploads:
        - path: "user123/profile.jpg"
          min_size: "1KB"
          max_size: "10MB"
          max_age: "5m"
          content_type: "image/jpeg"
        - path: "user123/document.pdf"
          min_size: "50KB"
          content_type: "application/pdf"
    files:
      - path: "backups/daily.zip"
        min_size: "100MB"
        max_age: "24h"
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-required-state", test.TestName)
				require.NotNil(t, test.VerifyState)
				require.NotNil(t, test.VerifyState.Required)

				// Check buckets
				require.Contains(t, test.VerifyState.Required.Buckets, "uploads")
				uploadsFiles := test.VerifyState.Required.Buckets["uploads"]
				assert.Len(t, uploadsFiles, 2)

				// Check first file
				assert.Equal(t, "user123/profile.jpg", uploadsFiles[0].Path)
				assert.Equal(t, "1KB", uploadsFiles[0].MinSize)
				assert.Equal(t, "10MB", uploadsFiles[0].MaxSize)
				assert.Equal(t, "5m", uploadsFiles[0].MaxAge)
				assert.Equal(t, "image/jpeg", uploadsFiles[0].ContentType)

				// Check direct files
				assert.Len(t, test.VerifyState.Required.Files, 1)
				assert.Equal(t, "backups/daily.zip", test.VerifyState.Required.Files[0].Path)
				assert.Equal(t, "100MB", test.VerifyState.Required.Files[0].MinSize)
				assert.Equal(t, "24h", test.VerifyState.Required.Files[0].MaxAge)
			},
		},
		{
			name: "forbidden state",
			yaml: `
name: test-forbidden-state
kind: minio
verify_state:
  forbidden:
    buckets:
      uploads:
        - "temp_*"
        - "*.tmp"
      cache:
        - "*"
    files:
      - "logs/debug.log"
      - "temp/cache.dat"
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-forbidden-state", test.TestName)
				require.NotNil(t, test.VerifyState)
				require.NotNil(t, test.VerifyState.Forbidden)

				// Check forbidden buckets
				require.Contains(t, test.VerifyState.Forbidden.Buckets, "uploads")
				uploadsPatterns := test.VerifyState.Forbidden.Buckets["uploads"]
				assert.Len(t, uploadsPatterns, 2)
				assert.Equal(t, "temp_*", uploadsPatterns[0])
				assert.Equal(t, "*.tmp", uploadsPatterns[1])

				// Check forbidden files
				assert.Len(t, test.VerifyState.Forbidden.Files, 2)
				assert.Equal(t, "logs/debug.log", test.VerifyState.Forbidden.Files[0])
				assert.Equal(t, "temp/cache.dat", test.VerifyState.Forbidden.Files[1])
			},
		},
		{
			name: "constraints",
			yaml: `
name: test-constraints
kind: minio
verify_state:
  constraints:
    - bucket: uploads
      file_count: 2
      max_total_size: "100MB"
      min_total_size: "1MB"
    - bucket: processed
      file_count: ">= 1"
    - total_buckets: "<= 5"
    - empty_buckets: "allowed"
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-constraints", test.TestName)
				require.NotNil(t, test.VerifyState)
				assert.Len(t, test.VerifyState.Constraints, 4)

				// Check first constraint
				constraint1 := test.VerifyState.Constraints[0]
				assert.Equal(t, "uploads", constraint1.Bucket)
				assert.Equal(t, 2, constraint1.FileCount)
				assert.Equal(t, "100MB", constraint1.MaxTotalSize)
				assert.Equal(t, "1MB", constraint1.MinTotalSize)

				// Check second constraint
				constraint2 := test.VerifyState.Constraints[1]
				assert.Equal(t, "processed", constraint2.Bucket)
				assert.Equal(t, ">= 1", constraint2.FileCount)

				// Check global constraints
				constraint3 := test.VerifyState.Constraints[2]
				assert.Equal(t, "", constraint3.Bucket)
				assert.Equal(t, "<= 5", constraint3.TotalBuckets)
			},
		},
		{
			name: "complete state verification",
			yaml: `
name: test-complete-state
kind: minio
verify_state:
  files_exist:
    - "uploads/user123/profile.jpg"
  bucket_counts:
    uploads: 1
  required:
    buckets:
      uploads:
        - path: "user123/profile.jpg"
          min_size: "1KB"
  forbidden:
    buckets:
      uploads:
        - "*.tmp"
  constraints:
    - bucket: uploads
      file_count: 1
`,
			check: func(t *testing.T, test *TestSuiteTest) {
				assert.Equal(t, "test-complete-state", test.TestName)
				require.NotNil(t, test.VerifyState)

				// Check all sections are present
				assert.Len(t, test.VerifyState.FilesExist, 1)
				assert.Len(t, test.VerifyState.BucketCounts, 1)
				require.NotNil(t, test.VerifyState.Required)
				require.NotNil(t, test.VerifyState.Forbidden)
				assert.Len(t, test.VerifyState.Constraints, 1)
			},
		},
		{
			name: "missing name",
			yaml: `
kind: minio
verify_state:
  files_exist:
    - "uploads/test.txt"
`,
			err: "test name is required",
		},
		{
			name: "missing verify_state",
			yaml: `
name: test-missing-state
kind: minio
`,
			err: "verify_state is required",
		},
		{
			name: "invalid yaml structure",
			yaml: `
name: test-invalid
kind: minio
verify_state: "invalid"
`,
			err: "failed to decode verify_state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yaml), &node)
			require.NoError(t, err)

			// Get the document node (first child of root)
			require.Len(t, node.Content, 1)

			test := &TestSuiteTest{}
			err = test.UnmarshalYAML(node.Content[0])

			if tt.err != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.err)
			} else {
				assert.NoError(t, err)
				if tt.check != nil {
					tt.check(t, test)
				}
			}
		})
	}
}

func TestMinioTest_RegistrationAndUnmarshalling(t *testing.T) {
	// Test that the test is properly registered
	assert.True(t, e2eframe.TestSuiteTestKindExists("minio"))

	yamlContent := `
name: test-minio-registration
kind: minio
verify_state:
  files_exist:
    - "uploads/test.txt"
  bucket_counts:
    uploads: 1
  constraints:
    - bucket: uploads
      file_count: ">= 1"
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	// Get the document node (first child of root)
	require.Len(t, node.Content, 1)

	testInterface, err := e2eframe.UnmarshallTestSuiteTest("minio", node.Content[0])
	require.NoError(t, err)

	minioTest, ok := testInterface.(*TestSuiteTest)
	require.True(t, ok)
	assert.Equal(t, "test-minio-registration", minioTest.Name())
	assert.Equal(t, "minio", minioTest.Kind())
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		err      bool
	}{
		{"100", 100, false},
		{"100B", 100, false},
		{"1KB", 1024, false},
		{"2MB", 2 * 1024 * 1024, false},
		{"3GB", 3 * 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"  1KB  ", 1024, false}, // test trimming
		{"1kb", 1024, false},     // test case insensitive
		{"invalid", 0, true},
		{"1XB", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseSize(tt.input)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		text     string
		expected bool
	}{
		{"*.txt", "file.txt", true},
		{"*.txt", "file.pdf", false},
		{"temp_*", "temp_123", true},
		{"temp_*", "permanent_123", false},
		{"user*/profile.*", "user123/profile.jpg", true},
		{"user*/profile.*", "admin/profile.jpg", false},
		{"exact_match", "exact_match", true},
		{"exact_match", "not_exact", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.text), func(t *testing.T) {
			result, err := matchPattern(tt.pattern, tt.text)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVerifyCountConstraint(t *testing.T) {
	tests := []struct {
		actual      int
		expected    string
		description string
		err         bool
	}{
		{5, "5", "files", false},
		{3, "5", "files", true},
		{5, "<= 5", "files", false},
		{6, "<= 5", "files", true},
		{5, ">= 5", "files", false},
		{4, ">= 5", "files", true},
		{5, "< 6", "files", false},
		{6, "< 6", "files", true},
		{6, "> 5", "files", false},
		{5, "> 5", "files", true},
		{5, "≤ 5", "files", false}, // Unicode operators
		{5, "≥ 5", "files", false},
		{5, "invalid", "files", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", tt.actual, tt.expected), func(t *testing.T) {
			err := verifyCountConstraint(tt.actual, tt.expected, tt.description)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
