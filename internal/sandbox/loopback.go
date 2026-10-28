package sandbox

// LoopbackBackend is the Phase 3B controlled spike backend. It provides
// process-level isolation for the canary: a fresh per-instance temp
// workspace, a sanitized environment that never carries provider secrets,
// a bounded timeout, and real cancel/pause/resume/destroy. It is a fixture
// for deterministic journeys — not a production isolation boundary.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const loopbackOutputLimit = 64 << 10

type LoopbackBackend struct {
	mu        sync.Mutex
	workRoot  string
	timeout   time.Duration
	dirs      map[string]string
	procs     map[string]*os.Process
	done      map[string]chan struct{}
	controls  map[string]*loopbackControl
	destroyed []string
}

func NewLoopbackBackend(workRoot string, timeout time.Duration) (*LoopbackBackend, error) {
	if workRoot == "" {
		workRoot = os.TempDir()
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(workRoot)
	if err != nil {
		return nil, err
	}
	return &LoopbackBackend{
		workRoot: root, timeout: timeout,
		dirs: make(map[string]string), procs: make(map[string]*os.Process), done: make(map[string]chan struct{}), controls: make(map[string]*loopbackControl),
	}, nil
}

func (backend *LoopbackBackend) Create(ctx context.Context, request CreateRequest) (Instance, error) {
	instanceID := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if containsString(backend.destroyed, instanceID) {
		return Instance{}, fmt.Errorf("loopback: instance %s destroyed", instanceID)
	}
	if _, exists := backend.dirs[instanceID]; exists {
		return Instance{InstanceID: instanceID}, nil
	}
	workdir := filepath.Join(backend.workRoot, instanceID)
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		return Instance{}, err
	}
	if err := os.MkdirAll(filepath.Join(workdir, "tmp"), 0o700); err != nil {
		return Instance{}, err
	}
	if err := os.Chmod(workdir, 0o700); err != nil {
		return Instance{}, err
	}
	backend.dirs[instanceID] = workdir
	return Instance{InstanceID: instanceID}, nil
}

