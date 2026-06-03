package harness

import "fmt"

// FileKind describes the type of a filesystem object.
type FileKind string

const (
	FileKindFile      FileKind = "file"
	FileKindDirectory FileKind = "directory"
	FileKindSymlink   FileKind = "symlink"
)

// FileErrorCode is a stable, backend-independent error code for filesystem operations.
type FileErrorCode string

const (
	FileErrorAborted         FileErrorCode = "aborted"
	FileErrorNotFound        FileErrorCode = "not_found"
	FileErrorPermissionDenied FileErrorCode = "permission_denied"
	FileErrorNotDirectory    FileErrorCode = "not_directory"
	FileErrorIsDirectory     FileErrorCode = "is_directory"
	FileErrorInvalid         FileErrorCode = "invalid"
	FileErrorNotSupported    FileErrorCode = "not_supported"
	FileErrorUnknown         FileErrorCode = "unknown"
)

// FileError is returned by FileSystem operations.
type FileError struct {
	Code    FileErrorCode
	Message string
	Path    string
	Cause   error
}

func (e *FileError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("FileError(%s): %s (path: %s)", e.Code, e.Message, e.Path)
	}
	return fmt.Sprintf("FileError(%s): %s", e.Code, e.Message)
}

func (e *FileError) Unwrap() error { return e.Cause }

func NewFileError(code FileErrorCode, message string, path string, cause error) *FileError {
	return &FileError{Code: code, Message: message, Path: path, Cause: cause}
}

// ExecutionErrorCode is a stable error code for execution environments.
type ExecutionErrorCode string

const (
	ExecErrorAborted        ExecutionErrorCode = "aborted"
	ExecErrorTimeout        ExecutionErrorCode = "timeout"
	ExecErrorShellUnavailable ExecutionErrorCode = "shell_unavailable"
	ExecErrorSpawn          ExecutionErrorCode = "spawn_error"
	ExecErrorCallback       ExecutionErrorCode = "callback_error"
	ExecErrorUnknown        ExecutionErrorCode = "unknown"
)

// ExecutionError is returned by Shell.Exec.
type ExecutionError struct {
	Code    ExecutionErrorCode
	Message string
	Cause   error
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("ExecutionError(%s): %s", e.Code, e.Message)
}

func (e *ExecutionError) Unwrap() error { return e.Cause }

func NewExecutionError(code ExecutionErrorCode, message string, cause error) *ExecutionError {
	return &ExecutionError{Code: code, Message: message, Cause: cause}
}

// CompactionErrorCode is a stable error code for compaction operations.
type CompactionErrorCode string

const (
	CompactionErrorAborted           CompactionErrorCode = "aborted"
	CompactionErrorSummarizationFailed CompactionErrorCode = "summarization_failed"
	CompactionErrorInvalidSession    CompactionErrorCode = "invalid_session"
	CompactionErrorUnknown           CompactionErrorCode = "unknown"
)

// CompactionError is returned by compaction helpers.
type CompactionError struct {
	Code    CompactionErrorCode
	Message string
	Cause   error
}

func (e *CompactionError) Error() string {
	return fmt.Sprintf("CompactionError(%s): %s", e.Code, e.Message)
}

func (e *CompactionError) Unwrap() error { return e.Cause }

func NewCompactionError(code CompactionErrorCode, message string, cause error) *CompactionError {
	return &CompactionError{Code: code, Message: message, Cause: cause}
}

// BranchSummaryErrorCode is a stable error code for branch summarization.
type BranchSummaryErrorCode string

const (
	BranchSummaryErrorAborted           BranchSummaryErrorCode = "aborted"
	BranchSummaryErrorSummarizationFailed BranchSummaryErrorCode = "summarization_failed"
	BranchSummaryErrorInvalidSession    BranchSummaryErrorCode = "invalid_session"
)

// BranchSummaryError is returned by branch summarization helpers.
type BranchSummaryError struct {
	Code    BranchSummaryErrorCode
	Message string
	Cause   error
}

