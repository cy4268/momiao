// momiao-bootstrap only creates the initial platform SUPER_ADMIN. It is never
// linked into an HTTP route or invoked by portal startup or the migration CLI.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cy4268/momiao/internal/nativeself"
	"github.com/cy4268/momiao/internal/platform"
)

// Set only from the approved immutable release build, using -ldflags -X.
// A development binary with no explicit build identity fails closed.
var releaseBuild string

const nativeSourceTree = "3c15a618fa7a528c06da92de7dbf2f2c843a9162"

type options struct {
	environment   string
	target        int64
	username      string
	expectedEmpty bool
}
type deployment struct {
	Environment      string `json:"environment"`
	Database         string `json:"database"`
	NativeSourceTree string `json:"native_source_tree"`
	NativeSocket     string `json:"native_socket"`
	ReleaseBuild     string `json:"release_build"`
}
type credential struct {
	AccessToken string `json:"access_token"`
	SessionID   string `json:"session_id"`
}

func main() {
	tty := false
	if stat, err := os.Stdin.Stat(); err == nil {
		tty = stat.Mode()&os.ModeCharDevice != 0
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdin, os.Stdout, tty, releaseBuild))
}
func parseOptions(args []string) (options, error) {
	var o options
	var id string
	fs := flag.NewFlagSet("momiao-bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.environment, "environment", "", "")
	fs.StringVar(&id, "newapi-user-id", "", "")
	fs.StringVar(&o.username, "expected-username", "", "")
	fs.BoolVar(&o.expectedEmpty, "expected-empty", false, "")
	seen := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			key := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)[0]
			if seen[key] {
				return o, platform.ErrBootstrapInvalid
			}
			seen[key] = true
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return o, platform.ErrBootstrapInvalid
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id || !o.expectedEmpty || !validEnvironment(o.environment) || !safeLabel(o.username) {
		return o, platform.ErrBootstrapInvalid
	}
	o.target = n
	return o, nil
}
func validEnvironment(v string) bool {
	return v == "DEVELOPMENT" || v == "STAGING" || v == "PRODUCTION"
}
func safeLabel(v string) bool {
	return len(v) > 0 && len(v) <= 128 && utf8.ValidString(v) && strings.IndexFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0
}
func privateFile(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) > 16384 || len(raw) == 0 {
		return nil, errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	return raw, nil
}
func privateJSON(path string, target any) error {
	raw, err := privateFile(path)
	if err != nil {
		return err
	}
	if !utf8.Valid(raw) {
		return errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if !nativeself.UniqueJSON(d, 0) {
		return errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return errors.New("BOOTSTRAP_PRIVATE_INPUT_INVALID")
	}
	return nil
}
func confirmProduction(in io.Reader, tty bool, target int64) bool {
	if !tty {
		return false
	}
	text, err := bufio.NewReader(io.LimitReader(in, 256)).ReadString('\n')
	return err == nil && strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r") == fmt.Sprintf("PRODUCTION BOOTSTRAP_SUPER_ADMIN %d", target)
}
func run(ctx context.Context, args []string, getenv func(string) string, in io.Reader, out io.Writer, tty bool, build string) int {
	err := execute(ctx, args, getenv, in, out, tty, build)
	if err != nil {
		fmt.Fprintln(out, err.Error())
		return 1
	}
	return 0
}
func execute(ctx context.Context, args []string, getenv func(string) string, in io.Reader, out io.Writer, tty bool, build string) error {
	o, err := parseOptions(args)
	if err != nil {
		return err
	}
	var d deployment
	if err = privateJSON(getenv("MOMIAO_BOOTSTRAP_DEPLOYMENT_FILE"), &d); err != nil {
		return err
	}
	if !validEnvironment(d.Environment) || o.environment != d.Environment || !safeLabel(d.Database) {
		return errors.New("BOOTSTRAP_ENVIRONMENT_MISMATCH")
	}
	if d.NativeSourceTree != nativeSourceTree || !filepath.IsAbs(d.NativeSocket) {
		return errors.New("BOOTSTRAP_SOURCE_UNVERIFIED")
	}
	if !safeLabel(build) || d.ReleaseBuild != build {
		return errors.New("BOOTSTRAP_RELEASE_MISMATCH")
	}
	// The independently mounted, operator-controlled manifest binds environment,
	// database, reviewed native tree and private socket to this immutable build.
	// It is deployment provenance, not a claim that HTTP self attests its binary.
	fmt.Fprintf(out, "BOOTSTRAP_PREVIEW environment=%s target=%d username=%s release=%s database=%s\n", o.environment, o.target, o.username, build, d.Database)
	if o.environment == "PRODUCTION" {
		fmt.Fprintf(out, "Type: PRODUCTION BOOTSTRAP_SUPER_ADMIN %d\n", o.target)
		if !confirmProduction(in, tty, o.target) {
			return errors.New("BOOTSTRAP_CONFIRMATION_REQUIRED")
		}
	}
	var c credential
	if err = privateJSON(getenv("MOMIAO_BOOTSTRAP_CREDENTIAL_FILE"), &c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	transport := nativeself.NewTransport(d.NativeSocket)
	defer transport.CloseIdleConnections()
	if err = nativeself.Verify(ctx, transport, o.target, o.username, c.AccessToken, c.SessionID); err != nil {
		return err
	}
	raw, err := privateFile(getenv("MOMIAO_BOOTSTRAP_DSN_FILE"))
	if err != nil {
		return err
	}
	s, err := platform.Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		return errors.New("BOOTSTRAP_DATABASE_UNAVAILABLE")
	}
	defer s.Close()
	if !s.BootstrapDatabaseMatches(ctx, d.Database) {
		return errors.New("BOOTSTRAP_DATABASE_MISMATCH")
	}
	receipt, err := s.Bootstrap(ctx, platform.BootstrapInput{Environment: o.environment, UserID: o.target, Username: o.username, ReleaseBuild: build, ExpectedEmpty: o.expectedEmpty})
	if err != nil {
		return err
	}
	if err = json.NewEncoder(out).Encode(receipt); err != nil {
		return platform.ErrBootstrapOutcomeUnknown
	}
	return nil
}
