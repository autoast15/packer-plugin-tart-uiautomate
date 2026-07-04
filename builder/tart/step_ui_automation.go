package tart

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"github.com/peter1122999/packer-plugin-tart-uiautomate/builder/tart/uiauto"
)

type StepUIAutomation struct {
	Config *uiauto.Config
}

func (s *StepUIAutomation) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	if s.Config == nil || !s.Config.Enabled {
		return multistep.ActionContinue
	}

	uiRaw, ok := state.GetOk("ui")
	if !ok {
		state.Put("error", fmt.Errorf("ui automation failed: missing packer UI in state bag"))
		return multistep.ActionHalt
	}

	ui, ok := uiRaw.(packer.Ui)
	if !ok {
		state.Put("error", fmt.Errorf("ui automation failed: state bag ui is not packer.Ui"))
		return multistep.ActionHalt
	}

	if err := s.Config.PrepareDefaults(); err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}

	// Adopt the live VNC endpoint stepRun captured from tart stdout so
	// vnc_* actions reach this boot's VNC server. An explicit vnc_password
	// in HCL wins over the discovered one.
	if host, ok := state.Get("vnc-host").(string); ok && host != "" {
		s.Config.VNCHost = host
	}
	if port, ok := state.Get("vnc-port").(int); ok && port != 0 {
		s.Config.VNCPort = port
	}
	if password, ok := state.Get("vnc-password").(string); ok && password != "" && s.Config.VNCPassword == "" {
		s.Config.VNCPassword = password
	}
	if s.Config.VNCPassword != "" {
		ui.Say(fmt.Sprintf("ui_automation: VNC input endpoint %s:%d (password set)",
			s.Config.VNCHost, s.Config.VNCPort))
	}

	ui.Say("Running ui_automation before SSH wait...")

	if err := uiauto.NewRunner(s.Config, ui).Run(ctx); err != nil {
		state.Put("error", fmt.Errorf("ui_automation failed: %w", err))
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *StepUIAutomation) Cleanup(state multistep.StateBag) {}
