package daemon

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"
)

const (
	DefaultServerURL                      = "ws://localhost:8080/ws"
	DefaultPollInterval                   = 30 * time.Second
	DefaultHeartbeatInterval              = 15 * time.Second
	DefaultAgentTimeout                   = 2 * time.Hour
	DefaultCodexSemanticInactivityTimeout = 10 * time.Minute
	DefaultRuntimeName                    = "Local Agent"
	DefaultWorkspaceSyncInterval          = 30 * time.Second
	DefaultHealthPort                     = 19514
	DefaultMaxConcurrentTasks             = 20
	DefaultGCInterval                     = 1 * time.Hour
	DefaultGCTTL                          = 24 * time.Hour // 1 day — AI-coding issues rarely stay open long
	DefaultGCOrphanTTL                    = 72 * time.Hour // 3 days — orphans with no meta (crashes, pre-GC leftovers)
)

// Config holds all daemon configuration.
type Config struct {
	ServerBaseURL                  string
	DaemonID                       string
	LegacyDaemonIDs                []string // historical daemon_ids this machine may have registered under; reported at register time so the server can merge old runtime rows
	DeviceName                     string
	RuntimeName                    string
	CLIVersion                     string                // multica CLI version (e.g. "0.1.13")
	LaunchedBy                     string                // "desktop" when spawned by the Electron app, empty for standalone
	Profile                        string                // profile name (empty = default)
	Agents                         map[string]AgentEntry // keyed by provider: claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor, kimi, kiro, agy
	WorkspacesRoot                 string                // base path for execution envs (default: ~/multica_workspaces)
	KeepEnvAfterTask               bool                  // preserve env after task for debugging
	HealthPort                     int                   // local HTTP port for health checks (default: 19514)
	MaxConcurrentTasks             int                   // max tasks running in parallel (default: 20)
	GCEnabled                      bool                  // enable periodic workspace garbage collection (default: true)
	GCInterval                     time.Duration         // how often the GC loop runs (default: 1h)
	GCTTL                          time.Duration         // clean dirs whose issue is done/canceled and updated_at < now()-TTL (default: 24h)
	GCOrphanTTL                    time.Duration         // clean orphan dirs with no meta older than this (default: 72h). Dirs whose issue returned 404 are cleaned immediately.
	PollInterval                   time.Duration
	HeartbeatInterval              time.Duration
	AgentTimeout                   time.Duration
	CodexSemanticInactivityTimeout time.Duration
	ClaudeArgs                     []string
	CodexArgs                      []string
}

// Overrides allows CLI flags to override environment variables and defaults.
// Zero values are ignored and the env/default value is used instead.
type Overrides struct {
	ServerURL                      string
	WorkspacesRoot                 string
	PollInterval                   time.Duration
	HeartbeatInterval              time.Duration
	AgentTimeout                   time.Duration
	CodexSemanticInactivityTimeout time.Duration
	MaxConcurrentTasks             int
	DaemonID                       string
	DeviceName                     string
	RuntimeName                    string
	Profile                        string // profile name (empty = default)
	HealthPort                     int    // health check port (0 = use default)
}

