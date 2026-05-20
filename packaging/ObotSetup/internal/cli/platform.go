package cli

import "runtime"

func DefaultPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/usr/local/bin/obot"
	case "windows":
		// TODO(g-linville): should be user-specific AppData folder
		return `C:\Program Files\Obot\obot.exe`
	default:
		return "obot"
	}
}
