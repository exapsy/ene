package e2eframe

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"gopkg.in/yaml.v3"
)

type ServiceNotFoundError struct {
	Kind string
}

func (e *ServiceNotFoundError) Error() string {
	return fmt.Sprintf("service not found: %s", e.Kind)
}

type ConfigKeyNotFoundError struct {
	Key string
}

func (e *ConfigKeyNotFoundError) Error() string {
	return fmt.Sprintf("config key not found: %s", e.Key)
}

// UserFriendlyError interface for errors that can provide simplified messages
type UserFriendlyError interface {
	UserFriendlyMessage() string
}

// FormatError formats an error message based on debug mode
// In debug mode, returns the full error message
// In non-debug mode, returns simplified message if error implements UserFriendlyError
func FormatError(err error, debug bool) string {
	if err == nil {
		return ""
	}

	if debug {
		return err.Error()
	}

	// Walk through the error chain to find UserFriendlyError
	currentErr := err
	for currentErr != nil {
		if userFriendlyErr, ok := currentErr.(UserFriendlyError); ok {
			return userFriendlyErr.UserFriendlyMessage()
		}

		// Try to unwrap the error
		if unwrapper, ok := currentErr.(interface{ Unwrap() error }); ok {
			currentErr = unwrapper.Unwrap()
		} else {
			break
		}
	}

	return err.Error()
}

type UnitStartOptions struct {
	Debug bool
	// Network is the network to use for the service.
	// If not set, a new network will be created.
	Network *testcontainers.DockerNetwork
	// Verbose enables verbose logging.
	Verbose bool
	// CacheImages enables caching for the service.
	CacheImages bool
	// CleanupCache removes old cached images to prevent bloat
	CleanupCache bool
	// Fixtures is a list of fixtures to apply on interpolations.
	Fixtures []Fixture
	// EventSink is a channel to send events to.
	EventSink EventSink
	// WorkingDir is the base directory
	WorkingDir string
}

type GetEnvRawOptions struct {
	WorkingDir string
}

type Unit interface {
	// Name is the name of the service
	Name() string

	// Start starts the service.
	Start(ctx context.Context, opts *UnitStartOptions) error

	// WaitForReady waits for the service to be ready.
	WaitForReady(ctx context.Context) error

	// Stop stops the service.
	Stop() error

	// ExternalEndpoint returns the external URL/addr for tests.
	ExternalEndpoint() string

	// LocalEndpoint returns the container URL/addr for internal container communication.
	LocalEndpoint() string

	// Get returns a service-specific variable
	Get(key string) (string, error)

	// GetEnvRaw returns the environment variables for the service,
	// but pre-processed without any interpolation.
	GetEnvRaw(opts *GetEnvRawOptions) map[string]string

	// SetEnvs sets the environment variables for the service.
	SetEnvs(env map[string]string)
}

type unitFactory func(node *yaml.Node) (Unit, error)

var unitMarshallerRegistry = make(map[UnitKind]unitFactory)

// RegisterUnitMarshaller registers a new unit kind with its factory function.
func RegisterUnitMarshaller(kind UnitKind, factory unitFactory) {
	if _, ok := unitMarshallerRegistry[kind]; ok {
		panic("unit already registered")
	}

	unitMarshallerRegistry[kind] = factory
}

// UnmarshallUnit unmarshals a YAML node into a Unit.
func UnmarshallUnit(kind UnitKind, node *yaml.Node) (Unit, error) {
	factory, ok := unitMarshallerRegistry[kind]
	if !ok {
		return nil, &ServiceNotFoundError{Kind: string(kind)}
	}

	return factory(node)
}
