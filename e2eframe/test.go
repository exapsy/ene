package e2eframe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"gopkg.in/yaml.v3"
)

type InterpolationError struct {
	err error
}

func (e *InterpolationError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("interpolation error: %v", e.err)
	}
	return "interpolation error"
}

func (e *InterpolationError) Unwrap() error {
	return e.err
}

type TestSuiteTestKindNotFoundErr struct {
	Kind TestSuiteTestKind
}

func (e TestSuiteTestKindNotFoundErr) Error() string {
	return fmt.Sprintf("subtest kind not found: %s", e.Kind)
}

type TestSuiteTestKind string

func (t TestSuiteTestKind) String() string {
	return string(t)
}

func (t TestSuiteTestKind) IsValid() bool {
	return TestSuiteTestKindExists(t)
}

func TestSuiteTestKindExists(kind TestSuiteTestKind) bool {
	_, ok := testSuiteTestUnmarshalers[kind]

	return ok
}

var testSuiteTestUnmarshalers = make(
	map[TestSuiteTestKind]func(node *yaml.Node) (TestSuiteTest, error),
)

func RegisterTestSuiteTestUnmarshaler(
	kind TestSuiteTestKind,
	factory func(node *yaml.Node) (TestSuiteTest, error),
) {
	if _, ok := testSuiteTestUnmarshalers[kind]; ok {
		panic("test suite test already registered")
	}

	testSuiteTestUnmarshalers[kind] = factory
}

func UnmarshallTestSuiteTest(kind TestSuiteTestKind, node *yaml.Node) (TestSuiteTest, error) {
	factory, ok := testSuiteTestUnmarshalers[kind]
	if !ok {
		return nil, &TestSuiteTestKindNotFoundErr{Kind: kind}
	}

	return factory(node)
}

type TestResult struct {
	SuiteName string // Add this field
	TestName  string
	Passed    bool
	Message   string
	Err       error
	Duration  time.Duration
}

func (t *TestResult) Unwrap() error {
	return t.Err
}

func (t *TestResult) MessageOrErr() string {
	if t.Message != "" {
		return t.Message
	}

	if t.Err != nil {
		return t.Err.Error()
	}

	// This should never happen if tests are properly implemented
	return fmt.Sprintf("no error message available for test '%s'", t.TestName)
}

func (t TestResult) Error() string {
	if t.Err != nil {
		return t.Err.Error()
	}

	return t.Message
}

type TestSuiteTestRunOptions struct {
	// Verbose enables verbose output
	Verbose bool
	// Fixtures contains fixtures that can be used in the test
	Fixtures []Fixture
	// Relative path to the test file, used for error reporting
	RelativePath string
}

type TestSuiteTest interface {
	// Name returns the name of the test.
	Name() string
	// Kind returns the kind of the test.
	Kind() string
	// Run executes the test.
	Run(ctx context.Context, opts *TestSuiteTestRunOptions) (*TestResult, error)
	// UnmarshalYAML unmarshals the test from YAML.
	UnmarshalYAML(node *yaml.Node) error
	// Initialize initializes the test with the parent test suite
	Initialize(testSuite TestSuite) error
}

// RunTestOptions contains options for running a test suite

type RunTestOptions struct {
	Debug bool // Enable debug mode
	// FilterFunc filters test execution by suite name and test name
	FilterFunc func(suiteName, testName string) bool
	// Verbose enables verbose output
	Verbose      bool
	CleanupCache bool // Cleanup old cached Docker images to prevent bloat
	EventSink    chan<- Event
	MaxRetries   int    // Number of retries for failed tests
	RetryDelay   string // Delay between retries (e.g. "2s")
	BaseDir      string // Base directory for the test suite, used for relative paths

	// Performance optimizations
	CacheImages bool // Enable image caching for faster builds
}

