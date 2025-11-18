package e2eframe

import (
	"fmt"
	"net"
	"os"
	"regexp"
)

// InterpolateEnvs gets the envCfg,
// checks if any value has an interpolation request (e.g. `{{ service.var }}` ),
// requests the variable from the corresponding service,
// and replaces it in the original envCfg.
func InterpolateEnvs(services []Unit, envCfg map[string]any) error {
	// {{ <service_name>.<variable> }}
	envRegex := regexp.MustCompile(`^\{\{ ([A-Za-z0-9_]+)\.([A-Za-z0-9_]+) }}$`)

	for key, value := range envCfg {
		valueStr, ok := value.(string)
		if !ok {
			continue
		}

		submatch := envRegex.FindStringSubmatch(valueStr)
		serviceName := submatch[1]
		variableName := submatch[2]

		for _, service := range services {
			if service.Name() == serviceName {
				variableValue, err := service.Get(variableName)
				if err != nil {
					return fmt.Errorf(
						"interpolate environment variable %s failed: %s",
						variableName,
						err,
					)
				}

				envCfg[key] = variableValue
			}
		}
	}

	return nil
}

// GetFreePort asks the kernel for a free open port that is ready to use.
func GetFreePort() (port int, err error) {
	var a *net.TCPAddr

	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()

			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}

	return
}

func HasTestEnvFile() bool {
	_, err := os.Stat("./.env.test")
	if err != nil {
		return false
	}

	return true
}

type Color struct {
	Reset  string
	Red    string
	Green  string
	Yellow string
	Blue   string
	Purple string
	Cyan   string
	White  string
	Gray   string
	Bold   string
	Dim    string
}

func NewColor(pretty bool) Color {
	if !pretty {
		return Color{}
	}

	return Color{
		Reset:  "\033[0m",
		Red:    "\033[31m",
		Green:  "\033[32m",
		Yellow: "\033[33m",
		Blue:   "\033[34m",
		Purple: "\033[35m",
		Cyan:   "\033[36m",
		White:  "\033[37m",
		Gray:   "\033[90m",
		Bold:   "\033[1m",
		Dim:    "\033[2m",
	}
}
