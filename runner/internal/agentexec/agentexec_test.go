package agentexec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPrepareCreatesFreshAttemptDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	t.Setenv(rootEnv, root)
	t.Setenv(uidEnv, strconv.Itoa(os.Getuid()))
	t.Setenv(gidEnv, strconv.Itoa(os.Getgid()))

	stale := filepath.Join(root, "workspace", "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideManagedDirs := filepath.Join(root, "contestant-created.txt")
	if err := os.WriteFile(outsideManagedDirs, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := Prepare()
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	t.Cleanup(paths.Cleanup)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale workspace survived: %v", err)
	}
	if _, err := os.Stat(outsideManagedDirs); !os.IsNotExist(err) {
		t.Fatalf("contestant-created state survived: %v", err)
	}
	for _, path := range []string{paths.Home, paths.Temp} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Errorf("prepared directory %s: info=%v err=%v", path, info, err)
		}
	}
}

func TestPrepareSeedsCodexAuth(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	seed := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(seed, []byte(`{"auth_mode":"apikey"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rootEnv, root)
	t.Setenv(uidEnv, strconv.Itoa(os.Getuid()))
	t.Setenv(gidEnv, strconv.Itoa(os.Getgid()))
	t.Setenv(authFileEnv, seed)

	paths, err := Prepare()
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	t.Cleanup(paths.Cleanup)

	raw, err := os.ReadFile(filepath.Join(paths.Home, ".codex", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"auth_mode":"apikey"}` {
		t.Fatalf("auth = %q", raw)
	}
}

func TestCommandUsesPrivateIdentityAndEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	t.Setenv(rootEnv, root)
	t.Setenv(uidEnv, "1234")
	t.Setenv(gidEnv, "5678")
	t.Setenv("EVAL_SHOULD_NOT_LEAK", "secret")

	cmd, err := Command(context.Background(), "/workspace", []string{"CODEX_HOME=/wrong"}, "codex", "exec")
	if err != nil {
		t.Fatalf("Command error: %v", err)
	}
	if cmd.Path != "setpriv" && !strings.HasSuffix(cmd.Path, "/setpriv") {
		t.Fatalf("command path = %q", cmd.Path)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"--reuid=1234", "--regid=5678", "--no-new-privs", "--bounding-set=-all", "codex exec",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("agent command does not have a private process group")
	}

	env := strings.Join(cmd.Env, "\n")
	for _, want := range []string{
		"PWD=/workspace",
		"HOME=" + filepath.Join(root, "home"),
		"CODEX_HOME=" + filepath.Join(root, "home", ".codex"),
	} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q", want)
		}
	}
	if strings.Contains(env, "EVAL_SHOULD_NOT_LEAK") || strings.Contains(env, "CODEX_HOME=/wrong") {
		t.Errorf("environment leaked evaluator state:\n%s", env)
	}
}

func TestParseProcessStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		wantUID    int
		wantZombie bool
		wantOK     bool
	}{
		{name: "running", status: "State:\tS (sleeping)\nUid:\t1000\t1000\t1000\t1000\n", wantUID: 1000, wantOK: true},
		{name: "zombie", status: "State:\tZ (zombie)\nUid:\t1001\t1001\t1001\t1001\n", wantUID: 1001, wantZombie: true, wantOK: true},
		{name: "missing uid", status: "State:\tR (running)\n", wantUID: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uid, zombie, ok := parseProcessStatus(tt.status)
			if uid != tt.wantUID || zombie != tt.wantZombie || ok != tt.wantOK {
				t.Fatalf("parseProcessStatus = (%d, %t, %t), want (%d, %t, %t)", uid, zombie, ok, tt.wantUID, tt.wantZombie, tt.wantOK)
			}
		})
	}
}

func TestConfiguredRootRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{name: "relative", root: "relative"},
		{name: "filesystem root", root: string(filepath.Separator)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(rootEnv, tt.root)
			if _, _, err := configuredRoot(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
