package vm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
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
		"--connect", "qemu:///system",
		"--original", s.cfg.BaseDomain,
		"--name", domainName,
		"--auto-clone",
		"--check", "disk_size=off",
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

	virtCustomize := exec.CommandContext(ctx, "sudo", "-E",
		"virt-customize",
		"--connect", "qemu:///system",
		"--no-network",
		"-d", domainName,
		"--upload", fmt.Sprintf("%s:/etc/vmagent.env", envFile.Name()),
		"--chmod", "0644:/etc/vmagent.env",
		// Allow local processes (e.g. the vmagent systemd service) to connect
		// to the X display without needing an Xauthority cookie.
		"--run-command", "echo 'xhost +local:' > /etc/X11/Xsession.d/90-xhost-local && chmod +x /etc/X11/Xsession.d/90-xhost-local",
	)
	virtCustomize.Env = append(os.Environ(), "LIBGUESTFS_BACKEND=direct")
	if out, err := virtCustomize.CombinedOutput(); err != nil {
		os.Remove(envFile.Name())
		s.cleanupDomain(domainName)
		return fmt.Errorf("virt-customize failed: %w: %s", err, out)
	}
	os.Remove(envFile.Name())

	// Start the VM.
	if out, err := exec.CommandContext(ctx, "virsh", "-c", "qemu:///system", "start", domainName).CombinedOutput(); err != nil {
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

// cleanupDomain force-stops a domain, removes snapshots, undefines it, and
// deletes its disk images. Errors are logged but not returned — best-effort.
func (s *VMService) cleanupDomain(domainName string) {
	virsh := func(args ...string) (string, error) {
		out, err := exec.Command("virsh", append([]string{"-c", "qemu:///system"}, args...)...).CombinedOutput()
		return string(out), err
	}

	// Force-stop if running.
	if out, err := virsh("domstate", domainName); err == nil {
		if strings.Contains(strings.ToLower(out), "running") {
			if _, err := virsh("destroy", domainName); err != nil {
				s.logger.Warn("virsh destroy failed", "domain", domainName, "err", err)
			}
		}
	}

	// Collect disk paths before undefining.
	diskPaths := s.collectDiskPaths(domainName)
	s.logger.Info("disk paths to remove", "domain", domainName, "disks", diskPaths)

	// Delete snapshot metadata.
	if snapOut, err := virsh("snapshot-list", domainName, "--name"); err == nil {
		for _, snap := range strings.Split(strings.TrimSpace(snapOut), "\n") {
			snap = strings.TrimSpace(snap)
			if snap == "" {
				continue
			}
			virsh("snapshot-delete", domainName, snap, "--metadata") //nolint
		}
	}

	// Undefine — try with all cleanup flags first, fall back to plain undefine.
	if _, err := virsh("undefine", domainName, "--nvram", "--managed-save", "--snapshots-metadata"); err != nil {
		if _, err2 := virsh("undefine", domainName); err2 != nil {
			s.logger.Error("virsh undefine failed", "domain", domainName, "err", err2)
		}
	}

	// Remove disk image files.
	for _, disk := range diskPaths {
		if err := os.Remove(disk); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to remove disk image", "path", disk, "err", err)
		} else {
			s.logger.Info("removed disk image", "path", disk)
		}
	}
}

// collectDiskPaths returns the file paths of all disk images attached to the domain.
func (s *VMService) collectDiskPaths(domainName string) []string {
	out, err := exec.Command("virsh", "-c", "qemu:///system", "domblklist", domainName, "--details").CombinedOutput()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// --details format: Type Device Target Source
		// keep rows where type=file, device=disk
		if len(fields) >= 4 && fields[1] == "disk" && strings.HasPrefix(fields[3], "/") {
			paths = append(paths, fields[3])
		}
	}

	// Fallback: plain domblklist without --details
	if len(paths) == 0 {
		out2, err := exec.Command("virsh", "-c", "qemu:///system", "domblklist", domainName).CombinedOutput()
		if err == nil {
			lines := strings.Split(string(out2), "\n")
			for _, line := range lines[2:] { // skip header rows
				fields := strings.Fields(line)
				if len(fields) >= 2 && strings.HasPrefix(fields[1], "/") {
					paths = append(paths, fields[1])
				}
			}
		}
	}

	return paths
}
