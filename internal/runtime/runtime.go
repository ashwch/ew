package runtime

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var stdinIsInteractive = isStdinInteractive

func ShouldExecute(mode string, yes bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "suggest":
		return false, nil
	case "yolo":
		return true, nil
	case "confirm", "":
		if yes {
			return true, nil
		}
		if !stdinIsInteractive() {
			return false, fmt.Errorf("confirm mode requires interactive terminal; rerun with --yes or --mode yolo")
		}
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Run this command? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		return trimmed == "y" || trimmed == "yes", nil
	default:
		return false, fmt.Errorf("unknown mode: %s", mode)
	}
}

func RunCommand(command string) error {
	shell, args := shellCommandInvocation(command)
	cmd := exec.Command(shell, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func shellCommandInvocation(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		comspec := strings.TrimSpace(os.Getenv("COMSPEC"))
		if comspec == "" {
			comspec = "cmd"
		}
		return comspec, []string{"/C", command}
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell != "" {
		if filepath.IsAbs(shell) {
			if _, err := os.Stat(shell); err == nil {
				return shell, []string{"-lc", command}
			}
		} else if resolved, err := exec.LookPath(shell); err == nil {
			return resolved, []string{"-lc", command}
		}
	}
	return "sh", []string{"-lc", command}
}

func NormalizeCommand(command string) (string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", fmt.Errorf("command cannot be empty")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("command contains invalid null byte")
	}

	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	switch {
	case strings.HasPrefix(trimmed, "$ "):
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "$ "))
	case strings.HasPrefix(trimmed, "> "):
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "> "))
	}

	if trimmed == "" {
		return "", fmt.Errorf("command cannot be empty")
	}
	return trimmed, nil
}

func isStdinInteractive() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func HighRisk(command string) bool {
	low := strings.ToLower(strings.TrimSpace(command))
	highRiskPatterns := []string{
		"rm -rf",
		"mkfs",
		"dd if=",
		"shutdown",
		"reboot",
		"userdel",
		"chmod 777 /",
	}
	for _, pattern := range highRiskPatterns {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}

func SuggestFix(command string) (string, string) {
	trimmed := strings.TrimSpace(command)
	switch {
	case strings.HasPrefix(trimmed, "gti "):
		return strings.Replace(trimmed, "gti ", "git ", 1), "common typo: gti -> git"
	case strings.HasPrefix(trimmed, "sl "):
		return strings.Replace(trimmed, "sl ", "ls ", 1), "common typo: sl -> ls"
	case strings.HasPrefix(trimmed, "grpe "):
		return strings.Replace(trimmed, "grpe ", "grep ", 1), "common typo: grpe -> grep"
	case strings.Contains(trimmed, "aws-vault clear"):
		return "aws-vault remove --all", "aws-vault clear is often remove --all"
	default:
		return "", ""
	}
}