// LoadConfig builds the daemon configuration from environment variables
// and optional CLI flag overrides.
func LoadConfig(overrides Overrides) (Config, error) {
	// Server URL: override > env > default
	rawServerURL := envOrDefault("MULTICA_SERVER_URL", DefaultServerURL)
	if overrides.ServerURL != "" {
		rawServerURL = overrides.ServerURL
	}
	serverBaseURL, err := NormalizeServerBaseURL(rawServerURL)
	if err != nil {
		return Config{}, err
	}

	// Probe available agent CLIs. Resolve bare command names to canonical
	// executable paths and skip recursive wrappers under ~/.multica/hooks.
	agents := map[string]AgentEntry{}
	probe := func(provider, envVar, defaultCmd, modelEnv string) {
		cmd := envOrDefault(envVar, defaultCmd)
		path, err := resolveAgentExecutablePath(cmd)
		if err != nil {
			return
		}
		agents[provider] = AgentEntry{Path: path, Model: strings.TrimSpace(os.Getenv(modelEnv))}
	}
	probe("claude", "MULTICA_CLAUDE_PATH", "claude", "MULTICA_CLAUDE_MODEL")
	probe("codex", "MULTICA_CODEX_PATH", "codex", "MULTICA_CODEX_MODEL")
	probe("opencode", "MULTICA_OPENCODE_PATH", "opencode", "MULTICA_OPENCODE_MODEL")
	probe("openclaw", "MULTICA_OPENCLAW_PATH", "openclaw", "MULTICA_OPENCLAW_MODEL")
	probe("hermes", "MULTICA_HERMES_PATH", "hermes", "MULTICA_HERMES_MODEL")
	probe("gemini", "MULTICA_GEMINI_PATH", "gemini", "MULTICA_GEMINI_MODEL")
	probe("pi", "MULTICA_PI_PATH", "pi", "MULTICA_PI_MODEL")
	probe("cursor", "MULTICA_CURSOR_PATH", "cursor-agent", "MULTICA_CURSOR_MODEL")
	probe("copilot", "MULTICA_COPILOT_PATH", "copilot", "MULTICA_COPILOT_MODEL")
	probe("kimi", "MULTICA_KIMI_PATH", "kimi", "MULTICA_KIMI_MODEL")
	probe("kiro", "MULTICA_KIRO_PATH", "kiro-cli", "MULTICA_KIRO_MODEL")
	probe("agy", "MULTICA_AGY_PATH", "agy", "MULTICA_AGY_MODEL")

	// Codex Desktop bundles its CLI outside PATH. Only use the bundle fallback
	// for the default command; an explicit MULTICA_CODEX_PATH remains authoritative.
	if _, ok := agents["codex"]; !ok && strings.TrimSpace(os.Getenv("MULTICA_CODEX_PATH")) == "" {
		for _, candidate := range codexDesktopAppBundlePaths() {
			if isExecutableFile(candidate) {
				agents["codex"] = AgentEntry{Path: canonicalExecutablePath(candidate), Model: strings.TrimSpace(os.Getenv("MULTICA_CODEX_MODEL"))}
				break
			}
		}
	}
	if len(agents) == 0 {
		return Config{}, fmt.Errorf("no agent CLI found: install claude, codex, copilot, opencode, openclaw, hermes, gemini, pi, cursor-agent, kimi, kiro-cli, or agy and ensure it is on PATH")
	}

	claudeArgs, err := shellArgsFromEnv("MULTICA_CLAUDE_ARGS")
	if err != nil {
		return Config{}, err
	}
	codexArgs, err := shellArgsFromEnv("MULTICA_CODEX_ARGS")
	if err != nil {
		return Config{}, err
	}

	// Host info
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "local-machine"
	}

	// Durations: override > env > default
	pollInterval, err := durationFromEnv("MULTICA_DAEMON_POLL_INTERVAL", DefaultPollInterval)
	if err != nil {
		return Config{}, err
	}
	if overrides.PollInterval > 0 {
		pollInterval = overrides.PollInterval
	}

	heartbeatInterval, err := durationFromEnv("MULTICA_DAEMON_HEARTBEAT_INTERVAL", DefaultHeartbeatInterval)
	if err != nil {
		return Config{}, err
	}
	if overrides.HeartbeatInterval > 0 {
		heartbeatInterval = overrides.HeartbeatInterval
	}

	agentTimeout, err := durationFromEnv("MULTICA_AGENT_TIMEOUT", DefaultAgentTimeout)
	if err != nil {
		return Config{}, err
	}
	if overrides.AgentTimeout > 0 {
		agentTimeout = overrides.AgentTimeout
	}

	codexSemanticInactivityTimeout, err := durationFromEnv("MULTICA_CODEX_SEMANTIC_INACTIVITY_TIMEOUT", DefaultCodexSemanticInactivityTimeout)
	if err != nil {
		return Config{}, err
	}
	if overrides.CodexSemanticInactivityTimeout > 0 {
		codexSemanticInactivityTimeout = overrides.CodexSemanticInactivityTimeout
	}

	maxConcurrentTasks, err := intFromEnv("MULTICA_DAEMON_MAX_CONCURRENT_TASKS", DefaultMaxConcurrentTasks)
	if err != nil {
		return Config{}, err
	}
	if overrides.MaxConcurrentTasks > 0 {
		maxConcurrentTasks = overrides.MaxConcurrentTasks
	}

	// Profile
	profile := overrides.Profile

	// daemon_id resolution: override > env > persistent UUID on disk.
	// The persistent UUID is written once to `<profile-dir>/daemon.id` and
	// then reused forever so hostname drift (.local suffix, system rename,
	// mDNS state, profile switch) no longer mints a new runtime identity.
	// Callers may still pin a specific id via MULTICA_DAEMON_ID or the
	// override field (e.g. for tests or embedded environments).
	daemonID := strings.TrimSpace(os.Getenv("MULTICA_DAEMON_ID"))
	if overrides.DaemonID != "" {
		daemonID = overrides.DaemonID
	}
	if daemonID == "" {
		persisted, err := EnsureDaemonID(profile)
		if err != nil {
			return Config{}, fmt.Errorf("ensure daemon id: %w", err)
		}
		daemonID = persisted
	}
	// Historical daemon_ids derived from the current hostname/profile. The
	// server uses these at register time to merge any pre-UUID runtime rows
	// for this machine into the new UUID-keyed row and delete the stale ones.
	legacyDaemonIDs := LegacyDaemonIDs(host, profile)
	// Pre-change (#1220) daemon identity was stored per profile, which means
	// the same machine could end up with multiple leftover daemon.id files
	// — e.g. ~/.multica/daemon.id (default) plus ~/.multica/profiles/<x>/
	// daemon.id. Surface those UUIDs so the server can merge their runtime
	// rows into the canonical machine UUID. Fatal-free: a broken profiles
	// dir shouldn't block startup.
	if uuids, err := LegacyDaemonUUIDs(); err == nil {
		legacyDaemonIDs = append(legacyDaemonIDs, uuids...)
	}
	// Strip anything that collides with the resolved daemon_id (e.g. when
	// the user explicitly pins MULTICA_DAEMON_ID=<hostname>, or when the
	// canonical id was itself promoted from a pre-change profile file).
	legacyDaemonIDs = filterLegacyIDs(legacyDaemonIDs, daemonID)

	deviceName := envOrDefault("MULTICA_DAEMON_DEVICE_NAME", host)
	if overrides.DeviceName != "" {
		deviceName = overrides.DeviceName
	}

	runtimeName := envOrDefault("MULTICA_AGENT_RUNTIME_NAME", DefaultRuntimeName)
	if overrides.RuntimeName != "" {
		runtimeName = overrides.RuntimeName
	}

	// Workspaces root: override > env > default (~/multica_workspaces or ~/multica_workspaces_<profile>)
	workspacesRoot := strings.TrimSpace(os.Getenv("MULTICA_WORKSPACES_ROOT"))
	if overrides.WorkspacesRoot != "" {
		workspacesRoot = overrides.WorkspacesRoot
	}
	if workspacesRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home directory: %w (set MULTICA_WORKSPACES_ROOT to override)", err)
		}
		if profile != "" {
			workspacesRoot = filepath.Join(home, "multica_workspaces_"+profile)
		} else {
			workspacesRoot = filepath.Join(home, "multica_workspaces")
		}
	}
	workspacesRoot, err = filepath.Abs(workspacesRoot)
	if err != nil {
		return Config{}, fmt.Errorf("resolve absolute workspaces root: %w", err)
	}

	// Health port: override > default
	healthPort := DefaultHealthPort
	if overrides.HealthPort > 0 {
		healthPort = overrides.HealthPort
	}

	// Keep env after task: env > default (false)
	keepEnv := os.Getenv("MULTICA_KEEP_ENV_AFTER_TASK") == "true" || os.Getenv("MULTICA_KEEP_ENV_AFTER_TASK") == "1"

	// GC config: env > defaults
	gcEnabled := true
	if v := os.Getenv("MULTICA_GC_ENABLED"); v == "false" || v == "0" {
		gcEnabled = false
	}
	gcInterval, err := durationFromEnv("MULTICA_GC_INTERVAL", DefaultGCInterval)
	if err != nil {
		return Config{}, err
	}
	gcTTL, err := durationFromEnv("MULTICA_GC_TTL", DefaultGCTTL)
	if err != nil {
		return Config{}, err
	}
	gcOrphanTTL, err := durationFromEnv("MULTICA_GC_ORPHAN_TTL", DefaultGCOrphanTTL)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ServerBaseURL:                  serverBaseURL,
		DaemonID:                       daemonID,
		LegacyDaemonIDs:                legacyDaemonIDs,
		DeviceName:                     deviceName,
		RuntimeName:                    runtimeName,
		Profile:                        profile,
		Agents:                         agents,
		WorkspacesRoot:                 workspacesRoot,
		KeepEnvAfterTask:               keepEnv,
		GCEnabled:                      gcEnabled,
		GCInterval:                     gcInterval,
		GCTTL:                          gcTTL,
		GCOrphanTTL:                    gcOrphanTTL,
		HealthPort:                     healthPort,
		MaxConcurrentTasks:             maxConcurrentTasks,
		PollInterval:                   pollInterval,
		HeartbeatInterval:              heartbeatInterval,
		AgentTimeout:                   agentTimeout,
		CodexSemanticInactivityTimeout: codexSemanticInactivityTimeout,
		ClaudeArgs:                     claudeArgs,
		CodexArgs:                      codexArgs,
	}, nil
}