type TestSuite interface {
	// Name returns the name of the test suite.
	Name() string
	// Units returns the units in the test suite.
	Units() []Unit
	// Target returns the target unit of the test suite.
	Target() Unit
	// Tests returns the tests in the suite.
	Tests() []TestSuiteTest
	// Run executes the test suite.
	Run(ctx context.Context, opts *RunTestOptions) error
}

type Fixture interface {
	// Name returns the name of the fixture.
	Name() string
	// Value returns the value of the fixture.
	Value() []byte
}

type FixtureV1 struct {
	FixtureName string `yaml:"name"`
	// The hard value of the fixture
	FixtureValue string `yaml:"value,omitempty"`
	// FixtureFile is an optional file path for the fixture value
	FixtureFile string `yaml:"file,omitempty"`
	// RelativePath is the relative path to the fixture file
	RelativePath string `yaml:"relative_path,omitempty"`
}

func (f *FixtureV1) Name() string {
	return f.FixtureName
}

func (f *FixtureV1) Value() []byte {
	// Return the fixture value if it is already set
	// Could be the cached value from a previous call
	if f.FixtureValue != "" {
		return []byte(f.FixtureValue)
	}

	// If the fixture file is set, read the file and set the fixture value
	if f.FixtureFile != "" {
		pathToFixture := path.Join(f.RelativePath, f.FixtureFile)

		data, err := os.ReadFile(pathToFixture)
		if err != nil {
			fmt.Printf("Error reading fixture file %s: %v\n", f.FixtureFile, err)

			return nil
		}

		f.FixtureValue = string(data)

		return data
	}

	return nil
}

type TestSuiteV1 struct {
	TestName string
	Fixtures []Fixture
	// TestBeforeAll is a script that runs before all tests
	TestBeforeAll string `yaml:"test_before_all,omitempty"`
	// TestAfterAll is a script that runs after all tests
	TestAfterAll string `yaml:"test_after_all,omitempty"`
	// TestBeforeEach is a script that runs before each test
	TestBeforeEach string `yaml:"test_before_each,omitempty"`
	// TestAfterEach is a script that runs after each test
	TestAfterEach  string `yaml:"test_after_each,omitempty"`
	TestKind       ConfigKind
	TestUnits      []Unit
	TestTarget     Unit
	TestSuiteTests []TestSuiteTest
	RelativePath   string // Relative path to the test suite file
	WorkingDir     string // Working directory for the test suite, used for relative paths
}

// NewTestSuiteV1 creates a new test suite with the given name, kind, units, target, and tests.
// TODO: Actually use this ... currently exists only to match the interface, do not remove, provides QA.
func NewTestSuiteV1(
	name string,
	kind ConfigKind,
	units []Unit,
	target Unit,
	tests []TestSuiteTest,
) TestSuite {
	return &TestSuiteV1{
		TestName:       name,
		TestKind:       kind,
		TestUnits:      units,
		TestTarget:     target,
		TestSuiteTests: tests,
	}
}

func (t *TestSuiteV1) Name() string {
	return t.TestName
}

func (t *TestSuiteV1) Units() []Unit {
	units := make([]Unit, len(t.TestUnits))
	for i, unit := range t.TestUnits {
		units[i] = unit
	}

	return units
}

func (t *TestSuiteV1) Target() Unit {
	return t.TestTarget
}

func (t *TestSuiteV1) Tests() []TestSuiteTest {
	return t.TestSuiteTests
}

func (t *TestSuiteV1) runTest(
	ctx context.Context,
	test TestSuiteTest,
	opts *RunTestOptions,
) (*TestResult, error) {
	if err := test.Initialize(t); err != nil {
		return nil, fmt.Errorf("initialize test: %w", err)
	}

	t.sendEvent(
		opts.EventSink,
		EventTestStarted,
		fmt.Sprintf("Running test %s in suite %s", test.Name(), t.Name()),
	)

	// Measure test execution time
	startTime := time.Now()
	result, err := test.Run(ctx, &TestSuiteTestRunOptions{
		Verbose:      opts.Verbose,
		Fixtures:     t.Fixtures,
		RelativePath: t.RelativePath,
	})
	duration := time.Since(startTime)

	if err != nil {
		return nil, err
	}

	// Add duration to result
	if result != nil {
		result.Duration = duration
	}

	return result, nil
}

