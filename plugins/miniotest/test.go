package miniotest

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gopkg.in/yaml.v3"
)

const (
	Kind e2eframe.TestSuiteTestKind = "minio"
)

type TestSuiteTest struct {
	TestName       string
	TestKind       string
	VerifyState    *StateVerification `yaml:"verify_state"`
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	testSuite      e2eframe.TestSuite
}

type StateVerification struct {
	Required     *RequiredState    `yaml:"required"`
	Forbidden    *ForbiddenState   `yaml:"forbidden"`
	Constraints  []StateConstraint `yaml:"constraints"`
	FilesExist   []string          `yaml:"files_exist"`
	BucketCounts map[string]int    `yaml:"bucket_counts"`
}

type RequiredState struct {
	Buckets map[string][]RequiredFile `yaml:"buckets"`
	Files   []RequiredFile            `yaml:"files"`
}

type ForbiddenState struct {
	Buckets map[string][]string `yaml:"buckets"`
	Files   []string            `yaml:"files"`
}

type RequiredFile struct {
	Path        string `yaml:"path"`
	MinSize     string `yaml:"min_size"`
	MaxSize     string `yaml:"max_size"`
	MaxAge      string `yaml:"max_age"`
	ContentType string `yaml:"content_type"`
	Pattern     string `yaml:"pattern"`
}

type StateConstraint struct {
	Bucket       string `yaml:"bucket"`
	FileCount    any    `yaml:"file_count"`
	MaxTotalSize string `yaml:"max_total_size"`
	MinTotalSize string `yaml:"min_total_size"`
	TotalBuckets string `yaml:"total_buckets"`
	EmptyBuckets string `yaml:"empty_buckets"`
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

	// Find the target unit and get Minio connection details
	target := testSuite.Target()
	if target == nil {
		return fmt.Errorf("target unit not found")
	}

	// Get Minio connection details from the target unit
	endpoint, err := target.Get("endpoint")
	if err != nil {
		return fmt.Errorf("failed to get minio endpoint: %w", err)
	}
	t.MinioEndpoint = endpoint

	accessKey, err := target.Get("access_key")
	if err != nil {
		return fmt.Errorf("failed to get minio access key: %w", err)
	}
	t.MinioAccessKey = accessKey

	secretKey, err := target.Get("secret_key")
	if err != nil {
		return fmt.Errorf("failed to get minio secret key: %w", err)
	}
	t.MinioSecretKey = secretKey

	return nil
}

