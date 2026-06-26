package uiauto

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Logger interface {
	Say(message string)
}

type noopLogger struct{}
func (noopLogger) Say(string) {}

type Runner struct {
	cfg    *Config
	log    Logger
	vnc    *VNC
	cg     *CGTool
	trace  *os.File
	step   int
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
func (r *Runner) Capture(ctx context.Context, path string) error { if r.useCGTool() { return r.cg.Capture(ctx, path) }; return r.vnc.Capture(ctx, path) }
func (r *Runner) Move(ctx context.Context, x, y int) error { if r.useCGTool() { return r.cg.Move(ctx, x, y) }; return r.vnc.Move(ctx, x, y) }
func (r *Runner) Click(ctx context.Context, x, y int) error { if r.useCGTool() { return r.cg.Click(ctx, x, y) }; return r.vnc.Click(ctx, x, y) }
func (r *Runner) DoubleClick(ctx context.Context, x, y int) error { if r.useCGTool() { return r.cg.DoubleClick(ctx, x, y) }; return r.vnc.DoubleClick(ctx, x, y) }
func (r *Runner) Drag(ctx context.Context, x1, y1, x2, y2 int) error { if r.useCGTool() { return r.cg.Drag(ctx, x1, y1, x2, y2) }; return r.vnc.Drag(ctx, x1, y1, x2, y2) }
func (r *Runner) Scroll(ctx context.Context, dx, dy int) error { if r.useCGTool() { return r.cg.Scroll(ctx, dx, dy) }; return r.vnc.Scroll(ctx, dx, dy) }
func (r *Runner) TypeText(ctx context.Context, text string) error { if r.useCGTool() { return r.cg.TypeText(ctx, text) }; return r.vnc.TypeText(ctx, text) }

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

func (r *Runner) execAction(ctx context.Context, a Action, scenes map[string]Scene) error {
	r.step++
	if r.cfg.ScreenshotMode == "before_after_each_step" {
		_ = r.capture(ctx, fmt.Sprintf("%04d-before-%s.png", r.step, a.Type))
	}
	r.writeEvent("action_start", map[string]interface{}{"action": a})

	var err error
	switch a.Type {
	case "move":
		err = r.Move(ctx, a.X, a.Y)
	case "click":
		err = r.Click(ctx, a.X, a.Y)
	case "double_click":
		err = r.DoubleClick(ctx, a.X, a.Y)
	case "drag":
		err = r.Drag(ctx, a.X, a.Y, a.X2, a.Y2)
	case "scroll":
		err = r.Scroll(ctx, a.DX, a.DY)
	case "type":
		err = r.TypeText(ctx, a.Text)
	case "screenshot":
		if a.Name == "" {
			a.Name = fmt.Sprintf("%04d-screenshot.png", r.step)
		}
		err = r.capture(ctx, a.Name)
	case "click_text", "click_if_visible":
		var d *Detection
		d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
		if err != nil {
			break
		}
		if hit, ok := selectOCR(d, a.Text, a.Region, a.Match); ok {
			x, y := hit.BBox.Center()
			err = r.Click(ctx, x, y)
			r.writeEvent("click_text", map[string]interface{}{"text": hit.Text, "x": x, "y": y})
		} else if a.Type == "click_text" {
			err = fmt.Errorf("text not visible: %q", a.Text)
		}
	case "click_control":
		var d *Detection
		d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
		if err != nil {
			break
		}
		if hit, ok := selectControl(d, a.Role, a.Label, a.Value, a.Region, a.Match, nil); ok {
			x, y := hit.BBox.Center()
			err = r.Click(ctx, x, y)
			r.writeEvent("click_control", map[string]interface{}{"role": hit.Role, "label": hit.Label, "x": x, "y": y})
		} else {
			err = fmt.Errorf("control not visible: role=%q label=%q", a.Role, a.Label)
		}
	case "assert_control":
		var d *Detection
		d, err = r.detection(ctx, fmt.Sprintf("%04d-detect.png", r.step))
		if err != nil {
			break
		}
		if _, ok := selectControl(d, a.Role, a.Label, a.Value, a.Region, a.Match, a.Selected); !ok {
			err = fmt.Errorf("assertion failed for control: role=%q label=%q", a.Role, a.Label)
		}
	case "run_scene":
		s, ok := scenes[a.Scene]
		if !ok {
			err = fmt.Errorf("unknown scene: %s", a.Scene)
			break
		}
		err = r.runScene(ctx, s, scenes)
	case "cgtool":
		err = r.cg.Raw(ctx, a.Args)
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
		if _, ok := selectControl(d, m.Role, m.Label, m.Value, m.Region, m.Match, m.Selected); !ok {
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
