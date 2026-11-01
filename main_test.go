package main

import (
	"strings"
	"testing"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Output of httpd 2.4.69 (event MPM, ExtendedStatus On) for /apache-status?auto.
const eventStatus = `127.0.0.1
ServerVersion: Apache/2.4.69 (Unix)
ServerMPM: event
Server Built: Oct 28 2026 01:15:47
ServerUptimeSeconds: 4
Total Accesses: 0
ReqPerSec: 0
BusyWorkers: 1
GracefulWorkers: 0
IdleWorkers: 74
Processes: 3
Stopping: 0
ConnsTotal: 0
Scoreboard: __________W________.....
`

// Output of an older prefork server without ExtendedStatus.
const preforkStatus = `BusyWorkers: 3
IdleWorkers: 5
Scoreboard: _W_KW___..
`

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    Status
		wantErr string
	}{
		{
			name: "event",
			body: eventStatus,
			want: Status{Version: "Apache/2.4.69 (Unix)", MPM: "event", BusyWorkers: 1, Scoreboard: "__________W________....."},
		},
		{
			name: "prefork without extended status",
			body: preforkStatus,
			want: Status{BusyWorkers: 3, Scoreboard: "_W_KW___.."},
		},
		{
			name:    "html page",
			body:    "<html><body><h1>It works!</h1></body></html>\n",
			wantErr: "not an Apache status page",
		},
		{
			name:    "invalid busy workers",
			body:    "BusyWorkers: many\n",
			wantErr: `invalid BusyWorkers "many"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatus(strings.NewReader(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseStatus() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseStatus() = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	event := Status{Version: "Apache/2.4.69 (Unix)", MPM: "event", Scoreboard: strings.Repeat("_", 400)}
	busy := func(s Status, n int) Status { s.BusyWorkers = n; return s }

	tests := []struct {
		name       string
		status     Status
		maxWorkers int
		want       Result
	}{
		{
			name:       "ok",
			status:     busy(event, 12),
			maxWorkers: 150,
			want:       Result{sensu.CheckStateOK, "Apache/2.4.69 (Unix) event MPM: 12/150 workers busy (8%)"},
		},
		{
			name:       "warning threshold reached",
			status:     busy(event, 120),
			maxWorkers: 150,
			want:       Result{sensu.CheckStateWarning, "Apache/2.4.69 (Unix) event MPM: 120/150 workers busy (80%)"},
		},
		{
			name:       "critical threshold reached",
			status:     busy(event, 143),
			maxWorkers: 150,
			want:       Result{sensu.CheckStateCritical, "Apache/2.4.69 (Unix) event MPM: 143/150 workers busy (95%)"},
		},
		{
			name:       "more busy workers than the given capacity",
			status:     busy(event, 200),
			maxWorkers: 150,
			want:       Result{sensu.CheckStateCritical, "Apache/2.4.69 (Unix) event MPM: 200/150 workers busy (133%)"},
		},
		{
			name:   "capacity from scoreboard",
			status: busy(event, 120),
			want:   Result{sensu.CheckStateOK, "Apache/2.4.69 (Unix) event MPM: 120/400 workers busy (30%, capacity from scoreboard)"},
		},
		{
			name:   "no version nor MPM",
			status: Status{BusyWorkers: 9, Scoreboard: "_W_KW___.."},
			want:   Result{sensu.CheckStateWarning, "Apache: 9/10 workers busy (90%, capacity from scoreboard)"},
		},
		{
			name:   "no capacity",
			status: Status{BusyWorkers: 1},
			want:   Result{sensu.CheckStateUnknown, "no scoreboard in the status page, set --max-workers"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluate(tt.status, tt.maxWorkers, 80, 95); got != tt.want {
				t.Errorf("evaluate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEvaluateHTTP(t *testing.T) {
	tests := []struct {
		code     int
		wantDone bool
		want     int
	}{
		{200, false, sensu.CheckStateOK},
		{301, true, sensu.CheckStateWarning},
		{302, true, sensu.CheckStateWarning},
		{401, true, sensu.CheckStateWarning},
		{403, true, sensu.CheckStateWarning},
		{404, true, sensu.CheckStateWarning},
		{400, true, sensu.CheckStateCritical},
		{500, true, sensu.CheckStateCritical},
		{503, true, sensu.CheckStateCritical},
	}
	for _, tt := range tests {
		got, done := evaluateHTTP(tt.code, "http://127.0.0.1:80/apache-status?auto")
		if done != tt.wantDone || got.State != tt.want {
			t.Errorf("evaluateHTTP(%d) = %+v, %v, want state %d, done %v", tt.code, got, done, tt.want, tt.wantDone)
		}
	}
}

func TestStatusURL(t *testing.T) {
	tests := []struct {
		host, path string
		port       int
		want       string
	}{
		{"127.0.0.1", "apache-status", 80, "http://127.0.0.1:80/apache-status?auto"},
		{"localhost", "/server-status", 8080, "http://localhost:8080/server-status?auto"},
		{"::1", "apache-status", 80, "http://[::1]:80/apache-status?auto"},
		{"[::1]", "apache-status", 80, "http://[::1]:80/apache-status?auto"},
		{"127.0.0.1", "status?refresh=0", 80, "http://127.0.0.1:80/status?refresh=0&auto"},
	}
	for _, tt := range tests {
		if got := statusURL(tt.host, tt.port, tt.path); got != tt.want {
			t.Errorf("statusURL(%q, %d, %q) = %q, want %q", tt.host, tt.port, tt.path, got, tt.want)
		}
	}
}