func (t *TestSuiteV1) runScript(ctx context.Context, script string, opts *RunTestOptions) error {
	if script == "" {
		return nil
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptExecuting,
		fmt.Sprintf("Executing script: %s", script),
	)

	cmdStrs := strings.Split(script, " ")
	cmdStr, args := cmdStrs[0], cmdStrs[1:]

	cmd, err := exec.CommandContext(ctx, cmdStr, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("run script: %w: %s", err, string(cmd))
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptCompleted,
		fmt.Sprintf("Script executed successfully: %s", script),
	)

	return nil
}

func (t *TestSuiteV1) runBeforeAll(ctx context.Context, opts *RunTestOptions) error {
	if t.TestBeforeAll == "" {
		return nil
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptExecuting,
		"Running beforeAll setup...",
	)

	if err := t.runScript(ctx, t.TestBeforeAll, opts); err != nil {
		return fmt.Errorf("run before all tests script: %w", err)
	}

	return nil
}

func (t *TestSuiteV1) runAfterAll(ctx context.Context, opts *RunTestOptions) error {
	if t.TestAfterAll == "" {
		return nil
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptExecuting,
		"Running afterAll cleanup...",
	)

	if err := t.runScript(ctx, t.TestAfterAll, opts); err != nil {
		return fmt.Errorf("run after all tests script: %w", err)
	}

	return nil
}

func (t *TestSuiteV1) runBeforeEach(ctx context.Context, opts *RunTestOptions) error {
	if t.TestBeforeEach == "" {
		return nil
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptExecuting,
		"Running beforeEach setup...",
	)

	if err := t.runScript(ctx, t.TestBeforeEach, opts); err != nil {
		return fmt.Errorf("run before each test script: %w", err)
	}

	return nil
}

func (t *TestSuiteV1) runAfterEach(ctx context.Context, opts *RunTestOptions) error {
	if t.TestAfterEach == "" {
		return nil
	}

	t.sendEvent(
		opts.EventSink,
		EventScriptExecuting,
		"Running afterEach cleanup...",
	)

	if err := t.runScript(ctx, t.TestAfterEach, opts); err != nil {
		return fmt.Errorf("run after each test script: %w", err)
	}

	return nil
}

type EnvDependency struct {
	DependencyPosition int
	DependantUnitName  string
	DependencyUnitName string
	AssignedEnvName    string
	VarName            string
	RawVarValue        string
	IsFixture          bool // Indicates if the variable is a fixture, used as a helper variable
}

func (t *TestSuiteV1) getFixture(name string) Fixture {
	for _, fixture := range t.Fixtures {
		if fixture.Name() == name {
			return fixture
		}
	}

	return nil
}

func (t *TestSuiteV1) calculateEnvDependencies() ([]EnvDependency, error) {
	var varDependencies []EnvDependency

	for depPos, unit := range t.TestUnits {
		if unit == nil {
			continue
		}

		envVars := unit.GetEnvRaw(&GetEnvRawOptions{
			WorkingDir: t.WorkingDir,
		})
		for key, value := range envVars {
			if value == "" {
				continue
			}

			// Check if the value is a fixture variable
			if FixtureInterpolationRegex.MatchString(value) {
				fixtureName := FixtureInterpolationRegex.FindStringSubmatch(value)[1]
				fixture := t.getFixture(fixtureName)

				if fixture == nil {
					return nil, fmt.Errorf(
						"fixture %s not found for unit %s",
						fixtureName,
						unit.Name(),
					)
				}

				varDependencies = append(varDependencies, EnvDependency{
					DependencyPosition: depPos,
					DependantUnitName:  unit.Name(),
					DependencyUnitName: fixture.Name(),
					AssignedEnvName:    key,
					VarName:            fixture.Name(),
					RawVarValue:        value,
					IsFixture:          true,
				})

				continue
			}

			// Check if the value is a service variable
			if !ServiceVariableInterpolationRegex.MatchString(value) {
				continue
			}

			matches := ServiceVariableInterpolationRegex.FindStringSubmatch(value)
			if len(matches) == 0 {
				continue
			}

			unitName, varName := matches[1], matches[2]
			varDependencies = append(varDependencies, EnvDependency{
				DependencyPosition: depPos,
				DependantUnitName:  unit.Name(),
				DependencyUnitName: unitName,
				AssignedEnvName:    key,
				VarName:            varName,
				RawVarValue:        value,
			})
		}
	}

	return varDependencies, nil
}

