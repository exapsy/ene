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

			for j := 0; j < len(value.Content); j += 2 {
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
		if routeMethodHandler[route.Path] == nil {
			routeMethodHandler[route.Path] = map[string]func(http.ResponseWriter, *http.Request){}
		}

		routeMethodHandler[route.Path][route.Method] = func(w http.ResponseWriter, r *http.Request) {
			body := route.Response.Body
			status := route.Response.Status
			headers := route.Response.Headers

			// Set headers
			for headerName, headerValue := range headers {
				w.Header().Set(headerName, headerValue)
			}

			// Send response w/ status
			w.WriteHeader(status)

			// Write body
			switch b := body.(type) {
			case string:
				w.Write([]byte(b))
			case map[string]interface{}:
				bytes, err := json.Marshal(b)
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
		mux.HandleFunc(route.Path, func(w http.ResponseWriter, r *http.Request) {
			routeMethodHandler[route.Path][r.Method](w, r)
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
