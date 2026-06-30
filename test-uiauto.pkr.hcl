packer {
  required_plugins {
    tart = {
      source  = "github.com/peter1122999/tart-uiautomate"
      version = ">= 0.0.8"
    }
  }
}

variable "ipsw_url" {
  type    = string
  default = "/Users/Shared/OS265.ipsw"
}

variable "vm_name" {
  type    = string
  default = "macos26-local-build"
}

variable "admin_name" {
  type    = string
  default = "Administrator"
}

variable "admin_uname" {
  type    = string
  default = "admin"
}

variable "admin_password" {
  type      = string
  default   = "admin"
  sensitive = true
}

variable "want_apple_reset_enabled" {
  type    = bool
  default = false
}

source "tart-uiautomate-cli" "macos26_local" {
  vm_name      = var.vm_name
  from_ipsw    = var.ipsw_url
  cpu_count    = 4
  memory_gb    = 8
  disk_size_gb = 80
  display      = "1280x800"
  boot_command = []
}

build {
  name    = "macos26-zero-touch"
  sources = ["source.tart-uiautomate-cli.macos26_local"]

  provisioner "uiauto" {
    # Global runner config — Integration #8
    vcs_socket          = "/tmp/vcs.sock"
    vcs_max_height      = 720
    artifacts_dir       = "./ui-artifacts/${var.vm_name}"
    rescreenshot        = true
    expect_scene_change = true
    on_no_change        = "retry_click"
    max_retries         = 3
    inter_retry_wait    = "1500ms"
    scene_timeout       = "120s"

    # --- Linear scene dispatch ---
    # The plugin matches each step's `scene` against the live capture.
    # Order isn't strict; each step waits for its scene to appear.

    # Chevron-advance scenes — Integration #7
    step {
      scene = "Language"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene = "Country or Region"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene = "Written Language"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene = "Spoken Language"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene = "Wi-Fi"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene = "Data & Privacy"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    # Skip-with-Set-Up-Later scenes
    step {
      scene = "Migration Assistant"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Apple Account"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Terms and Conditions"
      click_control {
        role           = "button"
        label_contains = "Agree"
      }
    }

    # Create-a-Mac-Account — uses every VCS feature
    step {
      scene = "Create a Mac Account"

      # Integration #1 — selector-tag clicking
      type_text {
        text_field_by_position = "@full_name"
        text                   = var.admin_name
        clear_first            = true
      }

      type_text {
        text_field_by_position = "@account_name"
        text                   = var.admin_uname
        clear_first            = true
      }

      type_text {
        text_field_by_position = "@password"
        text                   = var.admin_password
        clear_first            = true
      }

      type_text {
        text_field_by_position = "@verify_password"
        text                   = var.admin_password
        clear_first            = true
      }

      # Integration #2 — wait for primary-styled Continue
      wait_for_button {
        label   = "Continue"
        style   = "primary"
        enabled = true
        timeout = "10s"
      }

      click_control {
        role  = "button"
        label = "Continue"
      }
    }

    step {
      scene = "Enable Location Services"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Analytics"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Screen Time"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Siri"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "FileVault Disk Encryption"
      click_control {
        role           = "button"
        label_contains = "Continue"
      }
    }

    step {
      scene = "Touch ID"
      click_control {
        role           = "button"
        label_contains = "Set Up Later"
      }
    }

    step {
      scene = "Choose Your Look"
      click_control {
        role  = "button"
        label = "→"
      }
    }

    step {
      scene     = "Welcome to your new Mac"
      exit_loop = true
    }
  }

  provisioner "shell" {
    inline = [
      "id ${var.admin_uname}",
      "dscl . -read /Users/${var.admin_uname} NFSHomeDirectory",
      "test -d /Users/${var.admin_uname}"
    ]
  }
}