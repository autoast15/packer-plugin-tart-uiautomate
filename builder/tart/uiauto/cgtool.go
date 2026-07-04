package uiauto

import (
    "context"
    "fmt"
    "os"
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
    // Title-scoped focus (when we know which VM window we mean) instead
    // of a blanket "activate the Tart app" — with a single VM running,
    // AppleScript activate and focus-window behave the same, but
    // parallel builds each spawn their own Tart-owned window, and
    // "activate" has no way to say *which* one should come to front.
    // --no-click preserves focusTart's original semantics (raise/focus
    // only); the extra click focus-window can do to establish VM
    // keyboard capture isn't needed here since typing/clicking already
    // worked fine on plain activation before this change.
    if c.cfg.WindowTitle != "" {
        err := exec.CommandContext(ctx, c.cfg.CGToolPath,
            "focus-window", "--title", c.cfg.WindowTitle, "--no-click").Run()
        if err == nil {
            time.Sleep(150 * time.Millisecond)
            return
        }
        // Fall through to blanket activate — e.g. window not created yet,
        // or Accessibility permission missing (focus-window degrades to
        // activate+AX internally too, but exits nonzero on some AX
        // failures where activate alone would still have succeeded).
    }
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

    // The window can transiently report bogus geometry (guest display
    // renegotiation right after first login, mid-resize animation), so
    // retry before failing the build.
    var lastErr error
    for attempt := 1; attempt <= 6; attempt++ {
        // Raise Tart to foreground so its window is topmost / has correct frame.
        c.focusTart(ctx)

        x, y, w, h, err := c.WindowGeometry(ctx, c.cfg.WindowTitle)
        if err != nil {
            // CRITICAL: do NOT fall back to full-display screenshot. That
            // captures the host desktop and feeds bogus data to VCS.
            lastErr = fmt.Errorf("Capture: could not locate Tart window %q: %w", c.cfg.WindowTitle, err)
        } else if w < 400 || h < 300 || w > 4000 || h > 3000 {
            // Sanity check — dimensions must look like a Tart VM window,
            // not a tiny sidebar or huge full-screen artifact.
            lastErr = fmt.Errorf("Capture: Tart window %q has implausible geometry %dx%d — refusing to screenshot", c.cfg.WindowTitle, w, h)
        } else {
            return c.run(ctx, "screenshot-region",
                strconv.Itoa(x), strconv.Itoa(y),
                strconv.Itoa(w), strconv.Itoa(h),
                path)
        }

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(2 * time.Second):
        }
    }
    return lastErr
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

func (c *CGTool) MoveWindow(ctx context.Context, title string, x, y int) error {
    return c.run(ctx, "move-window", "--title", title, strconv.Itoa(x), strconv.Itoa(y))
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

// --- VNC input via cgtool's native RFB backend ---
//
// These send input over the RFB protocol directly into the guest's input
// stack: no host window focus, no Accessibility permission, and the host
// can never intercept the keystrokes. This is the only reliable way to
// deliver host-reserved shortcuts (cmd+space would otherwise open host
// Spotlight; CGEvent.postToPid is not forwarded by VZVirtualMachineView).
// The endpoint comes from cfg.VNCHost/VNCPort/VNCPassword, which
// StepUIAutomation fills from the vnc:// URL tart prints at boot.

func (c *CGTool) runVNC(ctx context.Context, sub string, rest ...string) error {
    args := []string{sub, "--host", c.cfg.VNCHost, "--port", strconv.Itoa(c.cfg.VNCPort)}
    args = append(args, rest...)
    cmd := exec.CommandContext(ctx, c.cfg.CGToolPath, args...)
    // Password travels via env, not argv, so it never shows up in `ps`
    // or packer logs.
    if c.cfg.VNCPassword != "" {
        cmd.Env = append(os.Environ(), "CGTOOL_VNC_PASSWORD="+c.cfg.VNCPassword)
    }
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("%s %v failed: %w: %s", c.cfg.CGToolPath, args, err, string(out))
    }
    return nil
}

func (c *CGTool) VNCKey(ctx context.Context, key string) error {
    return c.runVNC(ctx, "vnc-key", key)
}

func (c *CGTool) VNCHotkey(ctx context.Context, hotkey string) error {
    return c.runVNC(ctx, "vnc-hotkey", hotkey)
}

func (c *CGTool) VNCTypeText(ctx context.Context, text string) error {
    return c.runVNC(ctx, "vnc-type", text)
}

func (c *CGTool) VNCClick(ctx context.Context, x, y int) error {
    return c.runVNC(ctx, "vnc-click", strconv.Itoa(x), strconv.Itoa(y))
}

// VNCOpenPath opens a file/folder inside the guest via Finder's "Go to
// Folder" dialog (Cmd+Shift+G, type path, Return, Return) — the keyboard
// equivalent of a Finder double-click, with no icon-position detection.
// Requires Finder to be the frontmost app in the guest at the time this
// runs; see the vnc_open_path action's comment in runner.go/the .pkr.hcl
// for why that's safe to assume right after boot but not later.
func (c *CGTool) VNCOpenPath(ctx context.Context, path string) error {
    return c.runVNC(ctx, "vnc-open-path", path)
}