func (t *TestSuiteV1) hasCircularDependency(varDependencies []EnvDependency) bool {
	// Check for circular dependencies in the varDependencies slice
	for i, depA := range varDependencies {
		for j := i + 1; j < len(varDependencies); j++ {
			depB := varDependencies[j]
			if depA.DependencyUnitName == depB.DependantUnitName &&
				depA.DependantUnitName == depB.DependencyUnitName {
				return true
			}
		}
	}

	return false
}

func (t *TestSuiteV1) orderUnitsByDependencies(varDependencies []EnvDependency) ([]Unit, error) {
	// Reorder units based on their dependencies
	reorderedUnits := make([]Unit, len(t.TestUnits))
	copy(reorderedUnits, t.TestUnits)

	for _, dep := range varDependencies {
		unitNameThatDependsOn := dep.DependencyUnitName

		var unitThatDependOn Unit

		var unitIndexThatDependOn int

		for i, unit := range t.TestUnits {
			if unit.Name() == unitNameThatDependsOn {
				unitThatDependOn = unit
				unitIndexThatDependOn = i

				break
			}
		}

		if unitThatDependOn == nil {
			return nil, fmt.Errorf(
				"unit %s not found for dependency %s.%s",
				unitNameThatDependsOn,
				dep.DependencyUnitName,
				dep.VarName,
			)
		}

		// Place the dependant after the dependency
		reorderedUnits[unitIndexThatDependOn], reorderedUnits[dep.DependencyPosition] = reorderedUnits[dep.DependencyPosition], unitThatDependOn
	}

	return reorderedUnits, nil
}

