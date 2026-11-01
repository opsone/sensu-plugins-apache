package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Host       string
	Port       int
	Path       string
	MaxWorkers int
	Warning    int
	Critical   int
	Timeout    int
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-apache-status",
			Short:    "Check Apache through its mod_status page: availability and worker saturation",
			Keyspace: "sensu.io/plugins/check-apache-status/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "host",
			Env:       "CHECK_APACHE_HOST",
			Argument:  "host",
			Shorthand: "H", // -h is reserved for --help
			Usage:     "Apache host, e.g. 127.0.0.1 (required)",
			Value:     &plugin.Host,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "port",
			Env:       "CHECK_APACHE_PORT",
			Argument:  "port",
			Shorthand: "p",
			Usage:     "Apache port, e.g. 80 (required)",
			Value:     &plugin.Port,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "path",
			Env:       "CHECK_APACHE_PATH",
			Argument:  "path",
			Shorthand: "q",
			Usage:     "Path of the mod_status handler, e.g. apache-status (required, ?auto is appended)",
			Value:     &plugin.Path,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "max-workers",
			Env:       "CHECK_APACHE_MAX_WORKERS",
			Argument:  "max-workers",
			Shorthand: "m",
			Usage:     "MaxRequestWorkers of the server, used as capacity (0: scoreboard size, which may overestimate it)",
			Value:     &plugin.MaxWorkers,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "warning",
			Env:       "CHECK_APACHE_WARNING",
			Argument:  "warning",
			Shorthand: "w",
			Default:   80,
			Usage:     "Busy workers percentage of the capacity raising a warning",
			Value:     &plugin.Warning,
		},
		&sensu.PluginConfigOption[int]{
			Path:      "critical",
			Env:       "CHECK_APACHE_CRITICAL",
			Argument:  "critical",
			Shorthand: "c",
			Default:   95,
			Usage:     "Busy workers percentage of the capacity raising a critical",
			Value:     &plugin.Critical,
		},
		&sensu.PluginConfigOption[int]{
			Path:     "timeout",
			Env:      "CHECK_APACHE_TIMEOUT",
			Argument: "timeout",
			Default:  10,
			Usage:    "HTTP request timeout in seconds",
			Value:    &plugin.Timeout,
		},
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	switch {
	case plugin.Host == "":
		return sensu.CheckStateUnknown, fmt.Errorf("--host is required")
	case plugin.Port < 1 || plugin.Port > 65535:
		return sensu.CheckStateUnknown, fmt.Errorf("--port is required, between 1 and 65535")
	case strings.Trim(plugin.Path, "/") == "":
		return sensu.CheckStateUnknown, fmt.Errorf("--path is required")
	case plugin.MaxWorkers < 0:
		return sensu.CheckStateUnknown, fmt.Errorf("--max-workers must be 0 or greater")
	case plugin.Warning < 1 || plugin.Critical > 100 || plugin.Warning > plugin.Critical:
		return sensu.CheckStateUnknown, fmt.Errorf("thresholds must satisfy 1 <= --warning <= --critical <= 100")
	case plugin.Timeout <= 0:
		return sensu.CheckStateUnknown, fmt.Errorf("--timeout must be greater than 0")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	result := fetch(statusURL(plugin.Host, plugin.Port, plugin.Path))
	fmt.Printf("%s %s: %s\n", plugin.Name, stateLabel(result.State), result.Message)
	return result.State, nil
}

// Result is the outcome of the check.
type Result struct {
	State   int
	Message string
}

func fetch(statusURL string) Result {
	client := &http.Client{
		Timeout: time.Duration(plugin.Timeout) * time.Second,
		// Report redirections instead of following them: they mean a misconfigured path.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(statusURL)
	if err != nil {
		return Result{sensu.CheckStateCritical, fmt.Sprintf("Apache is down: %v", describeError(err))}
	}
	defer resp.Body.Close()

	if r, done := evaluateHTTP(resp.StatusCode, statusURL); done {
		return r
	}
	status, err := parseStatus(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{sensu.CheckStateWarning, fmt.Sprintf("%s: %v", statusURL, err)}
	}
	return evaluate(status, plugin.MaxWorkers, plugin.Warning, plugin.Critical)
}

// statusURL builds the machine-readable mod_status URL.
func statusURL(host string, port int, path string) string {
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/" + strings.TrimPrefix(path, "/") + sep + "auto"
}

// evaluateHTTP classifies a non-200 status code; done is false when the body must be parsed.
func evaluateHTTP(code int, target string) (r Result, done bool) {
	switch {
	case code == http.StatusOK:
		return Result{}, false
	case code >= 300 && code < 400, code == http.StatusUnauthorized, code == http.StatusForbidden, code == http.StatusNotFound:
		return Result{sensu.CheckStateWarning, fmt.Sprintf("status endpoint misconfigured: HTTP %d on %s", code, target)}, true
	default:
		return Result{sensu.CheckStateCritical, fmt.Sprintf("Apache error: HTTP %d on %s", code, target)}, true
	}
}

// Status holds the fields of the mod_status ?auto page used by the check.
type Status struct {
	Version     string
	MPM         string
	BusyWorkers int
	Scoreboard  string
}

var errNotStatus = errors.New("not an Apache status page (no BusyWorkers field)")

// parseStatus reads a mod_status ?auto page ("Key: value" lines).
func parseStatus(r io.Reader) (Status, error) {
	var s Status
	busy := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "ServerVersion":
			s.Version = value
		case "ServerMPM":
			s.MPM = value
		case "Scoreboard":
			s.Scoreboard = value
		case "BusyWorkers":
			n, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid BusyWorkers %q", value)
			}
			s.BusyWorkers, busy = n, true
		}
	}
	if err := scanner.Err(); err != nil {
		return s, fmt.Errorf("reading status page: %w", err)
	}
	if !busy {
		return s, errNotStatus
	}
	return s, nil
}

// evaluate compares the busy workers with the capacity: maxWorkers, or the scoreboard
// size when 0 (ServerLimit x ThreadsPerChild, which may exceed MaxRequestWorkers).
func evaluate(s Status, maxWorkers, warning, critical int) Result {
	capacity, source := maxWorkers, ""
	if capacity == 0 {
		capacity, source = len(s.Scoreboard), ", capacity from scoreboard"
	}
	if capacity == 0 {
		return Result{sensu.CheckStateUnknown, "no scoreboard in the status page, set --max-workers"}
	}

	pct := s.BusyWorkers * 100 / capacity
	state := sensu.CheckStateOK
	switch {
	case pct >= critical:
		state = sensu.CheckStateCritical
	case pct >= warning:
		state = sensu.CheckStateWarning
	}

	server := s.Version
	if server == "" {
		server = "Apache"
	}
	if s.MPM != "" {
		server += " " + s.MPM + " MPM"
	}
	return Result{state, fmt.Sprintf("%s: %d/%d workers busy (%d%%%s)", server, s.BusyWorkers, capacity, pct, source)}
}

// describeError drops the `Get "<url>":` prefix added by net/http, keeping the URL at the end.
func describeError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("%v (%s)", urlErr.Err, urlErr.URL)
	}
	return err.Error()
}

func stateLabel(state int) string {
	switch state {
	case sensu.CheckStateOK:
		return "OK"
	case sensu.CheckStateWarning:
		return "WARNING"
	case sensu.CheckStateCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}