func (backend *LoopbackBackend) Exec(ctx context.Context, request ExecRequest) (ExecResult, error) {
	backend.mu.Lock()
	workdir, ok := backend.dirs[request.InstanceID]
	if !ok {
		backend.mu.Unlock()
		return ExecResult{}, fmt.Errorf("loopback: unknown instance %s", request.InstanceID)
	}
	if backend.destroyed != nil && containsString(backend.destroyed, request.InstanceID) {
		backend.mu.Unlock()
		return ExecResult{}, fmt.Errorf("loopback: instance %s destroyed", request.InstanceID)
	}
	if backend.procs[request.InstanceID] != nil {
		backend.mu.Unlock()
		return ExecResult{}, fmt.Errorf("loopback: instance %s already executing", request.InstanceID)
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = backend.timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The live group leader performs group cleanup itself. The Go parent only
	// signals its os.Process handle; it never sends a signal to a cached PGID
	// after Wait may have reaped the leader and allowed that number to be reused.
	// Commands remain trusted fixtures: setsid escapes and deliberate attacks on
	// the supervisor are outside this backend's non-security isolation boundary.
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		backend.mu.Unlock()
		return ExecResult{}, err
	}
	defer statusRead.Close()
	resumeRead, resumeWrite, err := os.Pipe()
	if err != nil {
		statusWrite.Close()
		backend.mu.Unlock()
		return ExecResult{}, err
	}
	defer resumeRead.Close()
	controlRead, controlWrite, err := os.Pipe()
	if err != nil {
		statusWrite.Close()
		resumeWrite.Close()
		backend.mu.Unlock()
		return ExecResult{}, err
	}
	defer controlWrite.Close()
	control := &loopbackControl{requests: controlWrite, resumed: make(chan loopbackResumeAck, 1)}
	command := exec.CommandContext(runCtx, "sh", "-c", loopbackSupervisor, "loopback-supervisor", request.Command)
	command.Dir = workdir
	command.Env = sanitizedEnv(workdir)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.ExtraFiles = []*os.File{statusWrite, resumeWrite, controlRead, controlWrite}
	command.Cancel = func() error {
		control.canceled.Store(true)
		return stopLoopbackProcess(command.Process)
	}
	var output outputTail
	command.Stdout = &output
	command.Stderr = &output

	if err := command.Start(); err != nil {
		statusWrite.Close()
		resumeWrite.Close()
		controlRead.Close()
		backend.mu.Unlock()
		return ExecResult{}, err
	}
	statusWrite.Close()
	resumeWrite.Close()
	controlRead.Close()
	control.process = command.Process
	ackReaderDone := make(chan struct{})
	defer close(ackReaderDone)
	go func() {
		defer close(control.resumed)
		reader := bufio.NewReader(resumeRead)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			sequence, parseErr := parseLoopbackResumeAck(line)
			select {
			case control.resumed <- loopbackResumeAck{sequence, parseErr}:
			case <-ackReaderDone:
				return
			}
			if parseErr != nil {
				return
			}
		}
	}()
	status := bufio.NewReader(statusRead)
	ready, readyErr := status.ReadString('\n')
	if readyErr != nil || ready != "ready\n" {
		backend.mu.Unlock()
		waited := make(chan struct{})
		go func() { _ = command.Wait(); close(waited) }()
		_ = stopLoopbackProcess(command.Process)
		<-waited
		return ExecResult{}, fmt.Errorf("loopback: supervisor failed to initialize")
	}
	// Hold the same lock across startup and publication: Cancel/Destroy cannot
	// miss a started command, and a second Exec cannot overwrite its ownership.
	backend.procs[request.InstanceID] = command.Process
	backend.controls[request.InstanceID] = control
	completed := make(chan struct{})
	backend.done[request.InstanceID] = completed
	backend.mu.Unlock()
	defer func() {
		backend.mu.Lock()
		delete(backend.procs, request.InstanceID)
		delete(backend.done, request.InstanceID)
		delete(backend.controls, request.InstanceID)
		close(completed)
		backend.mu.Unlock()
	}()

	runErr := command.Wait()
	exitCode := -1 // Explicit cancellation has no normal child exit status.
	line, statusErr := status.ReadString('\n')
	if statusErr == nil {
		if err := validateLoopbackCompletion(runErr); err != nil {
			return ExecResult{}, err
		}
		exitCode, err = strconv.Atoi(strings.TrimSpace(line))
		if err != nil || exitCode < 0 || exitCode > 255 {
			return ExecResult{}, fmt.Errorf("loopback: invalid supervisor exit status")
		}
	} else if runCtx.Err() != nil {
		exitCode = 124
	} else {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != -1 {
			return ExecResult{}, fmt.Errorf("loopback: supervisor exited without command status: %w", statusErr)
		}
	}
	digest := output.Digest()
	return ExecResult{ExitCode: exitCode, OutputDigest: digest}, nil
}

func validateLoopbackCompletion(runErr error) error {
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() && status.Signal() == syscall.SIGKILL {
			return nil
		}
	}
	return fmt.Errorf("loopback: supervisor did not confirm process-group cleanup: %v", runErr)
}

func (backend *LoopbackBackend) Cancel(ctx context.Context, instanceID string) error {
	backend.mu.Lock()
	process := backend.procs[instanceID]
	if control := backend.controls[instanceID]; control != nil {
		control.canceled.Store(true)
	}
	backend.mu.Unlock()
	return stopLoopbackProcess(process)
}

