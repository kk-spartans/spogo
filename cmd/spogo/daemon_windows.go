//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/steipete/spogo/internal/app"
	"github.com/steipete/spogo/internal/config"
)

const (
	daemonName            = "daemon"
	daemonStateFilename   = "daemon.json"
	daemonLogFilename     = "daemon.log"
	daemonStateVersion    = 1
	daemonProbeTimeout    = 800 * time.Millisecond
	daemonShutdownTimeout = 2 * time.Second
	daemonStopCallTimeout = 2 * time.Second
	daemonRunCallTimeout  = 0
	daemonStartWait       = 4 * time.Second
	daemonStartPoll       = 80 * time.Millisecond
)

type daemonState struct {
	Version     int    `json:"version"`
	Addr        string `json:"addr"`
	Token       string `json:"token"`
	PID         int    `json:"pid"`
	StartedUnix int64  `json:"started_unix"`
}

type daemonRunRequest struct {
	Args []string `json:"args"`
}

type daemonRunResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

type daemonAsyncResult struct {
	args     []string
	exitCode int
	stdout   string
	stderr   string
}

type daemonRunner struct {
	mu       sync.Mutex
	contexts map[string]*app.Context
	logs     io.Writer
}

func newDaemonRunner(logs io.Writer) *daemonRunner {
	if logs == nil {
		logs = io.Discard
	}
	return &daemonRunner{contexts: map[string]*app.Context{}, logs: logs}
}

func (r *daemonRunner) run(args []string, out io.Writer, errOut io.Writer) int {
	started := time.Now()
	r.logf("run:start args=%q", strings.Join(args, " "))
	r.mu.Lock()
	exitCode := runWithContext(args, out, errOut, r.contextForSettings, nil, true)
	r.mu.Unlock()
	r.logf("run:done code=%d elapsed=%s", exitCode, time.Since(started).Round(time.Millisecond))
	return exitCode
}

func (r *daemonRunner) contextForSettings(settings app.Settings) (*app.Context, error) {
	key := daemonSettingsKey(settings)
	if ctx, ok := r.contexts[key]; ok {
		r.logf("ctx:hit profile=%q engine=%q timeout=%s", settings.Profile, settings.Engine, settings.Timeout)
		return ctx, nil
	}
	r.logf("ctx:miss profile=%q engine=%q timeout=%s", settings.Profile, settings.Engine, settings.Timeout)
	ctx, err := app.NewContext(settings)
	if err != nil {
		return nil, err
	}
	r.contexts[key] = ctx
	return ctx, nil
}

func (r *daemonRunner) logf(format string, args ...any) {
	if r == nil || r.logs == nil {
		return
	}
	stamp := time.Now().Format("15:04:05.000")
	_, _ = fmt.Fprintf(r.logs, "[%s] daemon %s\n", stamp, fmt.Sprintf(format, args...))
}

func (r *daemonRunner) logAsyncResult(result daemonAsyncResult) {
	if result.stdout != "" {
		r.logf("async:stdout %q", strings.TrimSpace(result.stdout))
	}
	if result.stderr != "" {
		r.logf("async:stderr %q", strings.TrimSpace(result.stderr))
	}
	r.logf("async:done args=%q code=%d", strings.Join(result.args, " "), result.exitCode)
}

func daemonSettingsKey(settings app.Settings) string {
	return strings.Join([]string{
		settings.ConfigPath,
		settings.Profile,
		settings.Timeout.String(),
		settings.Market,
		settings.Language,
		settings.Device,
		settings.Engine,
		string(settings.Format),
		fmt.Sprintf("%t", settings.NoColor),
		fmt.Sprintf("%t", settings.Quiet),
		fmt.Sprintf("%t", settings.Verbose),
		fmt.Sprintf("%t", settings.Debug),
		fmt.Sprintf("%t", settings.NoInput),
	}, "\x1f")
}

func isDaemonCommand(args []string) bool {
	cmd, _, ok := firstCommandToken(args)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cmd), daemonName)
}

