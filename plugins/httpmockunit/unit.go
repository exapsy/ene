package httpmockunit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"gopkg.in/yaml.v3"
)

const (
	UnitKind e2eframe.UnitKind = "httpmock"
)

type Unit struct {
	server   *http.Server
	Port     int     `yaml:"port"`
	UnitName string  `yaml:"name"`
	Routes   []Route `yaml:"routes"`
}

type Route struct {
	Path     string   `yaml:"path"`
	Method   string   `yaml:"method"`
	Response Response `yaml:"response"`
}

type Response struct {
	Status  int               `yaml:"status"`
	Body    any               `yaml:"body"`
	Headers map[string]string `yaml:"headers"`
	Delay   string            `yaml:"delay"`
}

func init() {
	e2eframe.RegisterUnitMarshaller(UnitKind, UnmarshallUnit)
}

func New(cfg map[string]any) (e2eframe.Unit, error) {
	name, ok := cfg["name"].(string)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'name'")
	}

	routes, ok := cfg["routes"].([]Route)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'routes'")
	}

	return &Unit{
		UnitName: name,
		Routes:   routes,
	}, nil
}

func UnmarshallUnit(node *yaml.Node) (e2eframe.Unit, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	m := map[string]interface{}{}

	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]

		switch key {
		case "name":
			m["name"] = value.Value
		case "routes":
			if value.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("expected sequence node, got %v", key)
			}

			var routes []Route

			for j := 0; j < len(value.Content); j++ {
				routeValue := value.Content[j]

				var route Route

				if err := routeValue.Decode(&route); err != nil {
					return nil, fmt.Errorf("error decoding route: %v", err)
				}

				routes = append(routes, route)
			}

			m["routes"] = routes
		}
	}

	return New(m)
}

func (u *Unit) Name() string {
	return u.UnitName
}

func (u *Unit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	u.sendEvent(
		opts.EventSink,
		e2eframe.EventMockServerBuilding,
		fmt.Sprintf("Building mock server %s", u.UnitName),
	)

	routeMethodHandler := map[string]map[string]func(http.ResponseWriter, *http.Request){}
	for _, route := range u.Routes {
		// Interpolate fixtures in route path
		interpolatedPath := u.interpolateFixtures(route.Path, opts.Fixtures)

		if routeMethodHandler[interpolatedPath] == nil {
			routeMethodHandler[interpolatedPath] = map[string]func(http.ResponseWriter, *http.Request){}
		}

		// Capture route response in closure to avoid referencing loop variable
		responseBody := route.Response.Body
		responseStatus := route.Response.Status
		responseHeaders := route.Response.Headers
		responseDelay := route.Response.Delay

		routeMethodHandler[interpolatedPath][route.Method] = func(w http.ResponseWriter, r *http.Request) {
			body := responseBody
			status := responseStatus
			headers := responseHeaders
			delay := responseDelay

			// Add delay if specified
			if delay != "" {
				duration, err := time.ParseDuration(delay)
				if err == nil {
					time.Sleep(duration)
				}
				// Note: Could log error, but opts.Logger not available in closure
			}

			// Set headers with fixture interpolation
			for headerName, headerValue := range headers {
				interpolatedHeaderName := u.interpolateFixtures(headerName, opts.Fixtures)
				interpolatedHeaderValue := u.interpolateFixtures(headerValue, opts.Fixtures)
				w.Header().Set(interpolatedHeaderName, interpolatedHeaderValue)
			}

			// Send response w/ status
			w.WriteHeader(status)

			// Write body
			switch b := body.(type) {
			case string:
				interpolatedBody := u.interpolateFixtures(b, opts.Fixtures)
				w.Write([]byte(interpolatedBody))
			case map[string]interface{}:
				// Interpolate fixtures in JSON object
				interpolatedBody := u.interpolateFixturesInMap(b, opts.Fixtures)
				bytes, err := json.Marshal(interpolatedBody)
				if err != nil {
					return
				}

				w.Write(bytes)
			}
		}
	}

	// setup routes
	mux := http.NewServeMux()
	for _, route := range u.Routes {
		// Use interpolated path for route registration
		interpolatedPath := u.interpolateFixtures(route.Path, opts.Fixtures)
		// Capture interpolatedPath in closure to avoid referencing loop variable
		path := interpolatedPath
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			handler := routeMethodHandler[path][r.Method]
			if handler == nil {
				http.NotFound(w, r)
				return
			}
			handler(w, r)
		})
	}

	randomPort, err := e2eframe.GetFreePort()
	if err != nil {
		return fmt.Errorf("could not get free port: %v", err)
	}

	u.Port = randomPort

	u.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", u.Port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		err = u.server.ListenAndServe()
		if err != nil {
			u.sendEvent(
				opts.EventSink,
				e2eframe.EventMockServerError,
				fmt.Sprintf("Mock server %s failed to start: %v", u.UnitName, err),
			)

			return
		}
	}()

	u.sendEvent(
		opts.EventSink,
		e2eframe.EventMockServerStarted,
		fmt.Sprintf("Mock server %s started on port %d", u.UnitName, u.Port),
	)

	return nil
}

func (u *Unit) Stop() error {
	return nil
}

func (u *Unit) WaitForReady(ctx context.Context) error {
	return nil
}

func (u *Unit) ExternalEndpoint() string {
	return fmt.Sprintf("http://localhost:%d", u.Port)
}

func (u *Unit) LocalEndpoint() string {
	return fmt.Sprintf("http://localhost:%d", u.Port)
}

func (u *Unit) Get(_ string) (string, error) {
	return "", nil
}

func (u *Unit) GetEnvRaw(_ *e2eframe.GetEnvRawOptions) map[string]string {
	return map[string]string{}
}

func (u *Unit) SetEnvs(env map[string]string) {
	// noop
}

func (u *Unit) interpolateFixtures(str string, fixtures []e2eframe.Fixture) string {
	if str == "" {
		return str
	}

	interpolationRegex := e2eframe.FixtureInterpolationRegex
	if interpolationRegex.MatchString(str) {
		return e2eframe.InterpolateString(interpolationRegex, str, fixtures)
	}

	return str
}

// interpolateFixturesInMap recursively interpolates fixtures in maps and slices
func (u *Unit) interpolateFixturesInMap(data interface{}, fixtures []e2eframe.Fixture) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = u.interpolateFixturesInMap(value, fixtures)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, value := range v {
			result[i] = u.interpolateFixturesInMap(value, fixtures)
		}
		return result
	case string:
		return u.interpolateFixtures(v, fixtures)
	default:
		return v
	}
}

func (u *Unit) sendEvent(sink e2eframe.EventSink, eventType e2eframe.EventType, message string) {
	if sink != nil {
		sink <- &e2eframe.UnitEvent{
			BaseEvent: e2eframe.BaseEvent{
				EventType:    eventType,
				EventTime:    time.Now(),
				EventMessage: message,
			},
			UnitName: u.UnitName,
			UnitKind: UnitKind,
		}
	}
}