func (t *TestSuiteV1) Run(ctx context.Context, opts *RunTestOptions) error {
	if opts == nil {
		opts = &RunTestOptions{
			FilterFunc: nil,
			Verbose:    false,
		}
	}

	if len(t.TestUnits) == 0 {
		return fmt.Errorf("no units found in test suite %s", t.TestName)
	}

	// Track suite timing
	suiteStartTime := time.Now()
	var setupEndTime time.Time
	var passedTests, failedTests, skippedTests int

	// Calculate environment variable dependencies
	varDependencies, err := t.calculateEnvDependencies()
	if err != nil {
		return fmt.Errorf("calculate env dependencies: %w", err)
	}

	// Check circular dependencies
	if t.hasCircularDependency(varDependencies) {
		return fmt.Errorf("circular dependency found in env vars")
	}

	// Reorder units based on their dependencies
	reorderedUnits, err := t.orderUnitsByDependencies(varDependencies)
	if err != nil {
		return fmt.Errorf("order units by dependencies: %w", err)
	}

	net, err := tcnetwork.New(ctx)
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}

	t.sendEvent(
		opts.EventSink,
		EventNetworkCreated,
		fmt.Sprintf("Network %s created", net.Name),
	)

	if err = t.interpolateVarsAndStartUnits(ctx, opts, reorderedUnits, varDependencies, net); err != nil {
		return fmt.Errorf("interpolate vars and start units: %w", err)
	}

	// Mark end of setup phase (containers are ready)
	setupEndTime = time.Now()

	// Stop all units
	// Remove networks
	defer func() {
		// Notify that cleanup is starting
		t.sendEvent(
			opts.EventSink,
			EventInfo,
			"Cleaning up containers...",
		)

		// Stop all units and wait for them to fully terminate
		for i := len(reorderedUnits) - 1; i >= 0; i-- {
			unit := reorderedUnits[i]
			if unit == nil {
				continue
			}

			t.sendEvent(
				opts.EventSink,
				EventContainerStopped,
				fmt.Sprintf("Stopping %s", unit.Name()),
			)

			if err := unit.Stop(); err != nil {
				fmt.Printf("failed to stop unit %s: %v\n", unit.Name(), err)
			}
		}

		// Wait for containers to actually terminate (with configurable timeout)
		t.sendEvent(
			opts.EventSink,
			EventInfo,
			"Waiting for containers to terminate...",
		)

		startWait := time.Now()
		terminationTimeout := 30 * time.Second
		if err := WaitForContainersTermination(ctx, net, terminationTimeout); err != nil {
			t.sendEvent(
				opts.EventSink,
				EventWarning,
				fmt.Sprintf("Container termination timeout after %v: %v", time.Since(startWait), err),
			)
			fmt.Printf("Warning: %v\n", err)
			// Continue anyway - ForceCleanupNetwork will do best effort cleanup
		} else {
			t.sendEvent(
				opts.EventSink,
				EventInfo,
				fmt.Sprintf("All containers terminated in %v", time.Since(startWait)),
			)
		}

		t.sendEvent(
			opts.EventSink,
			EventInfo,
			"Removing network...",
		)

		if err := ForceCleanupNetwork(ctx, net); err != nil {
			fmt.Printf("failed to cleanup network %s: %v\n", net.Name, err)
		}

		t.sendEvent(
			opts.EventSink,
			EventNetworkDestroyed,
			fmt.Sprintf("Network %s destroyed", net.Name),
		)
	}()

	// Run before all tests script if provided
	if err := t.runBeforeAll(ctx, opts); err != nil {
		return fmt.Errorf("run before all tests script: %w", err)
	}

	// Track test execution start time
	testsStartTime := time.Now()

	for _, test := range t.Tests() {
		// Run before each test script if provided
		err = t.runBeforeEach(ctx, opts)
		if err != nil {
			return err
		}

		// Run the test
		var result *TestResult

		var testErr error

		retryCount := 0
		retryDelay, _ := time.ParseDuration(opts.RetryDelay)

		for retryCount <= opts.MaxRetries {
			if retryCount > 0 {
				t.sendTestRetryEvent(
					opts.EventSink,
					test.Name(),
					retryCount,
					opts.MaxRetries,
				)
				time.Sleep(retryDelay)
			}

			// Run the test
			result, testErr = t.runTest(ctx, test, opts)

			if result != nil && result.Passed {
				break // Test passed, no need to retry
			}

			retryCount++
			if retryCount > opts.MaxRetries {
				break // Max retries reached
			}
		}

		if testErr != nil {
			// For errors without a result, we don't have timing data
			t.sendTestEvent(
				opts.EventSink,
				test.Name(),
				false,
				testErr.Error(),
				time.Duration(0),
				testErr,
			)

			return fmt.Errorf("run test %s: %w", test.Name(), testErr)
		}

		if result != nil {
			result.SuiteName = t.TestName
			if !result.Passed {
				failedTests++
				t.sendTestEvent(
					opts.EventSink,
					result.TestName,
					false,
					result.MessageOrErr(),
					result.Duration,
					result.Err,
				)

				return nil
			} else {
				passedTests++
				t.sendTestEvent(
					opts.EventSink,
					result.TestName,
					true,
					"",
					result.Duration,
					nil,
				)
			}
		}

		// Run after each test script if provided
		if err := t.runAfterEach(ctx, opts); err != nil {
			return fmt.Errorf("run after each test script: %w", err)
		}
	}

	// Run after all tests script if provided
	if err := t.runAfterAll(ctx, opts); err != nil {
		return fmt.Errorf("run after all tests script: %w", err)
	}

	// Calculate timing breakdown
	totalTime := time.Since(suiteStartTime)
	setupTime := setupEndTime.Sub(suiteStartTime)
	testTime := time.Since(testsStartTime)

	// Send suite finished event with timing breakdown
	if opts.EventSink != nil {
		opts.EventSink <- &SuiteFinishedEvent{
			BaseEvent: BaseEvent{
				EventType:    EventSuiteFinished,
				EventTime:    time.Now(),
				Suite:        t.TestName,
				EventMessage: fmt.Sprintf("Suite %s completed", t.TestName),
			},
			SetupTime:    setupTime,
			TestTime:     testTime,
			TotalTime:    totalTime,
			PassedCount:  passedTests,
			FailedCount:  failedTests,
			SkippedCount: skippedTests,
		}
	}

	return nil
}

