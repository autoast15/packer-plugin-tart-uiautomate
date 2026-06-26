package uiauto

import (
	"context"
	"fmt"
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

func (v *VNC) Capture(ctx context.Context, path string) error { return v.run(ctx, "capture", path) }
func (v *VNC) Move(ctx context.Context, x, y int) error { return v.run(ctx, "move", strconv.Itoa(x), strconv.Itoa(y)) }
func (v *VNC) Click(ctx context.Context, x, y int) error { return v.run(ctx, "move", strconv.Itoa(x), strconv.Itoa(y), "click", "1") }
func (v *VNC) DoubleClick(ctx context.Context, x, y int) error {
	return v.run(ctx, "move", strconv.Itoa(x), strconv.Itoa(y), "click", "1", "click", "1")
}
func (v *VNC) Drag(ctx context.Context, x1, y1, x2, y2 int) error {
	return v.run(ctx, "move", strconv.Itoa(x1), strconv.Itoa(y1), "mousedown", "1", "move", strconv.Itoa(x2), strconv.Itoa(y2), "mouseup", "1")
}
func (v *VNC) Scroll(ctx context.Context, dx, dy int) error {
	if dy < 0 {
		return v.run(ctx, "wheeldown", strconv.Itoa(-dy))
	}
	if dy > 0 {
		return v.run(ctx, "wheelup", strconv.Itoa(dy))
	}
	return nil
}
func (v *VNC) TypeText(ctx context.Context, text string) error { return v.run(ctx, "type", text) }
