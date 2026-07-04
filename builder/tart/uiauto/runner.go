package uiauto

import (
    "context"
    "encoding/json"
    "fmt"
    "hash/fnv"
    "os"
    "path/filepath"
    "strings"
    "time"
)

type Logger interface {
    Say(message string)
}

type noopLogger struct{}

func (noopLogger) Say(string) {}

type Runner struct {
    cfg        *Config
    log        Logger
    vnc        *VNC
    cg         *CGTool
    trace      *os.File
    step       int
    cachedDet  *Detection // cached detection from last wait_for_scene
    cachedScene string    // scene name the cache was built for
    cacheHits  int        // how many actions used the cache

    // lastClickAction/lastClickScene support wait_for_scene's stuck-retry:
    // if the guest is still showing the same scene the last click_control
    // ran against, the click plausibly never registered (dropped input,
    // animation ate it, etc.) and re-issuing the exact same click is a
    // reasonable, generic recovery — no per-step HCL config needed.
    lastClickAction *Action
    lastClickScene  string
}

func NewRunner(cfg *Config, log Logger) *Runner {
    if log == nil {
        log = noopLogger{}
    }
    return &Runner{
        cfg: cfg,
        log: log,
        vnc: NewVNC(cfg),
        cg:  NewCGTool(cfg),
    }
}

func (r *Runner) useCGTool() bool { return r.cfg.UIBackend == "cgtool" }

func isChevronLabel(label string) bool {
    switch label {
    case "→", "▶", "▸", "➤", "›", ">", "":
        return true
    }
    return false
}

// windowPinOffset deterministically spreads Tart windows across the
// screen by vm_name/window title, so parallel builds don't all pin to
// the exact same spot and fully overlap. Bounds are chosen for a
// 1280-wide guest window on a display with at least ~1680pt of usable
// width (x caps at 400, keeping the window's right edge within 1680) and
// enough headroom below the menu bar for a few rows of vertical stagger.
func windowPinOffset(title string) (int, int) {
    h := fnv.New32a()
    _, _ = h.Write([]byte(title))
    sum := h.Sum32()
    x := int(sum % 400)
    y := 40 + int((sum/400)%200)
    return x, y
}

func (r *Runner) translateClick(ctx context.Context, x, y int) (int, int) {
    if !r.useCGTool() || r.cfg.WindowTitle == "" {
        r.writeEvent("translate_skip", map[string]interface{}{"x": x, "y": y, "reason": "no cgtool or window title"})
        return x, y
    }
    wx, wy, _, _, err := r.cg.WindowGeometry(ctx, r.cfg.WindowTitle)
    if err != nil {
        r.writeEvent("translate_failed", map[string]interface{}{"x": x, "y": y, "error": err.Error()})
        return x, y
    }
    r.writeEvent("translate_applied", map[string]interface{}{
        "window_relative_x": x,
        "window_relative_y": y,
        "window_x":          wx,
        "window_y":          wy,
        "screen_x":          x + wx,
        "screen_y":          y + wy,
    })
    return x + wx, y + wy
}

func (r *Runner) Capture(ctx context.Context, path string) error {
    if r.useCGTool() {
        return r.cg.Capture(ctx, path)
    }
    return r.vnc.Capture(ctx, path)
}

func (r *Runner) Move(ctx context.Context, x, y int) error {
    if r.useCGTool() {
        return r.cg.Move(ctx, x, y)
    }
    return r.vnc.Move(ctx, x, y)
}

func (r *Runner) Click(ctx context.Context, x, y int) error {
    if r.useCGTool() {
        r.cg.focusTart(ctx)
        return r.cg.Click(ctx, x, y)
    }
    return r.vnc.Click(ctx, x, y)
}

func (r *Runner) DoubleClick(ctx context.Context, x, y int) error {
    if r.useCGTool() {
        return r.cg.DoubleClick(ctx, x, y)
    }
    return r.vnc.DoubleClick(ctx, x, y)
}

func (r *Runner) Drag(ctx context.Context, x1, y1, x2, y2 int) error {
    if r.useCGTool() {
        return r.cg.Drag(ctx, x1, y1, x2, y2)
    }
    return r.vnc.Drag(ctx, x1, y1, x2, y2)
}