func (t *TestSuiteTest) Run(ctx context.Context, opts *e2eframe.TestSuiteTestRunOptions) (*e2eframe.TestResult, error) {
	startTime := time.Now()

	// Create Minio client
	client, err := minio.New(t.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(t.MinioAccessKey, t.MinioSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("Failed to create Minio client: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}

	// Verify state
	if err := t.verifyState(ctx, client); err != nil {
		return &e2eframe.TestResult{
			TestName: t.TestName,
			Passed:   false,
			Message:  fmt.Sprintf("State verification failed: %v", err),
			Err:      err,
			Duration: time.Since(startTime),
		}, nil
	}

	return &e2eframe.TestResult{
		TestName: t.TestName,
		Passed:   true,
		Message:  "State verification passed",
		Duration: time.Since(startTime),
	}, nil
}

func (t *TestSuiteTest) verifyState(ctx context.Context, client *minio.Client) error {
	if t.VerifyState == nil {
		return fmt.Errorf("no state verification configuration provided")
	}

	// Verify simple files_exist
	if err := t.verifyFilesExist(ctx, client); err != nil {
		return err
	}

	// Verify bucket counts
	if err := t.verifyBucketCounts(ctx, client); err != nil {
		return err
	}

	// Verify required state
	if err := t.verifyRequiredState(ctx, client); err != nil {
		return err
	}

	// Verify forbidden state
	if err := t.verifyForbiddenState(ctx, client); err != nil {
		return err
	}

	// Verify constraints
	if err := t.verifyConstraints(ctx, client); err != nil {
		return err
	}

	return nil
}

func (t *TestSuiteTest) verifyFilesExist(ctx context.Context, client *minio.Client) error {
	for _, filePath := range t.VerifyState.FilesExist {
		parts := strings.SplitN(filePath, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid file path format: %s (expected bucket/object)", filePath)
		}

		bucket, object := parts[0], parts[1]
		_, err := client.StatObject(ctx, bucket, object, minio.StatObjectOptions{})
		if err != nil {
			return fmt.Errorf("file %s does not exist: %w", filePath, err)
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyBucketCounts(ctx context.Context, client *minio.Client) error {
	for bucketName, expectedCount := range t.VerifyState.BucketCounts {
		objects := []minio.ObjectInfo{}
		objectCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{})
		for object := range objectCh {
			if object.Err != nil {
				return fmt.Errorf("error listing objects in bucket %s: %w", bucketName, object.Err)
			}
			objects = append(objects, object)
		}

		if len(objects) != expectedCount {
			return fmt.Errorf("bucket %s has %d files, expected %d", bucketName, len(objects), expectedCount)
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyRequiredState(ctx context.Context, client *minio.Client) error {
	if t.VerifyState.Required == nil {
		return nil
	}

	// Verify required buckets and their files
	for bucketName, requiredFiles := range t.VerifyState.Required.Buckets {
		// Check if bucket exists
		exists, err := client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("error checking bucket %s existence: %w", bucketName, err)
		}
		if !exists {
			return fmt.Errorf("required bucket %s does not exist", bucketName)
		}

		// Verify required files in this bucket
		for _, requiredFile := range requiredFiles {
			if err := t.verifyRequiredFile(ctx, client, bucketName, requiredFile); err != nil {
				return err
			}
		}
	}

	// Verify required files (direct format)
	for _, requiredFile := range t.VerifyState.Required.Files {
		parts := strings.SplitN(requiredFile.Path, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid file path format: %s", requiredFile.Path)
		}
		if err := t.verifyRequiredFile(ctx, client, parts[0], requiredFile); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestSuiteTest) verifyRequiredFile(ctx context.Context, client *minio.Client, bucketName string, file RequiredFile) error {
	objectName := file.Path
	if strings.Contains(file.Path, "/") {
		parts := strings.SplitN(file.Path, "/", 2)
		objectName = parts[1]
	}

	// Check if file exists
	stat, err := client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("required file %s/%s does not exist: %w", bucketName, objectName, err)
	}

	// Verify file constraints
	if file.MinSize != "" {
		minSize, err := parseSize(file.MinSize)
		if err != nil {
			return fmt.Errorf("invalid min_size format: %w", err)
		}
		if stat.Size < minSize {
			return fmt.Errorf("file %s/%s size %d is less than minimum %d", bucketName, objectName, stat.Size, minSize)
		}
	}

	if file.MaxSize != "" {
		maxSize, err := parseSize(file.MaxSize)
		if err != nil {
			return fmt.Errorf("invalid max_size format: %w", err)
		}
		if stat.Size > maxSize {
			return fmt.Errorf("file %s/%s size %d exceeds maximum %d", bucketName, objectName, stat.Size, maxSize)
		}
	}

	if file.MaxAge != "" {
		maxAge, err := time.ParseDuration(file.MaxAge)
		if err != nil {
			return fmt.Errorf("invalid max_age format: %w", err)
		}
		if time.Since(stat.LastModified) > maxAge {
			return fmt.Errorf("file %s/%s is older than maximum age %s", bucketName, objectName, file.MaxAge)
		}
	}

	if file.ContentType != "" && stat.ContentType != file.ContentType {
		return fmt.Errorf("file %s/%s has content type %s, expected %s", bucketName, objectName, stat.ContentType, file.ContentType)
	}

	return nil
}

func (t *TestSuiteTest) verifyForbiddenState(ctx context.Context, client *minio.Client) error {
	if t.VerifyState.Forbidden == nil {
		return nil
	}

	// Verify forbidden files in buckets
	for bucketName, forbiddenPatterns := range t.VerifyState.Forbidden.Buckets {
		objects := []minio.ObjectInfo{}
		objectCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{})
		for object := range objectCh {
			if object.Err != nil {
				return fmt.Errorf("error listing objects in bucket %s: %w", bucketName, object.Err)
			}
			objects = append(objects, object)
		}

		for _, pattern := range forbiddenPatterns {
			for _, object := range objects {
				matched, err := matchPattern(pattern, object.Key)
				if err != nil {
					return fmt.Errorf("error matching pattern %s: %w", pattern, err)
				}
				if matched {
					return fmt.Errorf("forbidden file pattern %s found in bucket %s: %s", pattern, bucketName, object.Key)
				}
			}
		}
	}

	// Verify forbidden files (direct format)
	for _, forbiddenPath := range t.VerifyState.Forbidden.Files {
		parts := strings.SplitN(forbiddenPath, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid forbidden file path format: %s", forbiddenPath)
		}

		bucket, object := parts[0], parts[1]
		_, err := client.StatObject(ctx, bucket, object, minio.StatObjectOptions{})
		if err == nil {
			return fmt.Errorf("forbidden file %s exists", forbiddenPath)
		}
		// File not existing is what we want for forbidden files
	}

	return nil
}

func (t *TestSuiteTest) verifyConstraints(ctx context.Context, client *minio.Client) error {
	for _, constraint := range t.VerifyState.Constraints {
		if err := t.verifyConstraint(ctx, client, constraint); err != nil {
			return err
		}
	}
	return nil
}

func (t *TestSuiteTest) verifyConstraint(ctx context.Context, client *minio.Client, constraint StateConstraint) error {
	// Bucket-specific constraints
	if constraint.Bucket != "" {
		return t.verifyBucketConstraint(ctx, client, constraint)
	}

	// Global constraints
	if constraint.TotalBuckets != "" {
		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			return fmt.Errorf("error listing buckets: %w", err)
		}

		if err := verifyCountConstraint(len(buckets), constraint.TotalBuckets, "total buckets"); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestSuiteTest) verifyBucketConstraint(ctx context.Context, client *minio.Client, constraint StateConstraint) error {
	// List objects in bucket
	objects := []minio.ObjectInfo{}
	objectCh := client.ListObjects(ctx, constraint.Bucket, minio.ListObjectsOptions{})
	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("error listing objects in bucket %s: %w", constraint.Bucket, object.Err)
		}
		objects = append(objects, object)
	}

	// File count constraint
	if constraint.FileCount != nil {
		var expectedCount string
		switch v := constraint.FileCount.(type) {
		case int:
			expectedCount = strconv.Itoa(v)
		case string:
			expectedCount = v
		default:
			return fmt.Errorf("invalid file_count type: %T", constraint.FileCount)
		}

		if err := verifyCountConstraint(len(objects), expectedCount, fmt.Sprintf("files in bucket %s", constraint.Bucket)); err != nil {
			return err
		}
	}

	// Total size constraints
	var totalSize int64
	for _, obj := range objects {
		totalSize += obj.Size
	}

	if constraint.MaxTotalSize != "" {
		maxSize, err := parseSize(constraint.MaxTotalSize)
		if err != nil {
			return fmt.Errorf("invalid max_total_size format: %w", err)
		}
		if totalSize > maxSize {
			return fmt.Errorf("bucket %s total size %d exceeds maximum %d", constraint.Bucket, totalSize, maxSize)
		}
	}

	if constraint.MinTotalSize != "" {
		minSize, err := parseSize(constraint.MinTotalSize)
		if err != nil {
			return fmt.Errorf("invalid min_total_size format: %w", err)
		}
		if totalSize < minSize {
			return fmt.Errorf("bucket %s total size %d is less than minimum %d", constraint.Bucket, totalSize, minSize)
		}
	}

	return nil
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
		case "verify_state":
			if err := value.Decode(&t.VerifyState); err != nil {
				return fmt.Errorf("failed to decode verify_state: %w", err)
			}
		default:
			return fmt.Errorf("unknown field: %s", key.Value)
		}
	}

	if t.TestName == "" {
		return fmt.Errorf("test name is required")
	}

	if t.TestKind == "" {
		t.TestKind = string(Kind)
	}

	if t.VerifyState == nil {
		return fmt.Errorf("verify_state is required")
	}

	return nil
}

// Helper functions

func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	// Extract number and unit
	re := regexp.MustCompile(`^(\d+)([KMGT]?B?)$`)
	matches := re.FindStringSubmatch(sizeStr)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid size format: %s", sizeStr)
	}

	size, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	switch unit {
	case "", "B":
		return size, nil
	case "KB":
		return size * 1024, nil
	case "MB":
		return size * 1024 * 1024, nil
	case "GB":
		return size * 1024 * 1024 * 1024, nil
	case "TB":
		return size * 1024 * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unknown size unit: %s", unit)
	}
}

