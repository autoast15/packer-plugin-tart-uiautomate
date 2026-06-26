package uiauto

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

type CGTool struct {
	cfg *Config
}

func NewCGTool(cfg *Config) *CGTool {
	return &CGTool{cfg: cfg}
}

func (c *CGTool) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, c.cfg.CGToolPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", c.cfg.CGToolPath, args, err, string(out))
	}
	return nil
}

func (c *CGTool) Capture(ctx context.Context, path string) error {
	return c.run(ctx, "screenshot", path)
}
func (c *CGTool) Move(ctx context.Context, x, y int) error {
	return c.run(ctx, "move", strconv.Itoa(x), strconv.Itoa(y))
}
func (c *CGTool) Click(ctx context.Context, x, y int) error {
	return c.run(ctx, "click", strconv.Itoa(x), strconv.Itoa(y))
}
func (c *CGTool) DoubleClick(ctx context.Context, x, y int) error {
	return c.run(ctx, "double-click", strconv.Itoa(x), strconv.Itoa(y))
}
func (c *CGTool) Drag(ctx context.Context, x1, y1, x2, y2 int) error {
	return c.run(ctx, "drag", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2))
}
func (c *CGTool) Scroll(ctx context.Context, dx, dy int) error {
	return c.run(ctx, "scroll", strconv.Itoa(dx), strconv.Itoa(dy))
}
func (c *CGTool) TypeText(ctx context.Context, text string) error { return c.run(ctx, "type", text) }
func (c *CGTool) Raw(ctx context.Context, args []string) error    { return c.run(ctx, args...) }
