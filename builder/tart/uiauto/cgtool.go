package uiauto

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"regexp"
	"strconv"
)

type CGTool struct {
	cfg *Config
}

func NewCGTool(cfg *Config) *CGTool {
	return &CGTool{cfg: cfg}
}


// focusTart brings the Tart VM window to the front so input events reach it.
func (c *CGTool) focusTart(ctx context.Context) {
	_ = exec.CommandContext(ctx, "osascript",
		"-e", `tell application "Tart" to activate`).Run()
	time.Sleep(150 * time.Millisecond)
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
	if c.cfg.WindowTitle != "" {
		// Bring Tart to the front so its window isn't occluded by terminal etc.
		_ = exec.CommandContext(ctx, "osascript",
			"-e", `tell application "Tart" to activate`).Run()
		time.Sleep(150 * time.Millisecond)

		x, y, w, h, err := c.WindowGeometry(ctx, c.cfg.WindowTitle)
		if err == nil {
			return c.run(ctx, "screenshot-region",
				strconv.Itoa(x), strconv.Itoa(y),
				strconv.Itoa(w), strconv.Itoa(h),
				path)
		}
	}
	return c.run(ctx, "screenshot", path)
}

func (c *CGTool) WindowGeometry(ctx context.Context, title string) (int, int, int, int, error) {
	out, err := exec.CommandContext(ctx, c.cfg.CGToolPath, "list-windows").Output()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	re := regexp.MustCompile(`x=(\d+)\s+y=(\d+)\s+w=(\d+)\s+h=(\d+)`)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, title) {
			continue
		}
		m := re.FindStringSubmatch(line)
		if len(m) == 5 {
			x, _ := strconv.Atoi(m[1])
			y, _ := strconv.Atoi(m[2])
			w, _ := strconv.Atoi(m[3])
			h, _ := strconv.Atoi(m[4])
			return x, y, w, h, nil
		}
	}
	return 0, 0, 0, 0, fmt.Errorf("window %q not found", title)
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