func matchPattern(pattern, text string) (bool, error) {
	// Convert glob-style pattern to regex
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = "^" + pattern + "$"

	matched, err := regexp.MatchString(pattern, text)
	return matched, err
}

func verifyCountConstraint(actual int, expected string, description string) error {
	expected = strings.TrimSpace(expected)

	// Handle simple number
	if num, err := strconv.Atoi(expected); err == nil {
		if actual != num {
			return fmt.Errorf("%s count is %d, expected %d", description, actual, num)
		}
		return nil
	}

	// Handle operators
	if strings.HasPrefix(expected, "≤") || strings.HasPrefix(expected, "<=") {
		numStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(expected, "≤"), "<="))
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Errorf("invalid constraint format: %s", expected)
		}
		if actual > num {
			return fmt.Errorf("%s count is %d, expected ≤ %d", description, actual, num)
		}
	} else if strings.HasPrefix(expected, "≥") || strings.HasPrefix(expected, ">=") {
		numStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(expected, "≥"), ">="))
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Errorf("invalid constraint format: %s", expected)
		}
		if actual < num {
			return fmt.Errorf("%s count is %d, expected ≥ %d", description, actual, num)
		}
	} else if strings.HasPrefix(expected, "<") {
		numStr := strings.TrimSpace(strings.TrimPrefix(expected, "<"))
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Errorf("invalid constraint format: %s", expected)
		}
		if actual >= num {
			return fmt.Errorf("%s count is %d, expected < %d", description, actual, num)
		}
	} else if strings.HasPrefix(expected, ">") {
		numStr := strings.TrimSpace(strings.TrimPrefix(expected, ">"))
		num, err := strconv.Atoi(numStr)
		if err != nil {
			return fmt.Errorf("invalid constraint format: %s", expected)
		}
		if actual <= num {
			return fmt.Errorf("%s count is %d, expected > %d", description, actual, num)
		}
	} else {
		return fmt.Errorf("unsupported constraint format: %s", expected)
	}

	return nil
}
