package e2eframe

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type ErrParamIsRequired struct {
	Param string
}

func (e ErrParamIsRequired) Error() string {
	return "parameter is required: " + e.Param
}

var (
	ErrUnitNameRequired = &ErrParamIsRequired{Param: "name"}
	ErrUnitKindRequired = &ErrParamIsRequired{Param: "kind"}
)

type ConfigKind string

const (
	// ConfigKindE2ETest is the version 1 of the test suite configuration.
	ConfigKindE2ETest ConfigKind = "e2e_test:v1"
)

type Config interface {
	// Kind returns the kind of the configuration.
	Kind() ConfigKind
	// UnmarshalYAML unmarshals the YAML configuration into the test suite config.
	UnmarshalYAML(node *yaml.Node) error
}

type TestSuiteConfig interface {
	// Name returns the name of the test suite.
	Name() string
	// CreateTestSuite creates a test suite from the configuration.
	CreateTestSuite() (TestSuite, error)

	Config
}

type TestSuiteConfigV1 struct {
	TestName       string          `yaml:"name"`
	Fixtures       []Fixture       `yaml:"fixtures"`
	BeforeAll      string          `yaml:"before_all,omitempty"`
	AfterAll       string          `yaml:"after_all,omitempty"`
	BeforeEach     string          `yaml:"before_each,omitempty"`
	AfterEach      string          `yaml:"after_each,omitempty"`
	TestKind       ConfigKind      `yaml:"kind"`
	Units          []Unit          `yaml:"units"`
	Tests          []TestSuiteTest `yaml:"tests"`
	TestTargetName string          `yaml:"target"`
	RelativePath   string
}

func (t *TestSuiteConfigV1) Name() string {
	return t.TestName
}

func (t *TestSuiteConfigV1) Kind() ConfigKind {
	return t.TestKind
}

func (t *TestSuiteConfigV1) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node to yaml mapping, got: %v", node.Kind)
	}

	// Walk through the YAML node and unmarshal each field
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		switch key.Value {
		case "name":
			if err := value.Decode(&t.TestName); err != nil {
				return err
			}
		case "kind":
			if err := value.Decode(&t.TestKind); err != nil {
				return err
			}
		case "fixtures":
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected sequence node to yaml sequence, got: %v", value.Kind)
			}

			for i := 0; i < len(value.Content); i++ {
				fixture := &FixtureV1{RelativePath: t.RelativePath}
				fixtureValue := value.Content[i]

				if err := fixtureValue.Decode(fixture); err != nil {
					return err
				}

				t.Fixtures = append(t.Fixtures, fixture)
			}
		case "units":
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected sequence node to yaml sequence, got: %v", value.Kind)
			}

			type unitTmp struct {
				Kind UnitKind `yaml:"kind"`
			}

			for i := 0; i < len(value.Content); i++ {
				unit := &unitTmp{}
				unitValue := value.Content[i]

				if err := unitValue.Decode(unit); err != nil {
					return err
				}

				unitImpl, err := UnmarshallUnit(unit.Kind, unitValue)
				if err != nil {
					return err
				}

				t.Units = append(t.Units, unitImpl)
			}
		case "tests":
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("expected sequence node to yaml sequence, got: %v", value.Kind)
			}

			type testTmp struct {
				Kind TestSuiteTestKind `yaml:"kind"`
			}

			for i := 0; i < len(value.Content); i++ {
				test := &testTmp{}
				testValue := value.Content[i]

				if err := testValue.Decode(test); err != nil {
					return err
				}

				testImpl, err := UnmarshallTestSuiteTest(test.Kind, testValue)
				if err != nil {
					return err
				}

				t.Tests = append(t.Tests, testImpl)
			}
		case "target":
			if err := value.Decode(&t.TestTargetName); err != nil {
				return err
			}
		case "before_all":
			if err := value.Decode(&t.BeforeAll); err != nil {
				return fmt.Errorf("could not decode before_all: %w", err)
			}
		case "after_all":
			if err := value.Decode(&t.AfterAll); err != nil {
				return fmt.Errorf("could not decode after_all: %w", err)
			}
		case "before_each":
			if err := value.Decode(&t.BeforeEach); err != nil {
				return fmt.Errorf("could not decode before_each: %w", err)
			}
		case "after_each":
			if err := value.Decode(&t.AfterEach); err != nil {
				return fmt.Errorf("could not decode after_each: %w", err)
			}
		default:
			return fmt.Errorf("unknown field: %s", key.Value)
		}
	}

	if t.TestName == "" {
		return errors.New("test name is required")
	}

	if t.TestKind == "" {
		return errors.New("test kind is required")
	}

	if t.Units == nil {
		return errors.New("units is required")
	}

	if t.Tests == nil {
		return errors.New("tests is required")
	}

	if t.TestTargetName == "" {
		return errors.New("target is required")
	}

	var target Unit

	for _, unit := range t.Units {
		if unit.Name() == t.TestTargetName {
			target = unit

			break
		}
	}

	// Target unit must be found in the units list
	if target == nil {
		return fmt.Errorf("target unit not found: %s", t.TestTargetName)
	}

	if err := t.validateDuplicateTestNames(); err != nil {
		return err
	}

	return nil
}

func (t *TestSuiteConfigV1) findUnit(name string) Unit {
	for _, unit := range t.Units {
		if unit.Name() == name {
			return unit
		}
	}

	return nil
}

func (t *TestSuiteConfigV1) Target() Unit {
	return t.findUnit(t.TestTargetName)
}

func (t *TestSuiteConfigV1) validateDuplicateTestNames() error {
	testNames := make(map[string]struct{})

	for _, test := range t.Tests {
		name := test.Name()
		if _, exists := testNames[name]; exists {
			return fmt.Errorf("duplicate test name: %s", name)
		}

		testNames[name] = struct{}{}
	}

	return nil
}

type CreateSuiteParams struct {
	RelativePath string
	WorkingDir   string
}

func (t *TestSuiteConfigV1) CreateTestSuite(params CreateSuiteParams) (TestSuite, error) {
	if t.TestKind != ConfigKindE2ETest {
		return nil, fmt.Errorf("unsupported test suite kind: %s", t.TestKind)
	}

	target := t.Target()

	// add relative path to fixtures
	for i, fixture := range t.Fixtures {
		if fixture, ok := fixture.(*FixtureV1); ok {
			fixture.RelativePath = params.RelativePath
			t.Fixtures[i] = fixture
		} else {
			return nil, fmt.Errorf("fixture %d is not a FixtureV1", i)
		}
	}

	testSuite := &TestSuiteV1{
		WorkingDir:     params.WorkingDir,
		RelativePath:   params.RelativePath,
		TestKind:       t.TestKind,
		TestName:       t.TestName,
		Fixtures:       t.Fixtures,
		TestBeforeAll:  t.BeforeAll,
		TestAfterAll:   t.AfterAll,
		TestBeforeEach: t.BeforeEach,
		TestAfterEach:  t.AfterEach,
		TestUnits:      t.Units,
		TestSuiteTests: t.Tests,
		TestTarget:     target,
	}

	return testSuite, nil
}
