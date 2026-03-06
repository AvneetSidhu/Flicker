package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

type VMServiceConfig struct {
	BaseDomain string // libvirt domain name to clone from (e.g. "flicker-node")
	SFUHost    string // Tailscale IP — vmagent sends RTP here and connects for gRPC
	GRPCPort   string
}

type vmInstance struct {
	domainName string
}

type VMService struct {
	mu     sync.Mutex
	vms    map[string]*vmInstance
	cfg    VMServiceConfig
	logger *slog.Logger
}

func NewVMService(logger *slog.Logger, cfg VMServiceConfig) *VMService {
	return &VMService{
		vms:    make(map[string]*vmInstance),
		cfg:    cfg,
		logger: logger,
	}
}

// CreateVM clones the base libvirt domain, injects per-room config directly
// into the cloned disk via virt-customize, then starts the VM with virsh.
func (s *VMService) CreateVM(ctx context.Context, roomID string, audioPort int, videoPort int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	domainName := fmt.Sprintf("flicker-%s", roomID)

	// Clone the base domain (creates a stopped VM with a new QCOW2 overlay).
	if out, err := exec.CommandContext(ctx, "virt-clone",
		"--original", s.cfg.BaseDomain,
		"--name", domainName,
		"--auto-clone",
	).CombinedOutput(); err != nil {
		return fmt.Errorf("virt-clone failed: %w: %s", err, out)
	}
	s.logger.Info("VM cloned", "roomID", roomID, "domain", domainName)

	// Write /etc/vmagent.env into the cloned disk while it is still offline.
	envContent := fmt.Sprintf(
		"SFU_FORWARD_ADDR=%s\nSFU_GRPC_ADDR=%s:%s\nROOM_ID=%s\nAUDIO_PORT=%d\nVIDEO_PORT=%d\nLISTEN_PORT=5000\n",
		s.cfg.SFUHost, s.cfg.SFUHost, s.cfg.GRPCPort, roomID, audioPort, videoPort,
	)

	envFile, err := os.CreateTemp("", fmt.Sprintf("vmagent-%s-*.env", roomID))
	if err != nil {
		s.cleanupDomain(domainName)
		return fmt.Errorf("failed to create temp env file: %w", err)
	}
	if _, err := envFile.WriteString(envContent); err != nil {
		envFile.Close()
		os.Remove(envFile.Name())
		s.cleanupDomain(domainName)
		return fmt.Errorf("failed to write env file: %w", err)
	}
	envFile.Close()

	if out, err := exec.CommandContext(ctx, "virt-customize",
		"-d", domainName,
		"--upload", fmt.Sprintf("%s:/etc/vmagent.env", envFile.Name()),
	).CombinedOutput(); err != nil {
		os.Remove(envFile.Name())
		s.cleanupDomain(domainName)
		return fmt.Errorf("virt-customize failed: %w: %s", err, out)
	}
	os.Remove(envFile.Name())

	// Start the VM.
	if out, err := exec.CommandContext(ctx, "virsh", "start", domainName).CombinedOutput(); err != nil {
		s.cleanupDomain(domainName)
		return fmt.Errorf("virsh start failed: %w: %s", err, out)
	}

	s.vms[roomID] = &vmInstance{domainName: domainName}
	s.logger.Info("VM started", "roomID", roomID, "domain", domainName)
	return nil
}

// DestroyVM stops the VM and removes the cloned domain and its disk.
func (s *VMService) DestroyVM(roomID string) error {
	s.mu.Lock()
	inst, ok := s.vms[roomID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.vms, roomID)
	s.mu.Unlock()

	s.cleanupDomain(inst.domainName)
	s.logger.Info("VM destroyed", "roomID", roomID, "domain", inst.domainName)
	return nil
}

// cleanupDomain force-stops a domain and removes it along with its storage.
// Errors are logged but not returned — best-effort cleanup.
func (s *VMService) cleanupDomain(domainName string) {
	// destroy is a force-stop; ignore error if the VM is already stopped.
	exec.Command("virsh", "destroy", domainName).Run()

	if out, err := exec.Command("virsh", "undefine", domainName, "--remove-all-storage").CombinedOutput(); err != nil {
		s.logger.Error("virsh undefine failed", "domain", domainName, "err", err, "out", string(out))
	}
}