func (backend *LoopbackBackend) Pause(ctx context.Context, instanceID string) error {
	control := backend.control(instanceID)
	if control == nil {
		return nil
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.pending != 0 || control.canceled.Load() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if control.sequence == ^uint64(0) {
		return fmt.Errorf("loopback: pause sequence exhausted")
	}
	sequence := control.sequence + 1
	deadline := time.Now().Add(2 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := control.requests.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer control.requests.SetWriteDeadline(time.Time{})
	message := fmt.Sprintf("pause %d\n", sequence)
	n, err := control.requests.Write([]byte(message))
	if err != nil {
		return err
	}
	if n != len(message) {
		return fmt.Errorf("loopback: incomplete pause request")
	}
	control.sequence = sequence
	control.pending = sequence
	return nil
}

func (backend *LoopbackBackend) Resume(ctx context.Context, instanceID string) error {
	control := backend.control(instanceID)
	if control == nil {
		return nil
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.pending == 0 || control.canceled.Load() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := control.process.Signal(syscall.SIGCONT); err != nil {
			return err
		}
		select {
		case ack, ok := <-control.resumed:
			if !ok {
				return fmt.Errorf("loopback: supervisor ended before resume acknowledgment")
			}
			if ack.err != nil {
				return ack.err
			}
			matched, err := matchLoopbackResumeAck(control.pending, ack.sequence)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
			control.pending = 0
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("loopback: supervisor did not acknowledge resume")
		case <-tick.C:
		}
	}
}

func (backend *LoopbackBackend) Destroy(ctx context.Context, instanceID string) error {
	backend.mu.Lock()
	if !containsString(backend.destroyed, instanceID) {
		backend.destroyed = append(backend.destroyed, instanceID)
	}
	done := backend.done[instanceID]
	process := backend.procs[instanceID]
	if control := backend.controls[instanceID]; control != nil {
		control.canceled.Store(true)
	}
	backend.mu.Unlock()
	err := stopLoopbackProcess(process)
	if err != nil {
		return err
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if workdir, ok := backend.dirs[instanceID]; ok {
		if err := os.RemoveAll(workdir); err != nil {
			return err
		}
		delete(backend.dirs, instanceID)
	}
	return nil
}

// Install cleanup before starting the child and announce readiness on a pipe
// the child does not inherit. The worker reports completion into the same pipe
// as pause requests, so the supervisor handles ordered messages in its main
// loop rather than losing repeated signals inside a previous signal handler.
// Exit status is reported before self-killing the owned group, so background
// descendants cannot retain output pipes after completion.
// $$ is expanded by the live supervisor itself, never by the user command.
const loopbackSupervisor = `
trap 'trap "" TERM; kill -KILL -$$' TERM
printf 'ready\n' >&3
(
  sh -c "$1" </dev/null 6>&-
  code=$?
  printf 'exit %s\n' "$code" >&6
) 3>&- 4>&- 5>&- &
exec 6>&-
while IFS=' ' read -r action value extra <&5; do
  case "$value" in ''|*[!0-9]*) break ;; esac
  [ -z "$extra" ] || break
  case "$action" in
    pause)
      [ "$value" != 0 ] || break
      kill -STOP -$$
      kill -CONT -$$
      printf 'resumed %s\n' "$value" >&4
      ;;
    exit)
      [ "$value" -le 255 ] || break
      printf '%s\n' "$value" >&3
      kill -KILL -$$
      exit 125
      ;;
    *) break ;;
  esac
done
# Malformed input/EOF is not a successful command completion.
kill -KILL -$$
exit 125
`

func stopLoopbackProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	// TERM may race a queued pause request. Continue retrying through the live
	// process handle until Wait acknowledges exit, so an earlier CONT cannot be
	// consumed just before the pause trap stops the group. Never signal a PGID.
	deadline := time.Now().Add(2 * time.Second)
	for {
		for _, signal := range []os.Signal{syscall.SIGTERM, syscall.SIGCONT} {
			err := process.Signal(signal)
			if errors.Is(err, os.ErrProcessDone) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("loopback: supervisor did not acknowledge cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (backend *LoopbackBackend) InspectCapabilities(ctx context.Context) (Capabilities, error) {
	return Capabilities{Supported: []string{
		"create", "exec", "cancel", "pause", "resume", "destroy",
	}}, nil
}

type loopbackControl struct {
	mu       sync.Mutex
	process  *os.Process
	requests *os.File
	sequence uint64
	pending  uint64
	canceled atomic.Bool
	resumed  chan loopbackResumeAck
}

type loopbackResumeAck struct {
	sequence uint64
	err      error
}

func parseLoopbackResumeAck(line string) (uint64, error) {
	if !strings.HasSuffix(line, "\n") {
		return 0, fmt.Errorf("loopback: incomplete resume acknowledgment")
	}
	value, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), "resumed ")
	sequence, err := strconv.ParseUint(value, 10, 64)
	if !ok || err != nil || sequence == 0 || strconv.FormatUint(sequence, 10) != value {
		return 0, fmt.Errorf("loopback: invalid resume acknowledgment")
	}
	return sequence, nil
}

func matchLoopbackResumeAck(pending, ack uint64) (bool, error) {
	if pending == 0 || ack == 0 || ack > pending {
		return false, fmt.Errorf("loopback: unexpected resume acknowledgment")
	}
	return ack == pending, nil // An old acknowledgment cannot complete a later pause.
}

func (backend *LoopbackBackend) control(instanceID string) *loopbackControl {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.controls[instanceID]
}

func sanitizedEnv(workdir string) []string {
	return []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + workdir,
		"TMPDIR=" + filepath.Join(workdir, "tmp"),
		"LANG=C",
		"LC_ALL=C",
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
