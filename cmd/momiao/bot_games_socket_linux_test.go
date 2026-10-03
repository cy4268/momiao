package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func botGamesSocketTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func botGamesStaleSocket(t *testing.T, path string) os.FileInfo {
	t.Helper()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	l.SetUnlinkOnClose(false)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatal("real stale socket missing")
	}
	return info
}

func botGamesSocketConnects(t *testing.T, path string) {
	t.Helper()
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("listener not reachable: %v", err)
	}
	_ = c.Close()
}

func TestBotGamesSocketInactiveRecovery(t *testing.T) {
	path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
	before := botGamesStaleSocket(t, path)
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		_ = c.Close()
		t.Fatal("fixture endpoint is still active")
	}
	l, err := openBotGamesListener(path)
	if err != nil {
		t.Fatalf("inactive B endpoint did not recover: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	after, err := os.Lstat(path)
	if err != nil || after.Mode().Perm() != 0600 {
		t.Fatal("stale endpoint not replaced by private listener")
	}
	if os.SameFile(before, after) {
		t.Log("filesystem reused the freed inode number; recovery is checked by inactive-before / reachable-after behavior")
	}
	botGamesSocketConnects(t, path)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("closed B endpoint survived")
	}
	names, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(names) != 0 {
		t.Fatal("B listener created persistent lock artifacts")
	}
}

func TestBotGamesSocketRefusesUnownedOrAmbiguousEntries(t *testing.T) {
	for _, kind := range []string{"active", "regular", "symlink", "dangling_symlink", "foreign_owner", "ambiguous_unixgram"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
			switch kind {
			case "active":
				l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
			case "regular":
				if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink", "dangling_symlink":
				target := filepath.Join(filepath.Dir(path), "target")
				if kind == "symlink" {
					if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "foreign_owner":
				if os.Geteuid() != 0 {
					t.Skip("foreign UID fixture requires Linux root")
				}
				botGamesStaleSocket(t, path)
				if err := os.Chown(path, 65531, 65531); err != nil {
					t.Fatal(err)
				}
			case "ambiguous_unixgram":
				l, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if l, err := openBotGamesListener(path); err == nil {
				_ = l.Close()
				t.Fatal("unowned/active/ambiguous endpoint was replaced")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("rejected endpoint changed")
			}
			if kind == "active" {
				botGamesSocketConnects(t, path)
			}
			if kind == "regular" {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != "preserve" {
					t.Fatal("ordinary file changed")
				}
			}
		})
	}
}

func TestBotGamesSocketClosePreservesReplacement(t *testing.T) {
	path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
	l, err := openBotGamesListener(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("closing old B listener removed a replacement endpoint")
	}
	botGamesSocketConnects(t, path)
}

func TestBotGamesSocketRechecksIdentityBeforeRemoval(t *testing.T) {
	path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
	old := botGamesStaleSocket(t, path)
	// Keep the old inode allocated under another test-only name, then replace
	// the inspected pathname with a distinct, live endpoint before removal.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if err := removeBotGamesSocket(path, old); err == nil {
		t.Fatal("stale pre-probe identity authorized removal of its replacement")
	}
	botGamesSocketConnects(t, path)
}

func TestBotGamesSocketRequiresPrivateOwnedDirectory(t *testing.T) {
	for _, kind := range []string{"public", "symlink", "foreign_owner"} {
		t.Run(kind, func(t *testing.T) {
			dir := botGamesSocketTestDir(t)
			switch kind {
			case "public":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(botGamesSocketTestDir(t), "linked")
				if err := os.Symlink(dir, target); err != nil {
					t.Fatal(err)
				}
				dir = target
			case "foreign_owner":
				if os.Geteuid() != 0 {
					t.Skip("foreign UID fixture requires Linux root")
				}
				if err := os.Chown(dir, 65531, 65531); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, "dice.sock")
			if l, err := openBotGamesListener(path); err == nil {
				_ = l.Close()
				t.Fatal("nonprivate/unowned directory accepted")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("rejected directory changed")
			}
		})
	}
}

func TestBotGamesSocketCrashAndConcurrentStarts(t *testing.T) {
	if path := os.Getenv("MOMIAO_B_SOCKET_TEST_CHILD"); path != "" {
		l, err := openBotGamesListener(path)
		if err != nil {
			_, _ = os.Stdout.WriteString("REFUSED\n")
			return
		}
		defer l.Close()
		_, _ = os.Stdout.WriteString("READY\n")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	type child struct {
		command *exec.Cmd
		stop    io.WriteCloser
		ready   <-chan string
	}
	start := func(path string) child {
		t.Helper()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, "-test.run=^TestBotGamesSocketCrashAndConcurrentStarts$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "MOMIAO_B_SOCKET_TEST_CHILD="+path)
		stop, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		ready := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(out)
			if scanner.Scan() {
				ready <- strings.TrimSpace(scanner.Text())
			} else {
				ready <- "NO_READY"
			}
			_, _ = io.Copy(io.Discard, out)
		}()
		t.Cleanup(func() { _ = stop.Close(); _ = cmd.Process.Kill() })
		return child{cmd, stop, ready}
	}
	read := func(c child) string {
		t.Helper()
		select {
		case result := <-c.ready:
			return result
		case <-time.After(5 * time.Second):
			t.Fatal("child startup timed out")
			return ""
		}
	}
	t.Run("SIGKILL_leaves_inode_then_recovers", func(t *testing.T) {
		path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
		c := start(path)
		if read(c) != "READY" {
			t.Fatal("child listener failed to start")
		}
		botGamesSocketConnects(t, path)
		if err := c.command.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		if err := c.command.Wait(); err == nil {
			t.Fatal("SIGKILL unexpectedly succeeded cleanly")
		}
		if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSocket == 0 {
			t.Fatal("SIGKILL did not retain a socket inode")
		}
		l, err := openBotGamesListener(path)
		if err != nil {
			t.Fatalf("killed B listener did not recover: %v", err)
		}
		defer l.Close()
		botGamesSocketConnects(t, path)
	})
	t.Run("eight_processes_one_listener", func(t *testing.T) {
		path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
		botGamesStaleSocket(t, path)
		children := make([]child, 8)
		for i := range children {
			children[i] = start(path)
		}
		winners := 0
		for _, c := range children {
			switch read(c) {
			case "READY":
				winners++
			case "REFUSED":
			default:
				t.Fatal("invalid child result")
			}
		}
		if winners != 1 {
			t.Fatalf("simultaneous stale recovery started %d listeners, want one", winners)
		}
		botGamesSocketConnects(t, path)
		for _, c := range children {
			_ = c.stop.Close()
			if err := c.command.Wait(); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("eight_goroutines_one_listener", func(t *testing.T) {
		path := filepath.Join(botGamesSocketTestDir(t), "dice.sock")
		botGamesStaleSocket(t, path)
		start := make(chan struct{})
		results := make(chan net.Listener, 8)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; l, _ := openBotGamesListener(path); results <- l }()
		}
		close(start)
		wg.Wait()
		close(results)
		winners := 0
		for l := range results {
			if l != nil {
				winners++
				defer l.Close()
			}
		}
		if winners != 1 {
			t.Fatalf("simultaneous stale recovery started %d listeners, want one", winners)
		}
		botGamesSocketConnects(t, path)
	})
}
