package uiauto

import "fmt"

type Action struct {
	Type           string   `mapstructure:"type"`
	Name           string   `mapstructure:"name"`
	X              int      `mapstructure:"x"`
	Y              int      `mapstructure:"y"`
	X2             int      `mapstructure:"x2"`
	Y2             int      `mapstructure:"y2"`
	DX             int      `mapstructure:"dx"`
	DY             int      `mapstructure:"dy"`
	Text           string   `mapstructure:"text"`
	Role           string   `mapstructure:"role"`
	Label          string   `mapstructure:"label"`
	Value          string   `mapstructure:"value"`
	Selected       *bool    `mapstructure:"selected"`
	Region         string   `mapstructure:"region"`
	Match          string   `mapstructure:"match"`
	Scene          string   `mapstructure:"scene"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
	Args           []string `mapstructure:"args"`
}

type Scene struct {
	Name          string   `mapstructure:"name"`
	MatchText     []string `mapstructure:"match_text"`
	MatchControls []Action `mapstructure:"match_controls"`
	Actions       []Action `mapstructure:"actions"`
}

type Config struct {
	Enabled         bool     `mapstructure:"enabled"`
	VNCHost         string   `mapstructure:"vnc_host"`
	VNCPort         int      `mapstructure:"vnc_port"`
	VNCPassword     string   `mapstructure:"vnc_password"`
	VNCDoPath       string   `mapstructure:"vncdo_path"`
	CGToolPath      string   `mapstructure:"cgtool_path"`
	UIBackend       string   `mapstructure:"ui_backend"`
	ArtifactDir     string   `mapstructure:"artifact_dir"`
	ScreenshotMode  string   `mapstructure:"screenshot_mode"`
	DetectorCommand []string `mapstructure:"detector_command"`
	Actions         []Action `mapstructure:"actions"`
	Scenes          []Scene  `mapstructure:"scenes"`
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
