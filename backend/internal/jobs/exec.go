package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

func runCommandStreaming(ctx context.Context, cmd *exec.Cmd, onLine func(string)) error {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	var readErr error
	var readErrMu sync.Mutex
	var wg sync.WaitGroup

	consume := func(reader io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if onLine != nil {
				onLine(scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
			readErrMu.Lock()
			if readErr == nil {
				readErr = err
			}
			readErrMu.Unlock()
		}
	}

	wg.Add(2)
	go consume(stdoutPipe)
	go consume(stderrPipe)

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("command failed: %w", waitErr)
	}

	if readErr != nil {
		return fmt.Errorf("read command output: %w", readErr)
	}

	return nil
}
