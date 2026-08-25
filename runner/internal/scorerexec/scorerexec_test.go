package scorerexec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPrepareCreatesFreshPrivateDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scorer")
	stale := filepath.Join(root, "stale")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rootEnv, root)
	t.Setenv(uidEnv, strconv.Itoa(os.Getuid()))
	t.Setenv(gidEnv, strconv.Itoa(os.Getgid()))

	paths, err := Prepare()
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}
	t.Cleanup(func() {
		if err := paths.Cleanup(); err != nil {
			t.Errorf("Cleanup error: %v", err)
		}
	})

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale scorer state survived: %v", err)
	}
	for _, path := range []string{paths.Task, paths.Home, paths.Temp, paths.Cache} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("prepared directory %s: info=%v err=%v", path, info, err)
		}
	}
}

func TestCommandDropsPrivilegesAndUsesMinimalEnvironment(t *testing.T) {
	t.Setenv(rootEnv, "/score")
	t.Setenv(uidEnv, "1234")
	t.Setenv(gidEnv, "5678")
	t.Setenv("OPENAI_API_KEY", "secret")
	t.Setenv("HTTP_PROXY", "http://egress:3128")
	t.Setenv("EVAL_PRIVATE_VALUE", "secret")

	paths := &Paths{
		Root: "/score", Home: "/score/home", Temp: "/score/tmp", Cache: "/score/cache",
		uid: 1234, gid: 5678,
	}
	cmd := paths.Command(context.Background(), "/score/workspace", "bash", "/score/task/checks.sh")
	if cmd.Path != "setpriv" && !strings.HasSuffix(cmd.Path, "/setpriv") {
		t.Fatalf("command path = %q", cmd.Path)
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"--reuid=1234", "--regid=5678", "--no-new-privs",
		"--bounding-set=-all", "bash /score/task/checks.sh",
	} {
		if !strings.Contains(joinedArgs, want) {
			t.Errorf("args %q missing %q", joinedArgs, want)
		}
	}

	env := strings.Join(cmd.Env, "\n")
	for _, forbidden := range []string{"OPENAI_API_KEY", "HTTP_PROXY", "EVAL_PRIVATE_VALUE"} {
		if strings.Contains(env, forbidden) {
			t.Errorf("environment leaked %s:\n%s", forbidden, env)
		}
	}
	for _, want := range []string{
		"HOME=/score/home", "TMPDIR=/score/tmp", "GOCACHE=/score/cache",
		"GOPROXY=off", "GOTOOLCHAIN=local", "GIT_CONFIG_VALUE_0=/score/workspace",
		"NO_PROXY=127.0.0.1,localhost,::1", "no_proxy=127.0.0.1,localhost,::1",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q:\n%s", want, env)
		}
	}
}

func TestPrepareRequiresCompleteIdentity(t *testing.T) {
	t.Setenv(rootEnv, filepath.Join(t.TempDir(), "scorer"))
	t.Setenv(uidEnv, "1001")
	if _, err := Prepare(); err == nil {
		t.Fatal("expected missing gid error")
	}
}
