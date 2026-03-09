package pcsdownload

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
)

// ProgressManager maintains a list of concurrent download tasks and renders their progress
// at the bottom of the terminal without scrolling the command history.
type ProgressManager struct {
	mu            sync.Mutex
	activeTasks   map[string]string
	taskIDs       []string
	lastLineCount int
	isTerminal    bool
	enabled       bool
}

var (
	globalProgressManager *ProgressManager
	pmOnce                sync.Once
)

// GetProgressManager returns the singleton instance of ProgressManager and initializes signal handling.
func GetProgressManager() *ProgressManager {
	pmOnce.Do(func() {
		globalProgressManager = &ProgressManager{
			activeTasks: make(map[string]string),
			isTerminal:  isatty.IsTerminal(os.Stdout.Fd()),
		}
		if globalProgressManager.isTerminal {
			globalProgressManager.listenResize()
		}
	})
	return globalProgressManager
}

// listenResize sets up a listener for terminal resize events to reset state and ensure clean redraws.
func (pm *ProgressManager) listenResize() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, unix.SIGWINCH)
	go func() {
		for range sigChan {
			pm.mu.Lock()
			// Reset lastLineCount on resize as physical line wrapping may have changed,
			// making relative cursor movement unreliable.
			pm.lastLineCount = 0
			pm.mu.Unlock()
		}
	}()
}

// SetEnabled activates the dynamic dashboard if the output is a terminal.
func (pm *ProgressManager) SetEnabled(enabled bool) {
	pm.enabled = enabled && pm.isTerminal
}

// Update records or updates the progress string for a specific task and triggers a redraw.
func (pm *ProgressManager) Update(id string, status string) {
	if !pm.enabled {
		fmt.Print(status)
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.activeTasks[id]; !ok {
		pm.taskIDs = append(pm.taskIDs, id)
		sort.Strings(pm.taskIDs)
	}
	pm.activeTasks[id] = status
	pm.draw()
}

// Remove cleans up a finished task's entry from the dashboard.
func (pm *ProgressManager) Remove(id string) {
	if !pm.enabled {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.activeTasks[id]; ok {
		delete(pm.activeTasks, id)
		newIDs := make([]string, 0, len(pm.taskIDs)-1)
		for _, tid := range pm.taskIDs {
			if tid != id {
				newIDs = append(newIDs, tid)
			}
		}
		pm.taskIDs = newIDs
		pm.draw()
	}
}

// Printf displays log messages above the dynamic progress dashboard.
func (pm *ProgressManager) Printf(format string, a ...interface{}) {
	if !pm.enabled {
		fmt.Printf(format, a...)
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear the existing dashboard area before printing log messages.
	if pm.lastLineCount > 0 {
		fmt.Printf("\033[%dA\033[J", pm.lastLineCount)
	}

	fmt.Printf(format, a...)

	// Reset tracking and redraw the dashboard at the new bottom position.
	pm.lastLineCount = 0
	pm.draw()
}

// getTermWidth returns the current width of the terminal.
func (pm *ProgressManager) getTermWidth() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 80 // Default fallback
	}
	return int(ws.Col)
}

// draw performs the actual rendering of the dynamic progress lines.
func (pm *ProgressManager) draw() {
	// Move cursor up to the dashboard start position.
	if pm.lastLineCount > 0 {
		fmt.Printf("\033[%dA", pm.lastLineCount)
	}

	// Use \033[J to clear everything from the current cursor position to the bottom.
	fmt.Print("\033[J")

	width := pm.getTermWidth()
	totalLines := 0
	for _, id := range pm.taskIDs {
		str, ok := pm.activeTasks[id]
		if !ok {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(str, "\n"), "\n")
		for _, line := range lines {
			fmt.Print("\r")
			// Truncate line to fit terminal width to avoid wrapping.
			if runewidth.StringWidth(line) > width {
				line = runewidth.Truncate(line, width, "...")
			}
			fmt.Println(line)
			totalLines++
		}
	}

	pm.lastLineCount = totalLines
	os.Stdout.Sync()
}
