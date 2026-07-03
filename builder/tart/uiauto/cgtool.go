package uiauto

import (
    "context"
    "fmt"
    "os/exec"
    "regexp"
    "strconv"
    "strings"
    "time"
)

type CGTool struct {
    cfg *Config
}

func NewCGTool(cfg *Config) *CGTool {
    return &CGTool{cfg: cfg}
}

// tartOwnerPatterns lists process-name substrings that identify a legitimate
// Tart VM window. Anything else (Finder folder named the same, Terminal tab
// showing an ssh session, TextEdit editing the .pkr.hcl) is ignored.
var tartOwnerPatterns = []string{"Tart", "com.apple.Virtualization", "Virtualization"}

func isTartOwner(owner string) bool {
    for _, p := range tartOwnerPatterns {
        if strings.Contains(owner, p) {
            return true
        }
    }
    return false
}

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
    if c.cfg.WindowTitle == "" {
        // No window title configured — capture full display.
        return c.run(ctx, "screenshot", path)
    }

    // Raise Tart to foreground so its window is topmost / has correct frame.
    c.focusTart(ctx)

    x, y, w, h, err := c.WindowGeometry(ctx, c.cfg.WindowTitle)
    if err != nil {
        // CRITICAL: do NOT fall back to full-display screenshot. That
        // captures the host desktop and feeds bogus data to VCS. Return
        // the error so the runner surfaces it.
        return fmt.Errorf("Capture: could not locate Tart window %q: %w", c.cfg.WindowTitle, err)
    }

    // Sanity check — dimensions must look like a Tart VM window, not a
    // tiny sidebar or huge full-screen artifact.
    if w < 400 || h < 300 || w > 4000 || h > 3000 {
        return fmt.Errorf("Capture: Tart window %q has implausible geometry %dx%d — refusing to screenshot", c.cfg.WindowTitle, w, h)
    }

    return c.run(ctx, "screenshot-region",
        strconv.Itoa(x), strconv.Itoa(y),
        strconv.Itoa(w), strconv.Itoa(h),
        path)
}

// WindowGeometry looks up the on-screen bounds of the Tart VM window whose
// title contains `title`. It filters by owner process so Terminal tabs,
// Finder folders, and TextEdit documents with matching names are ignored.
// Retries up to 5x @ 200ms to handle transient states during Tart boot.
func (c *CGTool) WindowGeometry(ctx context.Context, title string) (int, int, int, int, error) {
    // list-windows lines look like:
    //   <pid>\t<OwnerName>\t<Title> x=N y=N w=N h=N
    // Fields are tab-separated up to the title, then geometry is at the end.
    geomRE := regexp.MustCompile(`x=(\d+)\s+y=(\d+)\s+w=(\d+)\s+h=(\d+)`)

    var lastErr error
    for attempt := 0; attempt < 5; attempt++ {
        out, err := exec.CommandContext(ctx, c.cfg.CGToolPath, "list-windows").Output()
        if err != nil {
            lastErr = err
            time.Sleep(200 * time.Millisecond)
            continue
        }

        var bestX, bestY, bestW, bestH int
        var found bool

        for _, line := range strings.Split(string(out), "\n") {
            if !strings.Contains(line, title) {
                continue
            }
            // Split by tab to extract owner (field index 1).
            parts := strings.SplitN(line, "\t", 3)
            if len(parts) < 3 {
                continue
            }
            owner := strings.TrimSpace(parts[1])
            if !isTartOwner(owner) {
                // Skip Finder/Terminal/TextEdit windows that happen to have
                // the vm name in their title.
                continue
            }
            m := geomRE.FindStringSubmatch(line)
            if len(m) != 5 {
                continue
            }
            x, _ := strconv.Atoi(m[1])
            y, _ := strconv.Atoi(m[2])
            w, _ := strconv.Atoi(m[3])
            h, _ := strconv.Atoi(m[4])
            // Prefer the largest matching window (Tart's main VM window
            // will be much bigger than any incidental helper windows).
            if !found || (w*h) > (bestW*bestH) {
                bestX, bestY, bestW, bestH = x, y, w, h
                found = true
            }
        }
        if found {
            return bestX, bestY, bestW, bestH, nil
        }
        lastErr = fmt.Errorf("window %q owned by a Tart process not found (attempt %d/5)", title, attempt+1)
        time.Sleep(200 * time.Millisecond)
    }
    return 0, 0, 0, 0, lastErr
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

func (c *CGTool) TypeText(ctx context.Context, text string) error {
    return c.run(ctx, "type", text)
}

func (c *CGTool) Key(ctx context.Context, key string) error {
    return c.run(ctx, "key", key)
}

func (c *CGTool) Hotkey(ctx context.Context, hotkey string) error {
    return c.run(ctx, "hotkey", hotkey)
}

func (c *CGTool) Raw(ctx context.Context, args []string) error {
    return c.run(ctx, args...)
}
