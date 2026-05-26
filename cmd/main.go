package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const appTitle = "Таймер выключения"

func main() {
	if runtime.GOOS == "linux" && !commandExists("zenity") {
		fmt.Println("Ошибка: для GUI нужен пакет zenity")
		fmt.Println("Установи его командой: sudo apt install zenity")
		return
	}

	minutes, ok := askMinutes()
	if !ok {
		return
	}

	canceled, err := runTimer(minutes)
	if canceled {
		showInfo("Таймер отменён")
		return
	}
	if err != nil {
		showError("Ошибка таймера: " + err.Error())
		return
	}

	if err := shutdown(); err != nil {
		showError("Ошибка выключения: " + err.Error())
	}
}

func askMinutes() (int, bool) {
	for {
		output, err := runZenityOutput(
			"--entry",
			"--title="+appTitle,
			"--text=Через сколько минут выключить ноутбук?",
			"--entry-text=30",
			"--width=380",
		)
		if err != nil {
			return 0, false
		}

		minutes, parseErr := parseMinutes(output)
		if parseErr == nil {
			return minutes, true
		}

		showError(parseErr.Error())
	}
}

func parseMinutes(value string) (int, error) {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || minutes <= 0 {
		return 0, fmt.Errorf("введи число минут больше нуля")
	}
	if time.Duration(minutes) > math.MaxInt64/time.Minute {
		return 0, fmt.Errorf("слишком большое значение минут")
	}

	return minutes, nil
}

func runTimer(minutes int) (bool, error) {
	totalSeconds := minutes * 60
	if totalSeconds <= 0 {
		return false, fmt.Errorf("неверное время таймера")
	}

	cmd := exec.Command(
		"zenity",
		"--progress",
		"--title="+appTitle,
		"--text=Ноутбук выключится через "+strconv.Itoa(minutes)+" мин.",
		"--percentage=0",
		"--auto-close",
		"--width=420",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false, err
	}

	if err := cmd.Start(); err != nil {
		return false, err
	}

	writer := bufio.NewWriter(stdin)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for elapsed := 0; elapsed <= totalSeconds; elapsed++ {
		remaining := totalSeconds - elapsed
		percentage := elapsed * 100 / totalSeconds

		if _, err := fmt.Fprintf(writer, "%d\n# Осталось: %s\n", percentage, formatSeconds(remaining)); err != nil {
			return waitProgress(cmd, true)
		}
		if err := writer.Flush(); err != nil {
			return waitProgress(cmd, true)
		}

		if elapsed == totalSeconds {
			break
		}

		<-ticker.C
	}

	_, _ = io.WriteString(stdin, "100\n")
	_ = stdin.Close()

	return waitProgress(cmd, false)
}

func waitProgress(cmd *exec.Cmd, maybeCanceled bool) (bool, error) {
	err := cmd.Wait()
	if err == nil {
		return false, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && maybeCanceled {
		return true, nil
	}

	return false, err
}

func formatSeconds(total int) string {
	if total < 0 {
		total = 0
	}

	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func showInfo(message string) {
	_ = runZenity(
		"--info",
		"--title="+appTitle,
		"--text="+message,
		"--width=320",
	)
}

func showError(message string) {
	_ = runZenity(
		"--error",
		"--title="+appTitle,
		"--text="+message,
		"--width=360",
	)
}

func runZenity(args ...string) error {
	return exec.Command("zenity", args...).Run()
}

func runZenityOutput(args ...string) (string, error) {
	output, err := exec.Command("zenity", args...).Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func shutdown() error {
	switch runtime.GOOS {
	case "windows":
		return runShutdownCommand("shutdown", "/s", "/t", "0")
	case "linux":
		return runShutdownCommand("systemctl", "poweroff")
	case "darwin":
		return runShutdownCommand("osascript", "-e", `tell application "System Events" to shut down`)
	default:
		return fmt.Errorf("неподдерживаемая система: %s", runtime.GOOS)
	}
}

func runShutdownCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}

	return fmt.Errorf("%w: %s", err, message)
}
