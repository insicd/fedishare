package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrCanceled means the user closed the folder dialog without choosing.
var ErrCanceled = errors.New("folder selection canceled")

// ChooseFolder opens the operating system's folder picker and returns
// the absolute path. start is an optional directory to show first.
func ChooseFolder(start string) (string, error) {
	start = existingDir(start)
	var (
		path string
		err  error
	)
	switch runtime.GOOS {
	case "darwin":
		path, err = chooseFolderDarwin(start)
	case "windows":
		path, err = chooseFolderWindows(start)
	default:
		path, err = chooseFolderLinux(start)
	}
	if err != nil {
		return "", err
	}
	return CleanFolderPath(path)
}

// CleanFolderPath trims a dialog result into an absolute directory path.
func CleanFolderPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	path = strings.Trim(path, `"'`)
	path = strings.TrimRight(path, "\r\n")
	if path == "" {
		return "", ErrCanceled
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	return filepath.Clean(path), nil
}

func existingDir(start string) string {
	start = strings.TrimSpace(start)
	if start == "" {
		return ""
	}
	info, err := os.Stat(start)
	if err == nil && info.IsDir() {
		return start
	}
	parent := filepath.Dir(start)
	if info, err := os.Stat(parent); err == nil && info.IsDir() {
		return parent
	}
	return ""
}

func chooseFolderDarwin(start string) (string, error) {
	script := `POSIX path of (choose folder with prompt "Choose the folder FediShare should share")`
	if start != "" {
		script = fmt.Sprintf(
			`POSIX path of (choose folder with prompt "Choose the folder FediShare should share" default location POSIX file "%s")`,
			escapeAppleScript(start),
		)
	}
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if isCanceled(err, out) {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("folder dialog: %w", err)
	}
	return string(out), nil
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func chooseFolderWindows(start string) (string, error) {
	ps := `
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Choose the folder FediShare should share'
$d.ShowNewFolderButton = $true
`
	if start != "" {
		ps += fmt.Sprintf("$d.SelectedPath = '%s'\n", strings.ReplaceAll(start, `'`, `''`))
	}
	ps += `
$r = $d.ShowDialog()
if ($r -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) } else { exit 2 }
`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-Command", ps)
	out, err := cmd.Output()
	if err != nil {
		if isCanceled(err, out) || exitCode(err) == 2 {
			return "", ErrCanceled
		}
		return "", fmt.Errorf("folder dialog: %w", err)
	}
	return string(out), nil
}

func chooseFolderLinux(start string) (string, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title=Choose the folder FediShare should share"}
		if start != "" {
			args = append(args, "--filename="+start+"/")
		}
		out, err := exec.Command("zenity", args...).Output()
		if err != nil {
			if isCanceled(err, out) || exitCode(err) == 1 {
				return "", ErrCanceled
			}
			return "", fmt.Errorf("folder dialog: %w", err)
		}
		return string(out), nil
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		startDir := start
		if startDir == "" {
			startDir = "."
		}
		out, err := exec.Command("kdialog", "--getexistingdirectory", startDir).Output()
		if err != nil {
			if isCanceled(err, out) || exitCode(err) == 1 {
				return "", ErrCanceled
			}
			return "", fmt.Errorf("folder dialog: %w", err)
		}
		return string(out), nil
	}
	return "", fmt.Errorf("no folder dialog found (install zenity or kdialog)")
}

func isCanceled(err error, out []byte) bool {
	low := strings.ToLower(string(out) + " " + err.Error())
	var x *exec.ExitError
	if errors.As(err, &x) {
		low += " " + strings.ToLower(string(x.Stderr))
	}
	return strings.Contains(low, "cancel")
}

func exitCode(err error) int {
	var x *exec.ExitError
	if errors.As(err, &x) {
		return x.ExitCode()
	}
	return -1
}