// NormalizeServerBaseURL converts a WebSocket or HTTP URL to a base HTTP URL.
func NormalizeServerBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid MULTICA_SERVER_URL: %w", err)
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("MULTICA_SERVER_URL must use ws, wss, http, or https")
	}
	if u.Path == "/ws" {
		u.Path = ""
	}
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func shellArgsFromEnv(name string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, nil
	}
	args, err := shellwords.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	return args, nil
}

// resolveAgentExecutablePath pins a discovered executable to a canonical path.
// A stale ~/.multica/hooks wrapper can recursively invoke itself, so bare
// command discovery skips that directory when another executable is available.
func resolveAgentExecutablePath(cmd string) (string, error) {
	resolved, err := exec.LookPath(cmd)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(cmd, "/\\") {
		return canonicalExecutablePath(resolved), nil
	}
	if isInMulticaHooksDir(resolved) {
		if unshadowed, err := lookPathExcludingMulticaHooks(cmd); err == nil {
			return unshadowed, nil
		}
		return "", exec.ErrNotFound
	}
	return canonicalExecutablePath(resolved), nil
}

func lookPathExcludingMulticaHooks(cmd string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		if isMulticaHooksDir(dir) {
			continue
		}
		for _, name := range executableCandidateNames(cmd) {
			candidate := filepath.Join(dir, name)
			if isExecutableFile(candidate) {
				return canonicalExecutablePath(candidate), nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func isInMulticaHooksDir(path string) bool {
	return path != "" && isMulticaHooksDir(filepath.Dir(path))
}

func isMulticaHooksDir(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	return samePathDir(dir, filepath.Join(home, ".multica", "hooks"))
}

func samePathDir(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	if realA, err := filepath.EvalSymlinks(absA); err == nil {
		absA = realA
	}
	if realB, err := filepath.EvalSymlinks(absB); err == nil {
		absB = realB
	}
	cleanA, cleanB := filepath.Clean(absA), filepath.Clean(absB)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}

func canonicalExecutablePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

func executableCandidateNames(cmd string) []string {
	if runtime.GOOS != "windows" || filepath.Ext(cmd) != "" {
		return []string{cmd}
	}
	names := []string{cmd}
	for _, ext := range filepath.SplitList(strings.TrimSpace(os.Getenv("PATHEXT"))) {
		if ext = strings.TrimSpace(ext); ext != "" {
			names = append(names, cmd+ext)
		}
	}
	return names
}

// OpenAI relocated the bundled CLI from Codex.app to ChatGPT.app. Keep the
// legacy locations after the new paths for older installations.
var codexDesktopAppBundlePaths = func() []string {
	paths := []string{
		"/Applications/ChatGPT.app/Contents/Resources/codex",
		"/Applications/Codex.app/Contents/Resources/codex",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
		)
	}
	return paths
}
