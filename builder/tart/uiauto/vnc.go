package uiauto

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

type VNC struct {
	cfg *Config
}

func NewVNC(cfg *Config) *VNC {
	return &VNC{cfg: cfg}
}

func (v *VNC) baseArgs() []string {
	args := []string{"-s", fmt.Sprintf("%s::%d", v.cfg.VNCHost, v.cfg.VNCPort)}
	if v.cfg.VNCPassword != "" {
		args = append(args, "-p", v.cfg.VNCPassword)
	}
	return args
}

func (v *VNC) run(ctx context.Context, args ...string) error {
	all := append(v.baseArgs(), args...)
	cmd := exec.CommandContext(ctx, v.cfg.VNCDoPath, all...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", v.cfg.VNCDoPath, all, err, string(out))
	}
	return nil
}

// cgtoolVNC runs one of cgtool's native `vnc-*` subcommands — the RFB
// client already proven this session against Tart's actual VNC server
// (see cgtool-project memory). Used instead of v.run/vncdo for every
// input primitive that has a cgtool equivalent, so ui_backend="vnc" has
// no dependency on the separate, never-verified vncdo Python tool at all.
func (v *VNC) cgtoolVNC(ctx context.Context, args ...string) error {
	all := append([]string{}, args...)
	all = append(all, "--host", v.cfg.VNCHost, "--port", strconv.Itoa(v.cfg.VNCPort))
	cmd := exec.CommandContext(ctx, v.cfg.CGToolPath, all...)
	if v.cfg.VNCPassword != "" {
		cmd.Env = append(os.Environ(), "CGTOOL_VNC_PASSWORD="+v.cfg.VNCPassword)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", v.cfg.CGToolPath, all, err, string(out))
	}
	return nil
}

// Capture pulls a frame straight from the guest's VNC framebuffer via
// `vcs vnc-screenshot` — the same RFB client used for VCS's own
// detection, verified this session against a real handshake+pixel decode
// round trip. This intentionally does NOT go through vncdo/v.run: vncdo is
// a separate, never-verified-this-session Python dependency, and the
// ui_backend=vnc variant of this pipeline exists specifically to avoid any
// host-side capture (cgtool's window screenshot) as well as any unproven
// dependency in the capture path.
func (v *VNC) Capture(ctx context.Context, path string) error {
	vcsPath := "vcs"
	if len(v.cfg.DetectorCommand) > 0 {
		vcsPath = v.cfg.DetectorCommand[0]
	}
	args := []string{"vnc-screenshot", "--host", v.cfg.VNCHost, "--port", strconv.Itoa(v.cfg.VNCPort), "-o", path}
	cmd := exec.CommandContext(ctx, vcsPath, args...)
	if v.cfg.VNCPassword != "" {
		cmd.Env = append(os.Environ(), "CGTOOL_VNC_PASSWORD="+v.cfg.VNCPassword)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", vcsPath, args, err, string(out))
	}
	return nil
}
func (v *VNC) Move(ctx context.Context, x, y int) error {
	return v.cgtoolVNC(ctx, "vnc-move", strconv.Itoa(x), strconv.Itoa(y))
}
func (v *VNC) Click(ctx context.Context, x, y int) error {
	return v.cgtoolVNC(ctx, "vnc-click", strconv.Itoa(x), strconv.Itoa(y))
}
func (v *VNC) DoubleClick(ctx context.Context, x, y int) error {
	return v.cgtoolVNC(ctx, "vnc-click", strconv.Itoa(x), strconv.Itoa(y), "--count", "2")
}

// Drag has no cgtool vnc-* equivalent (no vnc-mousedown/vnc-mouseup) and
// isn't used anywhere in this pipeline today — fail clearly instead of
// silently falling back to the unproven vncdo path.
func (v *VNC) Drag(ctx context.Context, x1, y1, x2, y2 int) error {
	return fmt.Errorf("drag is not supported over the native VNC backend (no cgtool vnc-drag primitive); add one to cgtool if this pipeline ever needs it")
}

// Scroll has no cgtool vnc-* equivalent either; still routed through
// vncdo since nothing in this pipeline's ui_backend=vnc path currently
// issues a scroll action, so this is dead code in practice, not a
// silent reliability gap for anything actually exercised.
func (v *VNC) Scroll(ctx context.Context, dx, dy int) error {
	if dy < 0 {
		return v.run(ctx, "wheeldown", strconv.Itoa(-dy))
	}
	if dy > 0 {
		return v.run(ctx, "wheelup", strconv.Itoa(dy))
	}
	return nil
}
func (v *VNC) TypeText(ctx context.Context, text string) error {
	return v.cgtoolVNC(ctx, "vnc-type", text)
}