func (r *Runner) Scroll(ctx context.Context, dx, dy int) error {
    if r.useCGTool() {
        return r.cg.Scroll(ctx, dx, dy)
    }
    return r.vnc.Scroll(ctx, dx, dy)
}

func (r *Runner) TypeText(ctx context.Context, text string) error {
    if r.useCGTool() {
        r.cg.focusTart(ctx)
        return r.cg.TypeText(ctx, text)
    }
    return r.vnc.TypeText(ctx, text)
}

func (r *Runner) Run(ctx context.Context) error {
    if r.cfg == nil || !r.cfg.Enabled {
        return nil
    }
    if err := r.cfg.PrepareDefaults(); err != nil {
        return err
    }
    if err := os.MkdirAll(r.cfg.ArtifactDir, 0755); err != nil {
        return err
    }
    tf, err := os.Create(filepath.Join(r.cfg.ArtifactDir, "trace.jsonl"))
    if err != nil {
        return err
    }
    r.trace = tf
    defer tf.Close()

    r.log.Say("Running Tart UI automation before SSH wait")
    defer func() { _ = r.capture(ctx, "last-screen.png") }()

    // Pin the Tart viewer to a known on-screen position before anything
    // else runs. macOS cascades new window positions relative to
    // whatever windows already exist/existed on the host desktop — over
    // a long dev/test session (or just an unlucky host state) that can
    // drift a 1280-wide window far enough right that part of it renders
    // off-screen. translateClick() computes click_control/click_text
    // coordinates from the window's *current* position, so an off-screen
    // window means a correctly-detected control still gets clicked at an
    // invalid screen point.
    //
    // The position is staggered by a hash of WindowTitle (== vm_name in
    // practice) rather than a single fixed point, so parallel builds
    // (each with a distinct vm_name/window title) land at different,
    // deterministic spots instead of every window pinning to the exact
    // same location and fully overlapping. Deterministic also means a
    // given vm_name always lands in the same place across runs, which is
    // convenient when watching a build live.
    //
    // Best-effort only: if it fails (no Accessibility permission, window
    // not found yet) later actions still get whatever position the
    // window already has.
    if r.useCGTool() && r.cfg.WindowTitle != "" {
        px, py := windowPinOffset(r.cfg.WindowTitle)
        if err := r.cg.MoveWindow(ctx, r.cfg.WindowTitle, px, py); err != nil {
            r.writeEvent("pin_window_failed", map[string]interface{}{"error": err.Error()})
        } else {
            r.writeEvent("pin_window", map[string]interface{}{"x": px, "y": py})
        }
    }

    scenes := map[string]Scene{}
    for _, s := range r.cfg.Scenes {
        scenes[s.Name] = s
    }

    for _, a := range r.cfg.Actions {
        if err := r.execAction(ctx, a, scenes); err != nil {
            r.writeEvent("failure", map[string]interface{}{"error": err.Error(), "action": a})
            _ = r.failureBundle(ctx)
            return err
        }
    }
    return nil
}

func (r *Runner) writeEvent(kind string, fields map[string]interface{}) {
    if r.trace == nil {
        return
    }
    fields["ts"] = time.Now().Format(time.RFC3339Nano)
    fields["event"] = kind
    fields["step"] = r.step
    b, _ := json.Marshal(fields)
    _, _ = r.trace.Write(append(b, '\n'))
}

func (r *Runner) capture(ctx context.Context, name string) error {
    return r.Capture(ctx, filepath.Join(r.cfg.ArtifactDir, name))
}

func (r *Runner) detection(ctx context.Context, name string) (*Detection, error) {
    shot := filepath.Join(r.cfg.ArtifactDir, name)
    if err := r.Capture(ctx, shot); err != nil {
        return nil, err
    }
    d, err := detect(ctx, r.cfg, shot)
    if err != nil {
        return nil, err
    }
    data, _ := json.MarshalIndent(d, "", "  ")
    _ = os.WriteFile(filepath.Join(r.cfg.ArtifactDir, name+".controls.json"), data, 0644)
    return d, nil
}

