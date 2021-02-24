package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/cli/safeexec"
	"github.com/github/hub/v2/ui"
)

// Cmd is a project-wide struct that represents a command to be run in the console.
type Cmd struct {
	Name   string
	Args   []string
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

func (cmd Cmd) String() string {
	args := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		if strings.ContainsRune(a, '"') {
			args[i] = fmt.Sprintf(`'%s'`, a)
		} else if a == "" || strings.ContainsRune(a, '\'') || strings.ContainsRune(a, ' ') {
			args[i] = fmt.Sprintf(`"%s"`, a)
		} else {
			args[i] = a
		}
	}
	return fmt.Sprintf("%s %s", cmd.Name, strings.Join(args, " "))
}

// WithArg returns the current argument
func (cmd *Cmd) WithArg(arg string) *Cmd {
	cmd.Args = append(cmd.Args, arg)

	return cmd
}

func (cmd *Cmd) WithArgs(args ...string) *Cmd {
	for _, arg := range args {
		cmd.WithArg(arg)
	}

	return cmd
}

func (cmd *Cmd) makeExecCmd() (*exec.Cmd, error) {
	binary, err := safeexec.LookPath(cmd.Name)
	if err != nil {
		return nil, err
	}

	return exec.Command(binary, cmd.Args...), nil
}

func (cmd *Cmd) Output() (string, error) {
	verboseLog(cmd)
	c, err := cmd.makeExecCmd()
	if err != nil {
		return "", err
	}
	c.Stderr = cmd.Stderr
	output, err := c.Output()

	return string(output), err
}

func (cmd *Cmd) CombinedOutput() (string, error) {
	verboseLog(cmd)
	c, err := cmd.makeExecCmd()
	if err != nil {
		return "", err
	}
	output, err := c.CombinedOutput()
	return string(output), err
}

func (cmd *Cmd) Success() bool {
	verboseLog(cmd)
	c, err := cmd.makeExecCmd()
	return err == nil && c.Run() == nil
}

// Run runs command with `Exec` on platforms except Windows
// which only supports `Spawn`
func (cmd *Cmd) Run() error {
	if isWindows() {
		return cmd.Spawn()
	}
	return cmd.Exec()
}

func isWindows() bool {
	return runtime.GOOS == "windows" || detectWSL()
}

var detectedWSL bool
var detectedWSLContents string

// https://github.com/Microsoft/WSL/issues/423#issuecomment-221627364
func detectWSL() bool {
	if !detectedWSL {
		b := make([]byte, 1024)
		f, err := os.Open("/proc/version")
		if err == nil {
			_, _ = f.Read(b)
			_ = f.Close()
			detectedWSLContents = string(b)
		}
		detectedWSL = true
	}
	return strings.Contains(detectedWSLContents, "Microsoft")
}

// Spawn runs command with spawn(3)
func (cmd *Cmd) Spawn() error {
	verboseLog(cmd)
	c, err := cmd.makeExecCmd()
	if err != nil {
		return err
	}
	c.Stdin = cmd.Stdin
	c.Stdout = cmd.Stdout
	c.Stderr = cmd.Stderr

	return c.Run()
}

// Exec runs command with exec(3)
// Note that Windows doesn't support exec(3): http://golang.org/src/pkg/syscall/exec_windows.go#L339
func (cmd *Cmd) Exec() error {
	verboseLog(cmd)

	binary, err := safeexec.LookPath(cmd.Name)
	if err != nil {
		return &exec.Error{
			Name: cmd.Name,
			Err:  fmt.Errorf("command not found"),
		}
	}

	args := []string{binary}
	args = append(args, cmd.Args...)

	return syscall.Exec(binary, args, os.Environ())
}

func New(name string) *Cmd {
	return &Cmd{
		Name:   name,
		Args:   []string{},
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func NewWithArray(cmd []string) *Cmd {
	return &Cmd{Name: cmd[0], Args: cmd[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}
}

func verboseLog(cmd *Cmd) {
	if os.Getenv("HUB_VERBOSE") != "" {
		msg := fmt.Sprintf("$ %s", cmd.String())
		if ui.IsTerminal(os.Stderr) {
			// bizarre: color `35` does not display at all in PowerShell (it does in Windows Terminal), but
			// using `35;1` works around that
			color := "35;1"
			msg = fmt.Sprintf("\033[%sm%s\033[m", color, msg)
		}
		ui.Errorln(msg)
	}
}
