package jobs

import (
	"context"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunCommandStreamingSlowConsumer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", `i=1; while [ $i -le 500 ]; do echo "out-$i"; echo "err-$i" 1>&2; i=$((i+1)); done`)

	var lineCount int64
	err := runCommandStreaming(ctx, cmd, func(_ string) {
		atomic.AddInt64(&lineCount, 1)
		time.Sleep(1 * time.Millisecond)
	})
	if err != nil {
		t.Fatalf("runCommandStreaming returned error: %v", err)
	}

	if got := atomic.LoadInt64(&lineCount); got < 1000 {
		t.Fatalf("unexpected line count: got=%d want>=1000", got)
	}
}
