package uiauto

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"
)

func detect(ctx context.Context, cfg *Config, screenshot string) (*Detection, error) {
    if len(cfg.DetectorCommand) == 0 {
        return &Detection{}, nil
    }
    args := append([]string{}, cfg.DetectorCommand[1:]...)
    args = append(args, screenshot)
    cmd := exec.CommandContext(ctx, cfg.DetectorCommand[0], args...)
    out, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("detector failed: %w", err)
    }
    var d Detection
    if err := json.NewDecoder(bytes.NewReader(out)).Decode(&d); err != nil {
        return nil, fmt.Errorf("invalid detector JSON: %w", err)
    }
    return &d, nil
}

func containsFold(s, sub string) bool {
    return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func regionOK(screen Screen, r Rect, region string) bool {
    if region == "" || region == "any" {
        return true
    }
    cx, cy := r.Center()
    w, h := screen.Width, screen.Height
    if w == 0 || h == 0 {
        return true
    }
    switch region {
    case "top":
        return cy < h/3
    case "center":
        return cx >= w/3 && cx <= 2*w/3 && cy >= h/3 && cy <= 2*h/3
    case "bottom":
        return cy > 2*h/3
    case "bottom-right":
        return cx > 2*w/3 && cy > 2*h/3
    case "bottom-left":
        return cx < w/3 && cy > 2*h/3
    case "top-right":
        return cx > 2*w/3 && cy < h/3
    case "top-left":
        return cx < w/3 && cy < h/3
    default:
        return true
    }
}

func selectOCR(d *Detection, text, region, match string) (OCRItem, bool) {
    var hits []OCRItem
    for _, o := range d.OCR {
        if containsFold(o.Text, text) && regionOK(d.Screen, o.BBox, region) {
            hits = append(hits, o)
        }
    }
    if len(hits) == 0 {
        return OCRItem{}, false
    }
    if match == "last" {
        return hits[len(hits)-1], true
    }
    return hits[0], true
}

func selectControl(
    d *Detection,
    role, label, value, region, match string,
    selected *bool,
    labelContains, style string,
    enabled *bool,
) (Control, bool) {
    var hits []Control
    for _, c := range d.Controls {
        if role != "" && !strings.EqualFold(c.Role, role) {
            continue
        }
        if label != "" && !strings.EqualFold(c.Label, label) {
            continue
        }
        if labelContains != "" && !containsFold(c.Label, labelContains) {
            continue
        }
        if value != "" && !containsFold(c.Value, value) {
            continue
        }
        if style != "" && !strings.EqualFold(c.Style, style) {
            continue
        }
        if selected != nil && (c.Selected == nil || *c.Selected != *selected) {
            continue
        }
        if enabled != nil && (c.Enabled == nil || *c.Enabled != *enabled) {
            continue
        }
        if !regionOK(d.Screen, c.BBox, region) {
            continue
        }
        hits = append(hits, c)
    }
    if len(hits) == 0 {
        return Control{}, false
    }
    if match == "last" {
        return hits[len(hits)-1], true
    }
    return hits[0], true
}

func selectTextFieldBySelector(d *Detection, selector string) (Control, bool) {
    s := strings.TrimPrefix(strings.TrimSpace(selector), "@")
    if s == "" {
        return Control{}, false
    }
    needle := "[@" + s + "]"
    for _, c := range d.Controls {
        if !strings.EqualFold(c.Role, "textfield") {
            continue
        }
        if containsFold(c.Label, needle) {
            return c, true
        }
    }
    for _, c := range d.Controls {
        if strings.EqualFold(c.Role, "textfield") &&
            containsFold(c.Label, s) {
            return c, true
        }
    }
    return Control{}, false
}

func selectScene(d *Detection, want string) bool {
    w := strings.TrimSpace(want)
    if w == "" {
        return true
    }
    return strings.EqualFold(strings.TrimSpace(d.Scene), w)
}

func sanitize(s string) string {
    return strings.Map(func(r rune) rune {
        switch r {
        case ' ', '/', '\\', ':', '?', '*', '"', '<', '>', '|':
            return '-'
        }
        return r
    }, s)
}