func runDaemonCommand(args []string, out io.Writer, errOut io.Writer) int {
	sub, _, ok := daemonSubcommand(args)
	if !ok {
		writeDaemonUsage(errOut)
		return 2
	}
	switch sub {
	case "start":
		return runDaemonStart(args, out, errOut)
	case "serve":
		return runDaemonServe(args, out, errOut)
	case "stop":
		return runDaemonStop(args, out, errOut)
	case "status":
		return runDaemonStatus(args, out, errOut)
	default:
		_, _ = fmt.Fprintf(errOut, "unknown daemon subcommand %q\n", sub)
		writeDaemonUsage(errOut)
		return 2
	}
}

func proxyToDaemon(args []string, out io.Writer, errOut io.Writer) (int, bool) {
	if !shouldProxyViaDaemon(args) {
		return 0, false
	}
	statePath, err := daemonStatePathForArgs(args)
	if err != nil {
		return 0, false
	}
	state, err := readDaemonState(statePath)
	if err != nil {
		return 0, false
	}
	var response daemonRunResponse
	err = callDaemon(state, "/run", daemonRunRequest{Args: args}, &response, daemonRunCallTimeout)
	if err != nil {
		_ = os.Remove(statePath)
		return 0, false
	}
	if response.Stdout != "" {
		_, _ = io.WriteString(out, response.Stdout)
	}
	if response.Stderr != "" {
		_, _ = io.WriteString(errOut, response.Stderr)
	}
	if response.Error != "" {
		_, _ = fmt.Fprintln(errOut, response.Error)
		if response.ExitCode == 0 {
			response.ExitCode = 1
		}
	}
	return response.ExitCode, true
}

func shouldProxyViaDaemon(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if isDaemonCommand(args) {
		return false
	}
	cmd, idx, ok := firstCommandToken(args)
	if !ok {
		return false
	}
	if strings.EqualFold(cmd, "auth") && idx+1 < len(args) && strings.EqualFold(args[idx+1], "paste") {
		return false
	}
	return true
}

func runDaemonStart(args []string, out io.Writer, errOut io.Writer) int {
	statePath, err := daemonStatePathForArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if current, err := readDaemonState(statePath); err == nil {
		if daemonPing(current) == nil {
			_, _ = fmt.Fprintf(out, "spogo daemon already running on %s (pid %d)\n", current.Addr, current.PID)
			return 0
		}
		_ = os.Remove(statePath)
	}
	logPath, err := daemonLogPathForArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	defer func() { _ = logFile.Close() }()
	prefix := daemonGlobalPrefixArgs(args)
	cmdArgs := append(prefix, daemonName, "serve")
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if err := cmd.Process.Release(); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	deadline := time.Now().Add(daemonStartWait)
	for time.Now().Before(deadline) {
		state, stateErr := readDaemonState(statePath)
		if stateErr == nil && daemonPing(state) == nil {
			_, _ = fmt.Fprintf(out, "spogo daemon running on %s (pid %d)\n", state.Addr, state.PID)
			_, _ = fmt.Fprintf(out, "logs: %s\n", logPath)
			return 0
		}
		time.Sleep(daemonStartPoll)
	}
	_, _ = fmt.Fprintln(errOut, "daemon did not start in time; check log:")
	_, _ = fmt.Fprintln(errOut, logPath)
	return 1
}

