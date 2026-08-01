package vm

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

type commandCall struct {
	name string
	args []string
	env  []string
}

type fakeCommandRunner struct {
	mu      sync.Mutex
	calls   []commandCall
	results []commandResult
}

type commandResult struct {
	output []byte
	err    error
}

func (r *fakeCommandRunner) Run(_ context.Context, name string, args []string, env []string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...), env: append([]string(nil), env...)})
	if len(r.results) == 0 {
		return nil, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result.output, result.err
}

func TestCreateVMRunsCloneCustomizeAndStart(t *testing.T) {
	runner := &fakeCommandRunner{}
	service := NewVMService(slog.Default(), VMServiceConfig{BaseDomain: "base", SFUHost: "10.0.0.1", GRPCPort: "5001"})
	service.runner = runner

	if err := service.CreateVM(context.Background(), "room", 6000, 6002); err != nil {
		t.Fatalf("CreateVM returned error: %v", err)
	}
	if service.vms["room"] == nil || service.vms["room"].domainName != "flicker-room" {
		t.Fatalf("VM instance was not recorded: %#v", service.vms["room"])
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 3 {
		t.Fatalf("command count = %d, want 3: %#v", len(runner.calls), runner.calls)
	}
	if runner.calls[0].name != "virt-clone" || runner.calls[1].name != "sudo" || runner.calls[2].name != "virsh" {
		t.Fatalf("unexpected command sequence: %#v", runner.calls)
	}
	if runner.calls[2].args[len(runner.calls[2].args)-2] != "start" || runner.calls[2].args[len(runner.calls[2].args)-1] != "flicker-room" {
		t.Fatalf("unexpected start command: %#v", runner.calls[2])
	}
}

func TestDestroyVMRunsCleanupAndRemovesInstance(t *testing.T) {
	runner := &fakeCommandRunner{
		results: []commandResult{
			{},
			{output: []byte("file disk vda /tmp/flicker-test-disk.qcow2\n")},
			{},
			{},
		},
	}
	service := NewVMService(slog.Default(), VMServiceConfig{})
	service.runner = runner
	service.vms["room"] = &vmInstance{domainName: "flicker-room"}

	if err := service.DestroyVM("room"); err != nil {
		t.Fatalf("DestroyVM returned error: %v", err)
	}
	if _, ok := service.vms["room"]; ok {
		t.Fatal("VM instance remained after destruction")
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 4 {
		t.Fatalf("command count = %d, want 4: %#v", len(runner.calls), runner.calls)
	}
	if runner.calls[0].args[2] != "destroy" || runner.calls[1].args[2] != "domblklist" || runner.calls[2].args[2] != "snapshot-list" || runner.calls[3].args[2] != "undefine" {
		t.Fatalf("unexpected cleanup commands: %#v", runner.calls)
	}
}
