package pcsdownload

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"os"
	"sort"
	"strings"
	"sync"
)

// ProgressManager 管理下载进度显示, 实现动态面板效果
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

// GetProgressManager 获取ProgressManager单例
func GetProgressManager() *ProgressManager {
	pmOnce.Do(func() {
		globalProgressManager = &ProgressManager{
			activeTasks: make(map[string]string),
			isTerminal:  isatty.IsTerminal(os.Stdout.Fd()),
		}
	})
	return globalProgressManager
}

// SetEnabled 设置是否启用动态面板
func (pm *ProgressManager) SetEnabled(enabled bool) {
	pm.enabled = enabled && pm.isTerminal
}

// Update 更新任务进度字符串并重绘面板
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

// Remove 从面板中移除任务并重绘
func (pm *ProgressManager) Remove(id string) {
	if !pm.enabled {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.activeTasks[id]; ok {
		delete(pm.activeTasks, id)
		for i, tid := range pm.taskIDs {
			if tid == id {
				pm.taskIDs = append(pm.taskIDs[:i], pm.taskIDs[i+1:]...)
				break
			}
		}
		pm.draw()
	}
}

// Printf 在动态面板上方打印信息
func (pm *ProgressManager) Printf(format string, a ...interface{}) {
	if !pm.enabled {
		fmt.Printf(format, a...)
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 向上移动并清除之前的面板行
	if pm.lastLineCount > 0 {
		fmt.Printf("\033[%dA", pm.lastLineCount)
		for i := 0; i < pm.lastLineCount; i++ {
			fmt.Print("\033[2K\n")
		}
		fmt.Printf("\033[%dA", pm.lastLineCount)
	}

	// 打印新消息
	fmt.Printf(format, a...)

	// 重置 lastLineCount，因为我们已经物理上清除了面板行并移动了光标
	// draw() 会根据当前状态重新绘制并设置正确的 lastLineCount
	pm.lastLineCount = 0
	pm.draw()
}

// draw 绘制面板的具体逻辑
func (pm *ProgressManager) draw() {
	// 向上移动到面板起始位置
	if pm.lastLineCount > 0 {
		fmt.Printf("\033[%dA", pm.lastLineCount)
	}

	totalLines := 0
	for _, id := range pm.taskIDs {
		str := pm.activeTasks[id]

		// 处理可能的多行状态(如启用status输出时)
		lines := strings.Split(strings.TrimSuffix(str, "\n"), "\n")
		for _, line := range lines {
			fmt.Print("\033[2K\r") // 清除当前行并回到行首
			fmt.Print(line)
			fmt.Print("\n")
			totalLines++
		}
	}

	// 如果当前行数少于上次, 清除多余的旧行
	if totalLines < pm.lastLineCount {
		diff := pm.lastLineCount - totalLines
		for i := 0; i < diff; i++ {
			fmt.Print("\033[2K") // 清除当前行
			fmt.Print("\n")
		}
		// 回到当前面板末尾
		fmt.Printf("\033[%dA", diff)
	}
	pm.lastLineCount = totalLines
	os.Stdout.Sync() // 刷新输出
}