func (t *TestSuiteV1) sendTestRetryEvent(
	eventSink EventSink,
	testName string,
	attempt int,
	maxRetries int,
) {
	if eventSink != nil {
		eventSink <- &TestRetryingEvent{
			BaseEvent: BaseEvent{
				EventType:    EventTestRetrying,
				EventTime:    time.Now(),
				Suite:        t.TestName,
				EventMessage: fmt.Sprintf("Retrying test %s (attempt %d/%d)", testName, attempt, maxRetries),
			},
			TestName:   testName,
			RetryCount: attempt,
			MaxRetries: maxRetries,
		}
	}
}

func (t *TestSuiteV1) sendEvent(eventSink EventSink, eventType EventType, message string) {
	if eventSink != nil {
		eventSink <- BaseEvent{
			EventType:    eventType,
			EventTime:    time.Now(),
			Suite:        t.TestName,
			EventMessage: message,
		}
	}
}

func (t *TestSuiteV1) sendTestEvent(
	eventSink EventSink,
	testName string,
	passed bool,
	message string,
	duration time.Duration,
	err error,
) {
	if eventSink != nil {
		eventSink <- &TestEvent{
			BaseEvent: BaseEvent{
				EventType:    EventTestCompleted,
				EventTime:    time.Now(),
				Suite:        t.TestName,
				EventMessage: message,
			},
			TestName: testName,
			Passed:   passed,
			Error:    err,
			Duration: duration,
		}
	}
}

func (t *TestSuiteV1) interpolateVarsAndStartUnits(
	ctx context.Context,
	opts *RunTestOptions,
	reorderedUnits []Unit,
	varDependencies []EnvDependency,
	net *testcontainers.DockerNetwork,
) error {
	var err error

	for _, unit := range reorderedUnits {
		if unit == nil {
			continue
		}

		// Get dependant env vars from unit
		envVars := map[string]string{}

		for _, dep := range varDependencies {
			if dep.DependantUnitName == unit.Name() {
				// Get the value of the env var from the dependency unit
				for _, dependencyUnit := range t.TestUnits {
					if dependencyUnit.Name() == dep.DependencyUnitName {
						envVars[dep.AssignedEnvName], err = dependencyUnit.Get(dep.VarName)
						if err != nil {
							err = fmt.Errorf(
								"get env var %s from unit %s: %w",
								dep.VarName,
								dep.DependencyUnitName,
								err,
							)
						}

						break
					}
				}
			}
		}

		unit.SetEnvs(envVars)

		if err = unit.Start(ctx, &UnitStartOptions{
			Network:      net,
			Verbose:      opts.Verbose,
			CacheImages:  true,
			CleanupCache: opts.CleanupCache,
			EventSink:    opts.EventSink,
			Fixtures:     t.Fixtures,
			Debug:        opts.Debug,
			WorkingDir:   opts.BaseDir,
		}); err != nil {
			err = fmt.Errorf("start unit %s: %w", unit.Name(), err)

			break
		}

		if err = unit.WaitForReady(context.Background()); err != nil {
			err = fmt.Errorf("wait for unit %s: %w", unit.Name(), err)

			break
		}
	}

	return err
}

