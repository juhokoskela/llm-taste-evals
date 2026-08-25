// Package agentexec prepares and executes contestant agent processes with a
// filesystem identity separate from the evaluator runner.
package agentexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const (
	rootEnv     = "EVAL_AGENT_ROOT"
	uidEnv      = "EVAL_AGENT_UID"
	gidEnv      = "EVAL_AGENT_GID"
	authFileEnv = "EVAL_CODEX_AUTH_FILE"
)

// Paths are the writable directories visible to one contestant attempt.
type Paths struct {
	Root      string
	Workspace string
	Home      string
	Temp      string
}

// Prepare removes state from the previous attempt and creates fresh private
// directories for the next contestant. A missing EVAL_AGENT_ROOT keeps the
// bare-metal development behavior unchanged.
func Prepare() (*Paths, error) {
	root, ok, err := configuredRoot()
	if err != nil || !ok {
		return nil, err
	}
	uid, gid, enabled, err := IDs()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, fmt.Errorf("%s requires %s and %s", rootEnv, uidEnv, gidEnv)
	}
	if err := os.RemoveAll(root); err != nil {
		return nil, fmt.Errorf("clean agent root: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create agent root: %w", err)
	}

	p := &Paths{
		Root:      root,
		Workspace: filepath.Join(root, "workspace"),
		Home:      filepath.Join(root, "home"),
		Temp:      filepath.Join(root, "tmp"),
	}
	if err := os.MkdirAll(p.Home, 0o700); err != nil {
		return nil, fmt.Errorf("create agent home: %w", err)
	}
	if err := os.MkdirAll(p.Temp, 0o700); err != nil {
		return nil, fmt.Errorf("create agent temp: %w", err)
	}
	if err := seedCodexAuth(p.Home); err != nil {
		return nil, err
	}
	if err := os.Chown(root, uid, gid); err != nil {
		return nil, fmt.Errorf("chown agent root: %w", err)
	}
	for _, path := range []string{p.Home, p.Temp} {
		if err := OwnTree(path); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Cleanup removes contestant-controlled state after it has been archived.
func (p *Paths) Cleanup() {
	if p == nil {
		return
	}
	_ = os.RemoveAll(p.Root)
}

// OwnTree transfers a prepared workspace or home tree to the contestant UID.
func OwnTree(root string) error {
	uid, gid, enabled, err := IDs()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
		return nil
	})
}

// Command builds a contestant process. In Compose it uses setpriv to drop the
// runner's root privileges and prevent the child from regaining them.
func Command(ctx context.Context, workdir string, extraEnv []string, name string, args ...string) (*exec.Cmd, error) {
	uid, gid, enabled, err := IDs()
	if err != nil {
		return nil, err
	}

	command := name
	commandArgs := args
	if enabled {
		command = "setpriv"
		commandArgs = append([]string{
			"--reuid=" + strconv.Itoa(uid),
			"--regid=" + strconv.Itoa(gid),
			"--clear-groups",
			"--no-new-privs",
			"--bounding-set=-all",
			"--inh-caps=-all",
			"--ambient-caps=-all",
			"--",
			name,
		}, args...)
	}

	cmd := exec.CommandContext(ctx, command, commandArgs...)
	cmd.Dir = workdir
	cmd.Env = environment(workdir, extraEnv)
	if enabled {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd, nil
}

// KillProcessGroup terminates descendants left behind by an agent CLI. Agent
// commands run in their own process group in isolated runs.
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill agent process group: %w", err)
	}
	return nil
}

// KillAll terminates detached processes owned by the configured contestant
// identity. It is a no-op outside Linux isolation and when the configured UID
// matches the runner.
func KillAll() error {
	uid, _, enabled, err := IDs()
	if err != nil || !enabled {
		return err
	}
	if runtime.GOOS != "linux" || uid == os.Geteuid() {
		return nil
	}

	for range 8 {
		found, err := killPass(uid)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		runtime.Gosched()
	}
	return fmt.Errorf("contestant processes survived cleanup")
}