func (e *BranchSummaryError) Error() string {
	return fmt.Sprintf("BranchSummaryError(%s): %s", e.Code, e.Message)
}

func (e *BranchSummaryError) Unwrap() error { return e.Cause }

func NewBranchSummaryError(code BranchSummaryErrorCode, message string, cause error) *BranchSummaryError {
	return &BranchSummaryError{Code: code, Message: message, Cause: cause}
}

// SessionErrorCode is a stable error code for session operations.
type SessionErrorCode string

const (
	SessionErrorNotFound          SessionErrorCode = "not_found"
	SessionErrorInvalidSession    SessionErrorCode = "invalid_session"
	SessionErrorInvalidEntry      SessionErrorCode = "invalid_entry"
	SessionErrorInvalidForkTarget SessionErrorCode = "invalid_fork_target"
	SessionErrorStorage           SessionErrorCode = "storage"
	SessionErrorUnknown           SessionErrorCode = "unknown"
)

// SessionError is thrown by session storage, repositories, and tree operations.
type SessionError struct {
	Code    SessionErrorCode
	Message string
	Cause   error
}

func (e *SessionError) Error() string {
	return fmt.Sprintf("SessionError(%s): %s", e.Code, e.Message)
}

func (e *SessionError) Unwrap() error { return e.Cause }

func NewSessionError(code SessionErrorCode, message string, cause error) *SessionError {
	return &SessionError{Code: code, Message: message, Cause: cause}
}

// AgentHarnessErrorCode is a stable top-level error code for the harness.
type AgentHarnessErrorCode string

const (
	HarnessErrorBusy          AgentHarnessErrorCode = "busy"
	HarnessErrorInvalidState  AgentHarnessErrorCode = "invalid_state"
	HarnessErrorInvalidArg    AgentHarnessErrorCode = "invalid_argument"
	HarnessErrorSession       AgentHarnessErrorCode = "session"
	HarnessErrorHook          AgentHarnessErrorCode = "hook"
	HarnessErrorAuth          AgentHarnessErrorCode = "auth"
	HarnessErrorCompaction    AgentHarnessErrorCode = "compaction"
	HarnessErrorBranchSummary AgentHarnessErrorCode = "branch_summary"
	HarnessErrorUnknown       AgentHarnessErrorCode = "unknown"
)

// AgentHarnessError is the public error type for harness failures.
type AgentHarnessError struct {
	Code    AgentHarnessErrorCode
	Message string
	Cause   error
}

func (e *AgentHarnessError) Error() string {
	return fmt.Sprintf("AgentHarnessError(%s): %s", e.Code, e.Message)
}

func (e *AgentHarnessError) Unwrap() error { return e.Cause }

func NewAgentHarnessError(code AgentHarnessErrorCode, message string, cause error) *AgentHarnessError {
	return &AgentHarnessError{Code: code, Message: message, Cause: cause}
}

// NormalizeHarnessError converts unknown errors into AgentHarnessError.
func NormalizeHarnessError(err error, fallback AgentHarnessErrorCode) *AgentHarnessError {
	if he, ok := err.(*AgentHarnessError); ok {
		return he
	}
	cause := ToError(err)
	if se, ok := cause.(*SessionError); ok {
		return NewAgentHarnessError(HarnessErrorSession, se.Message, se)
	}
	if ce, ok := cause.(*CompactionError); ok {
		return NewAgentHarnessError(HarnessErrorCompaction, ce.Message, ce)
	}
	if be, ok := cause.(*BranchSummaryError); ok {
		return NewAgentHarnessError(HarnessErrorBranchSummary, be.Message, be)
	}
	return NewAgentHarnessError(fallback, cause.Error(), cause)
}

// ToError normalizes unknown values into Error instances.
func ToError(err any) error {
	if e, ok := err.(error); ok {
		return e
	}
	if s, ok := err.(string); ok {
		return fmt.Errorf("%s", s)
	}
	return fmt.Errorf("%v", err)
}
