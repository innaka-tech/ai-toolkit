// Package apperr defines aitk's typed errors: a stable code, an exit code, and a fix hint.
package apperr

import "fmt"

// Exit codes (docs/spec/cli.md).
const (
	ExitOK       = 0
	ExitRuntime  = 1
	ExitUsage    = 2
	ExitGate     = 3
	ExitConflict = 4
)

// Error is returned by every command that fails in an expected way.
type Error struct {
	Code    string // E_*
	Message string
	Fix     string // exact command or action that resolves the error
	Exit    int
}

func (e *Error) Error() string { return e.Message }

// New builds an Error.
func New(code string, exit int, fix, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Fix: fix, Exit: exit}
}

// Common constructors.
func NotAProject() *Error {
	return New("E_NOT_A_PROJECT", ExitGate, "aitk init", "not inside an aitk project (no aitk.toml found in the git repository)")
}

func NeedsMigration() *Error {
	return New("E_NEEDS_MIGRATION", ExitGate, "aitk migrate --dry-run", "this is a v1 ai-toolkit project; migrate it to v2 first")
}

func NoActiveTask() *Error {
	return New("E_NO_ACTIVE_TASK", ExitGate, `aitk task start <id>   (or: aitk task new "<title>")`, "no active task in this worktree")
}

func TaskNotFound(id string) *Error {
	return New("E_TASK_NOT_FOUND", ExitUsage, "aitk task list", "task %q not found", id)
}

func Usage(format string, args ...any) *Error {
	return New("E_USAGE", ExitUsage, "aitk <command> --help", format, args...)
}
