// Package scorerexec prepares and executes post-run checks as a dedicated
// unprivileged identity.
package scorerexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	rootEnv = "EVAL_SCORER_ROOT"
	uidEnv  = "EVAL_SCORER_UID"
	gidEnv  = "EVAL_SCORER_GID"
)

// Paths are private to one scoring attempt. The contestant identity cannot
// traverse Root in the Compose runtime.
type Paths struct {
	Root      string
	Workspace string
	Task      string
	Home      string
	Temp      string
	Cache     string
	uid       int
	gid       int
}

// Prepare creates an empty scorer root. A missing EVAL_SCORER_ROOT keeps the
// bare-metal development behavior unchanged.
func Prepare() (*Paths, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return nil, nil
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("%s must be an absolute path", rootEnv)
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) {
		return nil, fmt.Errorf("%s must not be the filesystem root", rootEnv)
	}

	uid, gid, err := IDs()
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(root); err != nil {
		return nil, fmt.Errorf("clean scorer root: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create scorer root: %w", err)
	}

	p := &Paths{
		Root:      root,
		Workspace: filepath.Join(root, "workspace"),
		Task:      filepath.Join(root, "task"),
		Home:      filepath.Join(root, "home"),
		Temp:      filepath.Join(root, "tmp"),
		Cache:     filepath.Join(root, "cache"),
		uid:       uid,
		gid:       gid,
	}
	for _, path := range []string{p.Task, p.Home, p.Temp, p.Cache} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create scorer directory %s: %w", path, err)
		}
	}
	return p, nil
}

// OwnTree transfers the completed scoring tree to the scorer identity.
func (p *Paths) OwnTree() error {
	return filepath.WalkDir(p.Root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(path, p.uid, p.gid); err != nil {
			return fmt.Errorf("chown scorer path %s: %w", path, err)
		}
		return nil
	})
}

// Cleanup removes the private scoring copy.
func (p *Paths) Cleanup() error {
	if p == nil {
		return nil
	}
	if err := os.RemoveAll(p.Root); err != nil {
		return fmt.Errorf("clean scorer root: %w", err)
	}
	return nil
}

// Command builds a check process with no evaluator credentials or proxy
// configuration. Compose blocks this UID from non-loopback network traffic.
func (p *Paths) Command(ctx context.Context, workdir, name string, args ...string) *exec.Cmd {
	command := name
	commandArgs := args
	if p.uid != os.Geteuid() || p.gid != os.Getegid() {
		command = "setpriv"
		commandArgs = append([]string{
			"--reuid=" + strconv.Itoa(p.uid),
			"--regid=" + strconv.Itoa(p.gid),
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
	cmd.Env = p.environment(workdir)
	return cmd
}

// IDs returns the required scorer identity.
func IDs() (int, int, error) {
	uidText := os.Getenv(uidEnv)
	gidText := os.Getenv(gidEnv)
	if uidText == "" || gidText == "" {
		return 0, 0, fmt.Errorf("%s and %s are required when %s is set", uidEnv, gidEnv, rootEnv)
	}
	uid, err := parseID(uidEnv, uidText)
	if err != nil {
		return 0, 0, err
	}
	gid, err := parseID(gidEnv, gidText)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func parseID(name, value string) (int, error) {
	parsed, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return int(parsed), nil
}

func (p *Paths) environment(workdir string) []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
	}
	moduleCache := os.Getenv("GOMODCACHE")
	if moduleCache == "" {
		moduleCache = filepath.Join(p.Home, "go", "pkg", "mod")
	}
	goProxy := os.Getenv("GOPROXY")
	if goProxy == "" {
		goProxy = "off"
	}
	toolchain := os.Getenv("GOTOOLCHAIN")
	if toolchain == "" {
		toolchain = "local"
	}

	return []string{
		"PATH=" + path,
		"PWD=" + workdir,
		"OLDPWD=" + workdir,
		"HOME=" + p.Home,
		"TMPDIR=" + p.Temp,
		"GOCACHE=" + p.Cache,
		"GOMODCACHE=" + moduleCache,
		"GOPROXY=" + goProxy,
		"GOTOOLCHAIN=" + toolchain,
		"GOENV=off",
		"NO_PROXY=127.0.0.1,localhost,::1",
		"no_proxy=127.0.0.1,localhost,::1",
		"USER=scorer",
		"LOGNAME=scorer",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=" + workdir,
	}
}
