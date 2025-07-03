package httptest

import "time"

type Config struct {
	TestName string        `yaml:"name"`
	TestKind string        `yaml:"kind"`
	Timeout  time.Duration `yaml:"timeout"`
}

type ConfigRequest struct {
	Path    string            `yaml:"path"`
	Body    any               `yaml:"body"`
	Headers map[string]string `yaml:"headers"`
}

type ConfigResponse struct {
	StatusCode   int                          `yaml:"status_code"`
	BodyAssert   []ConfigResponseBodyAssert   `yaml:"body_assert"`
	HeaderAssert []ConfigResponseHeaderAssert `yaml:"header_assert"`
}

type BodyFieldType string

const (
	BodyFieldTypeString BodyFieldType = "string"
	BodyFieldTypeInt    BodyFieldType = "int"
	BodyFieldTypeFloat  BodyFieldType = "float"
	BodyFieldTypeBool   BodyFieldType = "bool"
	BodyFieldTypeArray  BodyFieldType = "array"
	BodyFieldTypeObject BodyFieldType = "object"
)

func (t BodyFieldType) IsValid() bool {
	switch t {
	case BodyFieldTypeString,
		BodyFieldTypeInt,
		BodyFieldTypeFloat,
		BodyFieldTypeBool,
		BodyFieldTypeArray,
		BodyFieldTypeObject:
		return true
	default:
		return false
	}
}

type ConfigResponseBodyAssert struct {
	Path        string        `yaml:"path"`
	Contains    string        `yaml:"contains"`
	NotContains string        `yaml:"not_contains"`
	Equals      string        `yaml:"equals"`
	NotEquals   string        `yaml:"not_equals"`
	Matches     string        `yaml:"matches"`
	NotMatches  string        `yaml:"not_matches"`
	Present     string        `yaml:"present"`
	Size        int           `yaml:"length"`
	GreaterThan int           `yaml:"greater_than"`
	LessThan    int           `yaml:"less_than"`
	Type        BodyFieldType `yaml:"type"`
}

type ConfigResponseHeaderAssert struct {
	Contains    string `yaml:"contains"`
	NotContains string `yaml:"not_contains"`
	Equals      string `yaml:"equals"`
	NotEquals   string `yaml:"not_equals"`
	Matches     string `yaml:"matches"`
	NotMatches  string `yaml:"not_matches"`
	IsPresent   string `yaml:"is_present"`
}
