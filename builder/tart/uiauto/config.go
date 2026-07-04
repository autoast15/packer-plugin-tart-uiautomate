//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Action,Scene

package uiauto

import "fmt"

type Action struct {
    Type                string   `mapstructure:"type" json:"type"`
    Name                string   `mapstructure:"name" json:"name"`
    X                   int      `mapstructure:"x" json:"x"`
    Y                   int      `mapstructure:"y" json:"y"`
    X2                  int      `mapstructure:"x2" json:"x2"`
    Y2                  int      `mapstructure:"y2" json:"y2"`
    DX                  int      `mapstructure:"dx" json:"dx"`
    DY                  int      `mapstructure:"dy" json:"dy"`
    Text                string   `mapstructure:"text" json:"text"`
    Key                 string   `mapstructure:"key" json:"key"`
    Hotkey              string   `mapstructure:"hotkey" json:"hotkey"`
    Role                string   `mapstructure:"role" json:"role"`
    Label               string   `mapstructure:"label" json:"label"`
    LabelContains       string   `mapstructure:"label_contains" json:"label_contains"`
    Value               string   `mapstructure:"value" json:"value"`
    Selected            *bool    `mapstructure:"selected" json:"selected"`
    Enabled             *bool    `mapstructure:"enabled" json:"enabled"`
    Style               string   `mapstructure:"style" json:"style"`
    TextFieldByPosition string   `mapstructure:"text_field_by_position" json:"text_field_by_position"`
    ClearFirst          bool     `mapstructure:"clear_first" json:"clear_first"`
    Region              string   `mapstructure:"region" json:"region"`
    Match               string   `mapstructure:"match" json:"match"`
    // OffsetX/OffsetY shift a click_text click away from the matched
    // text's own center, in the same window-relative coordinate space
    // click/click_control/click_text already use. For clicking a
    // control that sits a fixed distance from a label that OCRs
    // reliably but whose own detection is unreliable (e.g. a toggle
    // switch to the right of a Settings row's text) — see cgtool-project
    // memory (2026-07-03, click_text offset) for why this exists instead
    // of hardcoded absolute pixel positions.
    OffsetX             int      `mapstructure:"offset_x" json:"offset_x"`
    OffsetY             int      `mapstructure:"offset_y" json:"offset_y"`
    Scene               string   `mapstructure:"scene" json:"scene"`
    TimeoutSeconds      int      `mapstructure:"timeout_seconds" json:"timeout_seconds"`
    Args                []string `mapstructure:"args" json:"args"`
    // MaxScrolls bounds scroll_collect's scroll loop — without a cap a
    // pane whose content never stops changing (a live clock, a spinner)
    // would scroll forever since "no new controls" would never trigger.
    MaxScrolls          int      `mapstructure:"max_scrolls" json:"max_scrolls"`
}

type Scene struct {
    Name          string   `mapstructure:"name" json:"name"`
    MatchText     []string `mapstructure:"match_text" json:"match_text"`
    MatchControls []Action `mapstructure:"match_controls" json:"match_controls"`
    Actions       []Action `mapstructure:"actions" json:"actions"`
}

type Config struct {
    Enabled         bool     `mapstructure:"enabled" json:"enabled"`
    VNCHost         string   `mapstructure:"vnc_host" json:"vnc_host"`
    VNCPort         int      `mapstructure:"vnc_port" json:"vnc_port"`
    VNCPassword     string   `mapstructure:"vnc_password" json:"vnc_password"`
    VNCDoPath       string   `mapstructure:"vncdo_path" json:"vncdo_path"`
    CGToolPath      string   `mapstructure:"cgtool_path" json:"cgtool_path"`
    UIBackend       string   `mapstructure:"ui_backend" json:"ui_backend"`
    ArtifactDir     string   `mapstructure:"artifact_dir" json:"artifact_dir"`
    ScreenshotMode  string   `mapstructure:"screenshot_mode" json:"screenshot_mode"`
    WindowTitle     string   `mapstructure:"window_title" json:"window_title"`
    DetectorCommand []string `mapstructure:"detector_command" json:"detector_command"`
    Actions         []Action `mapstructure:"actions" json:"actions"`
    Scenes          []Scene  `mapstructure:"scenes" json:"scenes"`
}

func (c *Config) PrepareDefaults() error {
    if c == nil {
        return nil
    }
    if c.VNCHost == "" {
        c.VNCHost = "127.0.0.1"
    }
    if c.VNCPort == 0 {
        c.VNCPort = 5900
    }
    if c.VNCDoPath == "" {
        c.VNCDoPath = "vncdo"
    }
    if c.CGToolPath == "" {
        c.CGToolPath = "cgtool"
    }
    if c.UIBackend == "" {
        c.UIBackend = "cgtool"
    }
    if c.ArtifactDir == "" {
        c.ArtifactDir = "ui-artifacts"
    }
    if c.ScreenshotMode == "" {
        c.ScreenshotMode = "before_after_each_step"
    }
    if c.UIBackend != "vnc" && c.UIBackend != "cgtool" {
        return fmt.Errorf("ui_automation.ui_backend must be vnc or cgtool")
    }
    return nil
}