func killPass(uid int) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, fmt.Errorf("list processes: %w", err)
	}
	found := false
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, fmt.Errorf("read process %d status: %w", pid, err)
		}
		processUID, zombie, ok := parseProcessStatus(string(raw))
		if !ok || zombie || processUID != uid {
			continue
		}
		found = true
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return false, fmt.Errorf("kill contestant process %d: %w", pid, err)
		}
	}
	return found, nil
}

func parseProcessStatus(status string) (uid int, zombie, ok bool) {
	uid = -1
	for line := range strings.SplitSeq(status, "\n") {
		switch {
		case strings.HasPrefix(line, "Uid:"):
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, false, false
			}
			parsed, err := strconv.Atoi(fields[1])
			if err != nil {
				return 0, false, false
			}
			uid = parsed
		case strings.HasPrefix(line, "State:"):
			fields := strings.Fields(line)
			zombie = len(fields) >= 2 && fields[1] == "Z"
		}
	}
	return uid, zombie, uid >= 0
}

// IDs returns the configured contestant identity. Both values must be set or
// both must be absent.
func IDs() (uid, gid int, enabled bool, err error) {
	uidText := os.Getenv(uidEnv)
	gidText := os.Getenv(gidEnv)
	if uidText == "" && gidText == "" {
		return 0, 0, false, nil
	}
	if uidText == "" || gidText == "" {
		return 0, 0, false, fmt.Errorf("%s and %s must be set together", uidEnv, gidEnv)
	}
	uid64, err := strconv.ParseUint(uidText, 10, 31)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse %s: %w", uidEnv, err)
	}
	gid64, err := strconv.ParseUint(gidText, 10, 31)
	if err != nil {
		return 0, 0, false, fmt.Errorf("parse %s: %w", gidEnv, err)
	}
	return int(uid64), int(gid64), true, nil
}

func configuredRoot() (string, bool, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", false, nil
	}
	if !filepath.IsAbs(root) {
		return "", false, fmt.Errorf("%s must be an absolute path", rootEnv)
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) {
		return "", false, fmt.Errorf("%s must not be the filesystem root", rootEnv)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", false, fmt.Errorf("create agent root: %w", err)
	}
	return root, true, nil
}

func seedCodexAuth(home string) error {
	source := os.Getenv(authFileEnv)
	if source == "" {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open codex auth seed: %w", err)
	}
	defer in.Close()

	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create codex home: %w", err)
	}
	out, err := os.OpenFile(filepath.Join(dir, "auth.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create codex auth: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy codex auth: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close codex auth: %w", err)
	}
	return nil
}

func environment(workdir string, extra []string) []string {
	values := make(map[string]string)
	order := make([]string, 0, len(os.Environ())+len(extra)+10)
	add := func(entry string) {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			return
		}
		if _, exists := values[name]; !exists {
			order = append(order, name)
		}
		values[name] = value
	}
	for _, entry := range os.Environ() {
		add(entry)
	}
	for _, entry := range extra {
		add(entry)
	}

	values["PWD"] = workdir
	values["OLDPWD"] = workdir
	if root, ok, _ := configuredRoot(); ok {
		home := filepath.Join(root, "home")
		values["HOME"] = home
		values["TMPDIR"] = filepath.Join(root, "tmp")
		values["CODEX_HOME"] = filepath.Join(home, ".codex")
		values["CLAUDE_CONFIG_DIR"] = filepath.Join(home, ".claude")
		values["XDG_CONFIG_HOME"] = filepath.Join(home, ".config")
		values["XDG_CACHE_HOME"] = filepath.Join(home, ".cache")
		values["XDG_DATA_HOME"] = filepath.Join(home, ".local", "share")
		values["USER"] = "agent"
		values["LOGNAME"] = "agent"
	}

	env := make([]string, 0, len(order))
	for _, name := range order {
		if strings.HasPrefix(name, "EVAL_") {
			continue
		}
		env = append(env, name+"="+values[name])
	}
	for _, name := range []string{
		"PWD", "OLDPWD", "HOME", "TMPDIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR",
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "USER", "LOGNAME",
	} {
		if value, ok := values[name]; ok && !containsEnv(env, name) {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func containsEnv(env []string, name string) bool {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
