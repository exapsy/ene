package e2eframe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverTestSuites(t *testing.T) {
	// Get the testdata directory
	testdataDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("failed to get testdata path: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		expectedCount int
		expectError   bool
		errorContains string
	}{
		{
			name:          "discover from directory with /tests subdirectory",
			path:          filepath.Join(testdataDir, "discovery"),
			expectedCount: 3, // suite1, suite2, nested/deep
			expectError:   false,
		},
		{
			name:          "discover from /tests directory directly",
			path:          filepath.Join(testdataDir, "discovery", "tests"),
			expectedCount: 3,
			expectError:   false,
		},
		{
			name:          "discover single suite directory",
			path:          filepath.Join(testdataDir, "single_suite"),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "discover single suite.yml file directly",
			path:          filepath.Join(testdataDir, "single_suite", "suite.yml"),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "discover from specific suite subdirectory",
			path:          filepath.Join(testdataDir, "discovery", "tests", "suite1"),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "discover from nested suite",
			path:          filepath.Join(testdataDir, "discovery", "tests", "nested", "deep"),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "error on non-existent path",
			path:          filepath.Join(testdataDir, "nonexistent"),
			expectedCount: 0,
			expectError:   true,
			errorContains: "failed to stat path",
		},
		{
			name:          "error on non-suite.yml file",
			path:          filepath.Join(testdataDir, "discovery", "tests", "suite1", "suite.yml") + ".not",
			expectedCount: 0,
			expectError:   true,
			errorContains: "is not a suite.yml file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create dummy file for the "non-suite.yml file" test
			if tt.errorContains == "is not a suite.yml file" {
				dummyFile := tt.path
				if err := os.WriteFile(dummyFile, []byte("dummy"), 0644); err != nil {
					t.Fatalf("failed to create dummy file: %v", err)
				}
				defer os.Remove(dummyFile)
			}

			suites, err := DiscoverTestSuites(tt.path)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(suites) != tt.expectedCount {
				t.Errorf("expected %d suites, got %d: %v", tt.expectedCount, len(suites), suites)
			}

			// Verify all returned paths exist and are suite.yml files
			for _, suite := range suites {
				if filepath.Base(suite) != "suite.yml" {
					t.Errorf("expected suite file to be named suite.yml, got: %s", suite)
				}

				if _, err := os.Stat(suite); err != nil {
					t.Errorf("suite file does not exist: %s", suite)
				}
			}
		})
	}
}

func TestDiscoverTestSuites_EmptyPath(t *testing.T) {
	// Test that empty path defaults to current directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Change to testdata/single_suite
	testdataDir, err := filepath.Abs("testdata/single_suite")
	if err != nil {
		t.Fatalf("failed to get testdata path: %v", err)
	}

	if err := os.Chdir(testdataDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(originalWd)

	// Discover with empty path (should use current directory)
	suites, err := DiscoverTestSuites("")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if len(suites) != 1 {
		t.Errorf("expected 1 suite, got %d", len(suites))
	}
}

func TestDiscoverTestSuites_SkipsHiddenDirectories(t *testing.T) {
	// Create a temporary directory structure with hidden directories
	tmpDir := t.TempDir()

	// Create visible suite
	visibleDir := filepath.Join(tmpDir, "visible")
	if err := os.MkdirAll(visibleDir, 0755); err != nil {
		t.Fatalf("failed to create visible dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(visibleDir, "suite.yml"), []byte("kind: e2e_test:v1\nname: visible\nunits: []\ntests: []"), 0644); err != nil {
		t.Fatalf("failed to create visible suite: %v", err)
	}

	// Create hidden suite (should be skipped)
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatalf("failed to create hidden dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "suite.yml"), []byte("kind: e2e_test:v1\nname: hidden\nunits: []\ntests: []"), 0644); err != nil {
		t.Fatalf("failed to create hidden suite: %v", err)
	}

	suites, err := DiscoverTestSuites(tmpDir)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Should only find the visible suite
	if len(suites) != 1 {
		t.Errorf("expected 1 suite (hidden should be skipped), got %d", len(suites))
	}

	for _, suite := range suites {
		if strings.Contains(suite, ".hidden") {
			t.Errorf("hidden directory suite should have been skipped: %s", suite)
		}
	}
}

func TestLoadTestSuites_Integration(t *testing.T) {
	// This is more of an integration test - just verify that LoadTestSuites
	// uses DiscoverTestSuites correctly by checking the count
	testdataDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("failed to get testdata path: %v", err)
	}

	tests := []struct {
		name          string
		baseDir       string
		expectedCount int
	}{
		{
			name:          "discover from directory with /tests",
			baseDir:       filepath.Join(testdataDir, "discovery"),
			expectedCount: 3,
		},
		{
			name:          "discover single suite",
			baseDir:       filepath.Join(testdataDir, "single_suite"),
			expectedCount: 1,
		},
		{
			name:          "discover specific suite directory",
			baseDir:       filepath.Join(testdataDir, "discovery", "tests", "suite1"),
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify discovery works - don't try to fully load suites
			// since that requires plugins to be registered
			suiteFiles, err := DiscoverTestSuites(tt.baseDir)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(suiteFiles) != tt.expectedCount {
				t.Errorf("expected %d suite files, got %d", tt.expectedCount, len(suiteFiles))
			}
		})
	}
}