// ForceCleanupNetwork forcefully removes all containers from a network before attempting to delete it.
// WaitForContainersTermination waits for all containers in a network to be terminated
// It polls the Docker API and returns when all containers are gone or timeout is reached
func WaitForContainersTermination(ctx context.Context, net *testcontainers.DockerNetwork, timeout time.Duration) error {
	if net == nil {
		return nil
	}

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer dockerCli.Close()

	// Get initial list of containers in the network
	networkInfo, err := dockerCli.NetworkInspect(ctx, net.Name, network.InspectOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Network already removed, containers must be gone
			return nil
		}
		return fmt.Errorf("inspect network: %w", err)
	}

	// Collect container IDs to monitor
	containerIDs := make([]string, 0, len(networkInfo.Containers))
	for containerID := range networkInfo.Containers {
		containerIDs = append(containerIDs, containerID)
	}

	if len(containerIDs) == 0 {
		// No containers to wait for
		return nil
	}

	// Poll with exponential backoff
	startTime := time.Now()
	checkInterval := 100 * time.Millisecond
	maxInterval := 1 * time.Second

	for {
		// Check if we've exceeded timeout
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for %d containers to terminate after %v",
				len(containerIDs), timeout)
		}

		// Check each container
		remainingContainers := []string{}
		for _, containerID := range containerIDs {
			_, err := dockerCli.ContainerInspect(ctx, containerID)
			if err != nil {
				// Container not found = terminated (what we want)
				if client.IsErrNotFound(err) {
					continue
				}
				// Other errors, log but continue
				fmt.Printf("Warning: Error inspecting container %s: %v\n", containerID[:12], err)
				continue
			}
			// Container still exists
			remainingContainers = append(remainingContainers, containerID)
		}

		// All containers terminated!
		if len(remainingContainers) == 0 {
			return nil
		}

		// Update container list for next iteration
		containerIDs = remainingContainers

		// Wait before next check with exponential backoff
		time.Sleep(checkInterval)
		checkInterval = time.Duration(float64(checkInterval) * 1.5)
		if checkInterval > maxInterval {
			checkInterval = maxInterval
		}
	}
}

func ForceCleanupNetwork(ctx context.Context, net *testcontainers.DockerNetwork) error {
	if net == nil {
		return nil
	}

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer dockerCli.Close()

	// Get containers connected to this specific network
	networkInfo, err := dockerCli.NetworkInspect(ctx, net.Name, network.InspectOptions{})
	if err != nil {
		// Network might already be removed, which is fine
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("inspect network: %w", err)
	}

	// Disconnect containers that are still connected to this network
	for containerID := range networkInfo.Containers {
		// Check if container still exists and is not already being removed
		_, err := dockerCli.ContainerInspect(ctx, containerID)
		if err != nil {
			// Container doesn't exist anymore, skip
			continue
		}

		err = dockerCli.NetworkDisconnect(ctx, net.Name, containerID, true)
		if err != nil && !strings.Contains(err.Error(), "is not connected") &&
			!strings.Contains(err.Error(), "marked for removal") {
			fmt.Printf("Warning: Failed to disconnect container %s: %v\n", containerID[:12], err)
		}
	}

	// Wait longer for Docker to process the disconnects and container removals
	time.Sleep(1 * time.Second)

	// Try to remove the network with retries
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = dockerCli.NetworkRemove(ctx, net.Name)
		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "not found") {
			// Network already removed, which is fine
			return nil
		}

		if i < maxRetries-1 {
			// Wait before retry
			time.Sleep(500 * time.Millisecond)
		}
	}

	return err
}