func runDaemonServe(args []string, out io.Writer, errOut io.Writer) int {
	statePath, err := daemonStatePathForArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	token, err := daemonToken()
	if err != nil {
		_ = listener.Close()
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	state := daemonState{
		Version:     daemonStateVersion,
		Addr:        listener.Addr().String(),
		Token:       token,
		PID:         os.Getpid(),
		StartedUnix: time.Now().Unix(),
	}
	if err := writeDaemonState(statePath, state); err != nil {
		_ = listener.Close()
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	defer func() { _ = os.Remove(statePath) }()
	runner := newDaemonRunner(errOut)
	runner.logf("start addr=%s pid=%d goos=%s", state.Addr, state.PID, runtime.GOOS)

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), daemonShutdownTimeout)
				defer cancel()
				_ = server.Shutdown(ctx)
			}()
		})
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		runner.logf("http %s %s", r.Method, r.URL.Path)
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !daemonAuthorized(r, state.Token) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "pid": state.PID})
	})

	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		runner.logf("http %s %s", r.Method, r.URL.Path)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !daemonAuthorized(r, state.Token) {
			runner.logf("run:unauthorized")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		defer func() { _ = r.Body.Close() }()
		var req daemonRunRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			runner.logf("run:bad-request decode=%v", err)
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(daemonRunResponse{ExitCode: 2, Error: err.Error()})
			return
		}
		if len(req.Args) == 0 {
			runner.logf("run:bad-request missing-args")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(daemonRunResponse{ExitCode: 2, Error: "missing args"})
			return
		}
		if isDaemonCommand(req.Args) {
			runner.logf("run:bad-request daemon-command")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(daemonRunResponse{ExitCode: 2, Error: "daemon commands cannot run through daemon"})
			return
		}
		if shouldRunAsync(req.Args) {
			argsCopy := append([]string(nil), req.Args...)
			runner.logf("run:async-queued args=%q", strings.Join(argsCopy, " "))
			go func() {
				stdout := &bytes.Buffer{}
				stderr := &bytes.Buffer{}
				result := daemonAsyncResult{
					args:     argsCopy,
					exitCode: runner.run(argsCopy, stdout, stderr),
					stdout:   stdout.String(),
					stderr:   stderr.String(),
				}
				runner.logAsyncResult(result)
			}()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(daemonRunResponse{ExitCode: 0})
			return
		}
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := runner.run(req.Args, stdout, stderr)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(daemonRunResponse{
			ExitCode: exitCode,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
		})
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		runner.logf("http %s %s", r.Method, r.URL.Path)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !daemonAuthorized(r, state.Token) {
			runner.logf("stop:unauthorized")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		shutdown()
	})

	_, _ = fmt.Fprintf(out, "spogo daemon running on %s (pid %d)\n", state.Addr, state.PID)
	_, _ = fmt.Fprintln(out, "keep this terminal open; use `spogo daemon stop` to stop")
	err = server.Serve(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	runner.logf("shutdown")
	return 0
}

func runDaemonStop(args []string, out io.Writer, errOut io.Writer) int {
	statePath, err := daemonStatePathForArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	state, err := readDaemonState(statePath)
	if err != nil {
		_, _ = fmt.Fprintln(out, "spogo daemon is not running")
		return 0
	}
	if err := callDaemon(state, "/stop", map[string]any{"stop": true}, nil, daemonStopCallTimeout); err != nil {
		_ = os.Remove(statePath)
		_, _ = fmt.Fprintln(out, "spogo daemon is not running")
		return 0
	}
	_, _ = fmt.Fprintln(out, "spogo daemon stopped")
	return 0
}

func runDaemonStatus(args []string, out io.Writer, errOut io.Writer) int {
	statePath, err := daemonStatePathForArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	state, err := readDaemonState(statePath)
	if err != nil {
		_, _ = fmt.Fprintln(out, "spogo daemon: not running")
		return 1
	}
	if err := daemonPing(state); err != nil {
		_ = os.Remove(statePath)
		_, _ = fmt.Fprintln(out, "spogo daemon: not running")
		return 1
	}
	uptime := time.Since(time.Unix(state.StartedUnix, 0)).Round(time.Second)
	_, _ = fmt.Fprintf(out, "spogo daemon: running (pid %d, addr %s, uptime %s)\n", state.PID, state.Addr, uptime)
	return 0
}

func writeDaemonUsage(out io.Writer) {
	_, _ = fmt.Fprintln(out, "Usage: spogo daemon <start|stop|status>")
}

func daemonStatePathForArgs(args []string) (string, error) {
	configPath, err := resolveDaemonConfigPath(args)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), daemonStateFilename), nil
}

func daemonLogPathForArgs(args []string) (string, error) {
	configPath, err := resolveDaemonConfigPath(args)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), daemonLogFilename), nil
}