// cachedOrDetect returns the cached detection if available (scene_cached mode),
// otherwise takes a fresh screenshot and runs VCS. On cache hit, no screenshot
// or VCS invocation happens — this is the main speed optimisation.
func (r *Runner) cachedOrDetect(ctx context.Context, name string) (*Detection, bool, error) {
    if r.sceneCached() && r.cachedDet != nil {
        r.cacheHits++
        r.writeEvent("detection_cache_hit", map[string]interface{}{
            "cached_scene": r.cachedScene,
            "cache_hits":   r.cacheHits,
        })
        return r.cachedDet, true, nil
    }
    d, err := r.detection(ctx, name)
    return d, false, err
}

func (r *Runner) invalidateCache(reason string) {
    if r.cachedDet != nil {
        r.writeEvent("cache_invalidated", map[string]interface{}{
            "reason":       reason,
            "cached_scene": r.cachedScene,
            "cache_hits":   r.cacheHits,
        })
        r.cachedDet = nil
        r.cachedScene = ""
        r.cacheHits = 0
    }
}

func (r *Runner) sceneCached() bool {
    return r.cfg.ScreenshotMode == "scene_cached"
}

func (r *Runner) execAction(ctx context.Context, a Action, scenes map[string]Scene) error {
    r.step++
    if r.cfg.ScreenshotMode == "before_after_each_step" {
        _ = r.capture(ctx, fmt.Sprintf("%04d-before-%s.png", r.step, a.Type))
    }
    // In scene_cached mode, log that we're using the cache when applicable.
    r.writeEvent("action_start", map[string]interface{}{
        "action":       a,
        "cache_active": r.sceneCached() && r.cachedDet != nil,
    })

    var err error
    switch a.Type {
    case "move":
        tx, ty := r.translateClick(ctx, a.X, a.Y)
        err = r.Move(ctx, tx, ty)
    case "click":
        // Window-relative like click_control/click_text: the Tart window's
        // screen position isn't fixed run-to-run, so a raw click action
        // needs the same translateClick() the detection-based actions
        // already get, or hardcoded coordinates only work when the window
        // happens to be at the same spot it was calibrated against.
        tx, ty := r.translateClick(ctx, a.X, a.Y)
        err = r.Click(ctx, tx, ty)
    case "double_click":
        tx, ty := r.translateClick(ctx, a.X, a.Y)
        err = r.DoubleClick(ctx, tx, ty)
    case "drag":
        tx1, ty1 := r.translateClick(ctx, a.X, a.Y)
        tx2, ty2 := r.translateClick(ctx, a.X2, a.Y2)
        err = r.Drag(ctx, tx1, ty1, tx2, ty2)
    case "scroll":
        err = r.Scroll(ctx, a.DX, a.DY)
    case "type":
        err = r.TypeText(ctx, a.Text)
    case "wait":
        if a.TimeoutSeconds > 0 {
            r.log.Say(fmt.Sprintf("uiauto: waiting %d seconds", a.TimeoutSeconds))
            select {
            case <-time.After(time.Duration(a.TimeoutSeconds) * time.Second):
            case <-ctx.Done():
                err = ctx.Err()
            }
        }
    case "screenshot":
        r.log.Say(fmt.Sprintf("uiauto: screenshot %s", a.Name))
        if a.Name == "" {
            a.Name = fmt.Sprintf("%04d-screenshot.png", r.step)
        }
        err = r.capture(ctx, a.Name)
    case "click_text", "click_if_visible":
        var d *Detection
        var cached bool
        d, cached, err = r.cachedOrDetect(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        if err != nil {
            break
        }
        if hit, ok := selectOCR(d, a.Text, a.Region, a.Match); ok {
            x, y := hit.BBox.Center()
            x, y = x+a.OffsetX, y+a.OffsetY
            x, y = r.translateClick(ctx, x, y)
            err = r.Click(ctx, x, y)
            r.writeEvent("click_text", map[string]interface{}{"text": hit.Text, "x": x, "y": y, "offset_x": a.OffsetX, "offset_y": a.OffsetY, "cached": cached})
        } else if cached {
            // Cache miss — retry with fresh detection.
            r.invalidateCache("click_text_miss")
            d, err = r.detection(ctx, fmt.Sprintf("%04d-detect-retry.png", r.step))
            if err != nil {
                break
            }
            if hit, ok := selectOCR(d, a.Text, a.Region, a.Match); ok {
                x, y := hit.BBox.Center()
                x, y = x+a.OffsetX, y+a.OffsetY
                x, y = r.translateClick(ctx, x, y)
                err = r.Click(ctx, x, y)
                r.writeEvent("click_text", map[string]interface{}{"text": hit.Text, "x": x, "y": y, "offset_x": a.OffsetX, "offset_y": a.OffsetY, "retried": true})
            } else if a.Type == "click_text" {
                err = fmt.Errorf("text not visible: %q", a.Text)
            }
        } else if a.Type == "click_text" {
            err = fmt.Errorf("text not visible: %q", a.Text)
        }
    case "click_control":
        var d *Detection
        var cached bool
        if a.Enabled != nil {
            // An enabled filter exists to disambiguate same-labeled
            // controls across UI states (e.g. a background button vs.
            // a modal's same-labeled button that appears after an
            // earlier click in this same action). The scene cache is
            // only invalidated on a selector *miss*, so if the stale
            // cache still happens to contain a same-labeled control
            // matching the requested enabled state (the pre-modal
            // background button, say), a cache hit would silently
            // click that stale location instead of the live one.
            // Always take a fresh detection in this case.
            d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        } else {
            d, cached, err = r.cachedOrDetect(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        }
        if err != nil {
            break
        }
        if hit, ok := selectControl(d, a.Role, a.Label, a.Value, a.Region, a.Match, nil, a.LabelContains, a.Style, a.Enabled); ok {
            x, y := hit.BBox.Center()
            if isChevronLabel(hit.Label) {
                x = hit.BBox.X + (hit.BBox.W * 3 / 4)
            }
            x, y = r.translateClick(ctx, x, y)
            err = r.Click(ctx, x, y)
            r.writeEvent("click_control", map[string]interface{}{"role": hit.Role, "label": hit.Label, "x": x, "y": y, "cached": cached})
            if err == nil {
                clicked := a
                r.lastClickAction = &clicked
                r.lastClickScene = d.Scene
            }
        } else if cached {
            // Cache miss — retry with fresh detection.
            r.invalidateCache("click_control_miss")
            d, err = r.detection(ctx, fmt.Sprintf("%04d-detect-retry.png", r.step))
            if err != nil {
                break
            }
            if hit, ok := selectControl(d, a.Role, a.Label, a.Value, a.Region, a.Match, nil, a.LabelContains, a.Style, a.Enabled); ok {
                x, y := hit.BBox.Center()
                if isChevronLabel(hit.Label) {
                    x = hit.BBox.X + (hit.BBox.W * 3 / 4)
                }
                x, y = r.translateClick(ctx, x, y)
                err = r.Click(ctx, x, y)
                r.writeEvent("click_control", map[string]interface{}{"role": hit.Role, "label": hit.Label, "x": x, "y": y, "retried": true})
                if err == nil {
                    clicked := a
                    r.lastClickAction = &clicked
                    r.lastClickScene = d.Scene
                }
            } else {
                err = fmt.Errorf("control not visible: role=%q label=%q", a.Role, a.Label)
            }
        } else {
            err = fmt.Errorf("control not visible: role=%q label=%q", a.Role, a.Label)
        }
    case "assert_control":
        var d *Detection
        if a.Enabled != nil {
            // See the matching comment in the click_control case: an
            // enabled filter targets state-dependent controls that a
            // stale cache can misrepresent.
            d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        } else {
            d, _, err = r.cachedOrDetect(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        }
        if err != nil {
            break
        }
        if _, ok := selectControl(d, a.Role, a.Label, a.Value, a.Region, a.Match, a.Selected, a.LabelContains, a.Style, a.Enabled); !ok {
            err = fmt.Errorf("assertion failed for control: role=%q label=%q", a.Role, a.Label)
        }
    case "wait_for_scene":
        err = r.waitForScene(ctx, a)
    case "assert_scene":
        var d *Detection
        d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        if err != nil {
            break
        }
        if !selectScene(d, a.Scene) {
            err = fmt.Errorf("assert_scene failed: want=%q got=%q", a.Scene, d.Scene)
        }
    case "run_scene":
        s, ok := scenes[a.Scene]
        if !ok {
            err = fmt.Errorf("unknown scene: %s", a.Scene)
            break
        }
        err = r.runScene(ctx, s, scenes)
    case "key":
        if r.useCGTool() {
            err = r.cg.run(ctx, "key", a.Key)
        } else {
            err = r.cg.VNCKey(ctx, a.Key)
        }
    case "hotkey":
        if r.useCGTool() {
            err = r.cg.run(ctx, "hotkey", a.Hotkey)
        } else {
            err = r.cg.VNCHotkey(ctx, a.Hotkey)
        }
    // vnc_* actions always use cgtool's RFB backend regardless of
    // ui_backend — input lands in the guest's input stack directly, so
    // host-reserved shortcuts (cmd+space) cannot leak to the host.
    case "vnc_key":
        r.log.Say(fmt.Sprintf("uiauto: vnc key %s", a.Key))
        err = r.cg.VNCKey(ctx, a.Key)
    case "vnc_hotkey":
        r.log.Say(fmt.Sprintf("uiauto: vnc hotkey %s", a.Hotkey))
        err = r.cg.VNCHotkey(ctx, a.Hotkey)
    case "vnc_type":
        r.log.Say(fmt.Sprintf("uiauto: vnc type %d chars", len(a.Text)))
        err = r.cg.VNCTypeText(ctx, a.Text)
    case "vnc_click":
        r.log.Say(fmt.Sprintf("uiauto: vnc click %d,%d", a.X, a.Y))
        err = r.cg.VNCClick(ctx, a.X, a.Y)
    case "vnc_open_path":
        // Path goes in the `text` field (no dedicated `path` field, to
        // avoid an Action struct/hcl2spec change for one action type).
        // Requires Finder to be frontmost in the guest — see the
        // vnc_open_path usage note in test-uiauto.pkr.hcl.
        r.log.Say(fmt.Sprintf("uiauto: vnc open-path %s", a.Text))
        err = r.cg.VNCOpenPath(ctx, a.Text)
    case "cgtool":
        r.log.Say(fmt.Sprintf("uiauto: running cgtool %s", strings.Join(a.Args, " ")))
        err = r.cg.Raw(ctx, a.Args)
    case "type_into_field":
        var d *Detection
        var cached bool
        d, cached, err = r.cachedOrDetect(ctx, fmt.Sprintf("%04d-detect.png", r.step))
        if err != nil {
            break
        }
        hit, ok := selectTextFieldBySelector(d, a.TextFieldByPosition)
        if !ok && cached {
            // Cache miss — retry with fresh detection.
            r.invalidateCache("type_into_field_miss")
            d, err = r.detection(ctx, fmt.Sprintf("%04d-detect-retry.png", r.step))
            if err != nil {
                break
            }
            hit, ok = selectTextFieldBySelector(d, a.TextFieldByPosition)
        }
        if !ok {
            err = fmt.Errorf("textfield not visible: text_field_by_position=%q", a.TextFieldByPosition)
            break
        }
        x, y := hit.BBox.Center()
        x, y = r.translateClick(ctx, x, y)
        if err = r.Click(ctx, x, y); err != nil {
            break
        }
        r.writeEvent("type_into_field_click", map[string]interface{}{
            "text_field_by_position": a.TextFieldByPosition,
            "label":                  hit.Label,
            "x":                      x,
            "y":                      y,
        })
        if a.ClearFirst {
            if e := r.cg.run(ctx, "hotkey", "cmd+a"); e != nil {
                err = e
                break
            }
            time.Sleep(80 * time.Millisecond)
            if e := r.cg.run(ctx, "key", "delete"); e != nil {
                err = e
                break
            }
            time.Sleep(80 * time.Millisecond)
        }
        if a.Text != "" {
            err = r.TypeText(ctx, a.Text)
        }
    case "wait_for_button":
        err = r.waitForButton(ctx, a)
    case "identify_panes":
        err = r.identifyPanes(ctx)
    case "scroll_collect":
        err = r.scrollCollect(ctx, a)
    default:
        err = fmt.Errorf("unknown action type: %s", a.Type)
    }

    if r.cfg.ScreenshotMode == "before_after_each_step" {
        _ = r.capture(ctx, fmt.Sprintf("%04d-after-%s.png", r.step, a.Type))
    }

    if err != nil {
        return err
    }
    r.writeEvent("action_done", map[string]interface{}{"type": a.Type})
    return nil
}

func (r *Runner) waitForScene(ctx context.Context, a Action) error {
    timeout := time.Duration(a.TimeoutSeconds) * time.Second
    if timeout == 0 {
        timeout = 60 * time.Second
    }
    poll := 1500 * time.Millisecond
    deadline := time.Now().Add(timeout)
    attempt := 0
    lastScene := ""

    // Stuck-scene recovery: a click that should have advanced the wizard
    // can silently fail to register (dropped input, an animation ate it,
    // the guest was momentarily busy) and no amount of re-polling
    // classification fixes that — the same scene is genuinely still on
    // screen. If this wait is still seeing the scene the last
    // click_control ran against, re-issue that exact click a couple of
    // times over the course of the timeout rather than just failing at
    // the deadline. Generic and automatic: no per-step HCL config.
    const retryEvery = 6 // ~9s at the 1.5s poll interval below
    const maxRetries = 2
    retriesUsed := 0

    r.log.Say(fmt.Sprintf("uiauto: waiting for scene %q (timeout %s)", a.Scene, timeout))
    r.invalidateCache("wait_for_scene")

    for {
        attempt++
        d, derr := r.detection(ctx, fmt.Sprintf("%04d-wait-%s-%03d.png",
            r.step, sanitize(a.Scene), attempt))
        if derr == nil {
            lastScene = d.Scene
            if selectScene(d, a.Scene) {
                // Cache the detection for subsequent actions in this scene.
                r.cachedDet = d
                r.cachedScene = d.Scene
                r.cacheHits = 0
                r.writeEvent("wait_for_scene_match", map[string]interface{}{
                    "scene":   a.Scene,
                    "attempt": attempt,
                    "cached":  r.sceneCached(),
                })
                return nil
            }
            if r.lastClickAction != nil && r.lastClickScene != "" &&
                d.Scene == r.lastClickScene && retriesUsed < maxRetries &&
                attempt%retryEvery == 0 {
                retriesUsed++
                r.writeEvent("wait_for_scene_retry_click", map[string]interface{}{
                    "scene":        a.Scene,
                    "stuck_on":     d.Scene,
                    "attempt":      attempt,
                    "retry_number": retriesUsed,
                })
                if rerr := r.execAction(ctx, *r.lastClickAction, nil); rerr != nil {
                    r.writeEvent("wait_for_scene_retry_click_failed", map[string]interface{}{"error": rerr.Error()})
                }
            }
        } else {
            r.writeEvent("wait_for_scene_detect_error", map[string]interface{}{
                "attempt": attempt,
                "error":   derr.Error(),
            })
        }

        if time.Now().After(deadline) {
            return fmt.Errorf("wait_for_scene timed out: want=%q last=%q attempts=%d",
                a.Scene, lastScene, attempt)
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(poll):
        }
    }
}

func (r *Runner) waitForButton(ctx context.Context, a Action) error {
    r.invalidateCache("wait_for_button")
    timeout := time.Duration(a.TimeoutSeconds) * time.Second
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    poll := time.Second
    deadline := time.Now().Add(timeout)
    attempt := 0

    wantLabel := a.Label
    if wantLabel == "" {
        wantLabel = a.LabelContains
    }
    r.log.Say(fmt.Sprintf("uiauto: waiting for button %q style=%q (timeout %s)",
        wantLabel, a.Style, timeout))

    for {
        attempt++
        d, derr := r.detection(ctx,
            fmt.Sprintf("%04d-waitbtn-%s-%03d.png",
                r.step, sanitize(wantLabel), attempt))
        if derr == nil {
            if _, ok := selectControl(
                d,
                "button", a.Label, a.Value, a.Region, a.Match, nil,
                a.LabelContains, a.Style, a.Enabled,
            ); ok {
                r.writeEvent("wait_for_button_match", map[string]interface{}{
                    "label":   wantLabel,
                    "style":   a.Style,
                    "attempt": attempt,
                })
                return nil
            }
        } else {
            r.writeEvent("wait_for_button_detect_error", map[string]interface{}{
                "attempt": attempt,
                "error":   derr.Error(),
            })
        }

        if time.Now().After(deadline) {
            return fmt.Errorf(
                "wait_for_button timed out: label=%q style=%q attempts=%d",
                wantLabel, a.Style, attempt)
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(poll):
        }
    }
}

// identifyPanes runs one detection and logs the sidebar/content split VCS
// found (via PaneDetector on the Swift side) — mainly a debugging/sanity
// action for HCL authors, and a way to confirm pane geometry looks right
// before relying on it in a scroll_collect step.
func (r *Runner) identifyPanes(ctx context.Context) error {
    d, err := r.detection(ctx, fmt.Sprintf("%04d-panes.png", r.step))
    if err != nil {
        return err
    }
    if d.Panes == nil {
        r.log.Say("uiauto: identify_panes — no sidebar/content split found")
        r.writeEvent("identify_panes", map[string]interface{}{"found": false})
        return nil
    }
    r.log.Say(fmt.Sprintf("uiauto: identify_panes — sidebar=%+v content=%+v", d.Panes.Sidebar, d.Panes.Content))
    r.writeEvent("identify_panes", map[string]interface{}{
        "found":   true,
        "sidebar": d.Panes.Sidebar,
        "content": d.Panes.Content,
    })
    return nil
}

// scrollCollectEntry is the scroll_collect artifact shape: a detected
// control plus which scroll step(s) it was seen at, so a human (or a later
// automated pass) can tell a one-off detection glitch from a control that
// was genuinely visible throughout.
type scrollCollectEntry struct {
    Control
    FirstSeenScroll int `json:"first_seen_scroll"`
    LastSeenScroll  int `json:"last_seen_scroll"`
}

// scrollCollect solves the "content taller than the viewport" problem
// generically: a control below the fold (e.g. Remote Login in a long
// Sharing pane) is invisible to a single screenshot no matter how correct
// the detector is. This scrolls the content pane in a.DY-sized steps,
// running detection after each step and merging newly-seen controls
// (deduped by role+label) into a single inventory, until a full step turns
// up nothing new (two in a row, to tolerate one sparse/blank stretch) or
// a.MaxScrolls is hit. It deliberately does NOT click anything or rewrite
// the scene cache — it leaves the pane positioned wherever the last new
// control was found, so a normal click_control right after this action
// still does a live, correctly-positioned detection instead of clicking a
// stale cached bbox from an earlier scroll position.
func (r *Runner) scrollCollect(ctx context.Context, a Action) error {
    maxScrolls := a.MaxScrolls
    if maxScrolls <= 0 {
        maxScrolls = 8
    }
    dy := a.DY
    if dy == 0 {
        dy = -300
    }
    dx := a.DX

    first, err := r.detection(ctx, fmt.Sprintf("%04d-scrollcollect-000.png", r.step))
    if err != nil {
        return err
    }

    contentPane := Rect{X: 0, Y: 0, W: first.Screen.Width, H: first.Screen.Height}
    if first.Panes != nil {
        contentPane = first.Panes.Content
    }
    // Move the cursor into the content pane before scrolling — CGEvent
    // scroll-wheel events target whatever's under the pointer, so without
    // this a scroll issued while the pointer sits over the sidebar (or
    // wherever the last click happened to land) would scroll the wrong
    // list, or nothing at all.
    cxWin, cyWin := contentPane.X+contentPane.W/2, contentPane.Y+contentPane.H/2
    tx, ty := r.translateClick(ctx, cxWin, cyWin)
    if err := r.Move(ctx, tx, ty); err != nil {
        return err
    }

    merged := map[string]*scrollCollectEntry{}
    matches := func(c Control) bool {
        if a.Role != "" && !strings.EqualFold(c.Role, a.Role) {
            return false
        }
        if a.Label != "" && !strings.EqualFold(c.Label, a.Label) {
            return false
        }
        if a.LabelContains != "" && !containsFold(c.Label, a.LabelContains) {
            return false
        }
        cx, cy := c.BBox.Center()
        return cx >= contentPane.X && cx < contentPane.X+contentPane.W &&
            cy >= contentPane.Y && cy < contentPane.Y+contentPane.H
    }
    absorb := func(d *Detection, scrollIdx int) int {
        newCount := 0
        for _, c := range d.Controls {
            if !matches(c) {
                continue
            }
            key := strings.ToLower(c.Role) + "|" + strings.ToLower(c.Label)
            if e, ok := merged[key]; ok {
                e.Control = c
                e.LastSeenScroll = scrollIdx
            } else {
                merged[key] = &scrollCollectEntry{Control: c, FirstSeenScroll: scrollIdx, LastSeenScroll: scrollIdx}
                newCount++
            }
        }
        return newCount
    }

    absorb(first, 0)
    consecutiveEmpty := 0

    for i := 1; i <= maxScrolls; i++ {
        if err := r.Scroll(ctx, dx, dy); err != nil {
            return err
        }
        time.Sleep(250 * time.Millisecond)
        d, err := r.detection(ctx, fmt.Sprintf("%04d-scrollcollect-%03d.png", r.step, i))
        if err != nil {
            return err
        }
        newCount := absorb(d, i)
        r.writeEvent("scroll_collect_step", map[string]interface{}{
            "scroll_index":   i,
            "new_controls":   newCount,
            "total_controls": len(merged),
        })
        if newCount == 0 {
            consecutiveEmpty++
            if consecutiveEmpty >= 2 {
                break // two scrolls with nothing new — treat as bottom/top reached
            }
        } else {
            consecutiveEmpty = 0
        }
    }

    entries := make([]*scrollCollectEntry, 0, len(merged))
    for _, e := range merged {
        entries = append(entries, e)
    }
    data, _ := json.MarshalIndent(entries, "", "  ")
    outName := a.Name
    if outName == "" {
        outName = fmt.Sprintf("%04d-scroll-collect.json", r.step)
    }
    if werr := os.WriteFile(filepath.Join(r.cfg.ArtifactDir, outName), data, 0644); werr != nil {
        return werr
    }
    r.log.Say(fmt.Sprintf("uiauto: scroll_collect found %d unique controls", len(entries)))
    r.writeEvent("scroll_collect_done", map[string]interface{}{"total_controls": len(entries), "artifact": outName})
    return nil
}

func (r *Runner) runScene(ctx context.Context, s Scene, scenes map[string]Scene) error {
    d, err := r.detection(ctx, fmt.Sprintf("%04d-scene-%s.png", r.step, s.Name))
    if err != nil {
        return err
    }
    for _, t := range s.MatchText {
        if _, ok := selectOCR(d, t, "", ""); !ok {
            return fmt.Errorf("scene %s did not match text %q", s.Name, t)
        }
    }
    for _, m := range s.MatchControls {
        if _, ok := selectControl(d, m.Role, m.Label, m.Value, m.Region, m.Match, m.Selected, m.LabelContains, m.Style, m.Enabled); !ok {
            return fmt.Errorf("scene %s did not match control role=%q label=%q", s.Name, m.Role, m.Label)
        }
    }
    for _, a := range s.Actions {
        if err := r.execAction(ctx, a, scenes); err != nil {
            return err
        }
    }
    return nil
}

func (r *Runner) failureBundle(ctx context.Context) error {
    d, err := r.detection(ctx, "failure-last-screen.png")
    if err != nil {
        return err
    }
    b, _ := json.MarshalIndent(d.OCR, "", "  ")
    _ = os.WriteFile(filepath.Join(r.cfg.ArtifactDir, "failure-ocr.json"), b, 0644)
    b, _ = json.MarshalIndent(d.Controls, "", "  ")
    _ = os.WriteFile(filepath.Join(r.cfg.ArtifactDir, "failure-controls.json"), b, 0644)
    return nil
}