func resolveDaemonConfigPath(args []string) (string, error) {
	configPath := strings.TrimSpace(os.Getenv("SPOGO_CONFIG"))
	commandIndex := firstCommandIndex(args)
	if commandIndex < 0 {
		commandIndex = len(args)
	}
	for i := 0; i < commandIndex; i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--config":
			if i+1 < commandIndex {
				configPath = strings.TrimSpace(args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
		}
	}
	if configPath == "" {
		var defaultErr error
		configPath, defaultErr = config.DefaultPath()
		if defaultErr != nil {
			return "", defaultErr
		}
	}
	if !filepath.IsAbs(configPath) {
		if abs, absErr := filepath.Abs(configPath); absErr == nil {
			configPath = abs
		}
	}
	return configPath, nil
}

func daemonSubcommand(args []string) (string, int, bool) {
	cmd, idx, ok := firstCommandToken(args)
	if !ok || !strings.EqualFold(cmd, daemonName) {
		return "", -1, false
	}
	if idx+1 >= len(args) {
		return "", -1, false
	}
	sub := strings.ToLower(strings.TrimSpace(args[idx+1]))
	if sub == "" || strings.HasPrefix(sub, "-") {
		return "", -1, false
	}
	return sub, idx, true
}

func daemonGlobalPrefixArgs(args []string) []string {
	idx := firstCommandIndex(args)
	if idx <= 0 {
		return nil
	}
	out := make([]string, 0, idx)
	out = append(out, args[:idx]...)
	return out
}

func shouldRunAsync(args []string) bool {
	if len(args) == 0 {
		return false
	}
	cmd, idx, ok := firstCommandToken(args)
	if !ok {
		return false
	}
	if strings.EqualFold(cmd, "play") || strings.EqualFold(cmd, "pause") || strings.EqualFold(cmd, "next") || strings.EqualFold(cmd, "prev") {
		return true
	}
	if strings.EqualFold(cmd, "shuffle") || strings.EqualFold(cmd, "repeat") || strings.EqualFold(cmd, "volume") || strings.EqualFold(cmd, "seek") {
		return true
	}
	if strings.EqualFold(cmd, "queue") && idx+1 < len(args) && strings.EqualFold(args[idx+1], "add") {
		return true
	}
	return false
}

func firstCommandToken(args []string) (string, int, bool) {
	idx := firstCommandIndex(args)
	if idx < 0 {
		return "", -1, false
	}
	return args[idx], idx, true
}

func firstCommandIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return i
		}
		hasValue, consumed := consumesGlobalValue(arg)
		if hasValue {
			i += consumed
		}
	}
	return -1
}

func consumesGlobalValue(arg string) (bool, int) {
	if strings.HasPrefix(arg, "--") {
		if strings.Contains(arg, "=") {
			return false, 0
		}
		switch arg {
		case "--config", "--profile", "--timeout", "--market", "--language", "--device", "--engine":
			return true, 1
		default:
			return false, 0
		}
	}
	return false, 0
}

func readDaemonState(path string) (daemonState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return daemonState{}, err
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return daemonState{}, err
	}
	if state.Version != daemonStateVersion {
		return daemonState{}, fmt.Errorf("unsupported daemon state version %d", state.Version)
	}
	if strings.TrimSpace(state.Addr) == "" || strings.TrimSpace(state.Token) == "" {
		return daemonState{}, errors.New("invalid daemon state")
	}
	return state, nil
}

func writeDaemonState(path string, state daemonState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func daemonToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func daemonAuthorized(req *http.Request, token string) bool {
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if len(got) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func daemonPing(state daemonState) error {
	return callDaemon(state, "/health", nil, nil, daemonProbeTimeout)
}

func callDaemon(state daemonState, endpoint string, reqBody any, respBody any, timeout time.Duration) error {
	var payload io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	method := http.MethodGet
	if reqBody != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequest(method, "http://"+state.Addr+endpoint, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+state.Token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{}
	if timeout > 0 {
		client.Timeout = timeout
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if len(body) > 0 {
			return fmt.Errorf("daemon returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("daemon returned %d", resp.StatusCode)
	}
	if respBody == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return err
	}
	return nil
}
