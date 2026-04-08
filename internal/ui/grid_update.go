package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateGridMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle number-based navigation (1-9)
	if num, err := strconv.Atoi(key); err == nil && num >= 1 && num <= 9 {
		targetIndex := num - 1 // Convert to 0-based index

		if m.navigationTimer == nil { // This is the first number press (column selection)
			lastCol := len(m.grid[0]) - 1
			targetCol := min(targetIndex, lastCol)

			// Ensure the target column is valid before proceeding
			if targetCol < 0 {
				return m, nil
			}

			m.cursorCol = targetCol
			// Snap Row to first populated in this column
			m.cursorRow = 0
			for r := 0; r < len(m.grid); r++ {
				if strings.TrimSpace(m.grid[r][targetCol]) != "" {
					m.cursorRow = r
					break
				}
			}

			timeoutMs := m.Config.GridSelectionTimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = 500
			}
			m.navigationTimer = time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)

			return m, func() tea.Msg {
				<-m.navigationTimer.C
				return navTimeoutMsg{}
			}

		} else { // This is the second number press (row selection)
			m.navigationTimer.Stop()
			m.navigationTimer = nil

			lastRow := len(m.grid) - 1
			targetRow := min(targetIndex, lastRow)

			m.cursorRow = targetRow
			return m, nil
		}
	}

	// If a navigation sequence was in progress, any non-numeric key cancels it.
	if m.navigationTimer != nil {
		m.navigationTimer.Stop()
		m.navigationTimer = nil
	}

	switch {
	case IsQuit(m.Config.Keys, msg):
		m.Quitting = true
		return m, tea.Quit

	// Glassroot Cases - handled in update.go
	// ====================
	case IsInventory(m.Config.Keys, msg):
		m.mode = inventoryMode
		m.inventory = InitInventoryModel(m.configDir)
		return m, nil

	case IsPathGridMode(m.Config.Keys, msg):
		m.mode = pathMode
	// ====================

	case IsUp(m.Config.Keys, msg):
		m.moveCursor(-1, 0)
	case IsDown(m.Config.Keys, msg):
		m.moveCursor(1, 0)
	case IsLeft(m.Config.Keys, msg):
		m.moveCursor(0, -1)
	case IsRight(m.Config.Keys, msg):
		m.moveCursor(0, 1)
	case IsExplain(m.Config.Keys, msg):
		selectedChoice := m.grid[m.cursorRow][m.cursorCol]
		if strings.TrimSpace(selectedChoice) == "" {
			return m, nil
		}
		for _, cmd := range m.Config.Commands {
			if cmd.Name == selectedChoice {
				m.previousMode = m.mode

				// Resolve execution mode and auto-close
				autoClose := true
				if cmd.AutoCloseExecution != nil {
					autoClose = *cmd.AutoCloseExecution
				}
				debug := false
				if cmd.DebugExecution != nil {
					debug = *cmd.DebugExecution
				}
				execMode := "live"
				if debug {
					execMode = "debug"
				}

				cmdStr := ""
				if strings.TrimSpace(cmd.Command) == "" {
					cmdStr = "Error: no command. ( This might be a folder of commands!)"
				} else {
					cmdStr = cmd.Command
				}

				m.activeDetail = &DetailState{
					Title:       selectedChoice,
					KeyLabel:    "Command",
					Value:       cmdStr,
					Description: cmd.Description,
					Meta: []DetailMeta{
						{Label: "Exec", Value: execMode},
						{Label: "Auto-close", Value: fmt.Sprintf("%v", autoClose)},
						{Label: "CWD", Value: m.path.CurrentPath},
					},
				}
				m.mode = infoMode
				return m, nil
			}
		}
		// Not found in config
		m.previousMode = m.mode
		m.activeDetail = &DetailState{
			Title:       selectedChoice,
			KeyLabel:    "Command",
			Value:       "Error: command not found",
			Description: "",
			Meta: []DetailMeta{
				{Label: "CWD", Value: m.path.CurrentPath},
			},
		}
		m.mode = infoMode
		return m, nil
	case IsConfirm(m.Config.Keys, msg):
		selectedChoice := m.grid[m.cursorRow][m.cursorCol]

		// Special handling for Exit Rescue Mode command
		if selectedChoice == "Exit Rescue Mode" {
			// Reset to Core profile (index 0)
			if updated, cmd, ok := m.switchToProfileIndex(0); ok {
				m = updated
				return m, cmd
			}
			return m, nil
		}

		if selectedChoice != "" {
			// Check if this command has dropdown items
			for _, cmd := range m.Config.Commands {
				if cmd.Name == selectedChoice {
					if len(cmd.Items) > 0 {
						// Open dropdown menu
						m.mode = dropdownMode
						m.dropdownRow = m.cursorRow
						m.dropdownCol = m.cursorCol
						m.dropdownItems = cmd.Items
						m.dropdownSelectedIdx = 0
						return m, nil
					}
					break
				}
			}
			// Single command, execute normally
			m.Selected = selectedChoice
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) moveCursor(rowDir, colDir int) {
	bestRow, bestCol := -1, -1
	minDist := math.MaxFloat64

	for r, row := range m.grid {
		for c, val := range row {
			if val == "" || (r == m.cursorRow && c == m.cursorCol) {
				continue
			}

			rowDiff := r - m.cursorRow
			colDiff := c - m.cursorCol
			isCorrectDirection := false
			if rowDir > 0 && rowDiff > 0 {
				isCorrectDirection = true
			}
			if rowDir < 0 && rowDiff < 0 {
				isCorrectDirection = true
			}
			if colDir > 0 && colDiff > 0 {
				isCorrectDirection = true
			}
			if colDir < 0 && colDiff < 0 {
				isCorrectDirection = true
			}

			if isCorrectDirection {
				dist := math.Sqrt(math.Pow(float64(rowDiff), 2) + math.Pow(float64(colDiff), 2))
				if dist < minDist {
					minDist = dist
					bestRow, bestCol = r, c
				}
			}
		}
	}

	if bestRow != -1 {
		m.cursorRow = bestRow
		m.cursorCol = bestCol
	}
}
func (m Model) resolveMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	if m.mode == dropdownMode {
		return m.resolveDropdownMouseClick(msg)
	}

	if m.mode == pathMode || m.mode == pickerMode {
		return m.resolvePathMouseClick(msg)
	}

	if m.mode == inventoryMode {
		return m.resolveInventoryMouseClick(msg)
	}

	if m.mode == infoMode {
		m.mode = m.previousMode
		m.activeDetail = nil
		return m, nil
	}

	if m.zones == nil {
		return m, nil
	}

	// Profile button zones
	for i := range m.profiles {
		if m.zones.Get(profileZone(i)).InBounds(msg) {
			if updated, cmd, ok := m.switchToProfileIndex(i); ok {
				return updated, cmd
			}
			return m, nil
		}
	}

	// Grid cell zones
	for r := range m.grid {
		for c := range m.grid[r] {
			if m.zones.Get(gridCellZone(r, c)).InBounds(msg) {
				if c == m.cursorCol && r == m.cursorRow {
					return m.updateGridMode(tea.KeyMsg{Type: tea.KeyEnter})
				}
				m.cursorCol = c
				m.cursorRow = r
				return m, nil
			}
		}
	}

	// Path component zones (enter path mode from grid)
	for i := range m.path.PathComponents {
		if m.zones.Get(pathComponentZone(i)).InBounds(msg) {
			m.mode = pathMode
			m.path.SelectedPathIndex = i
			m.path.ListChildDirs()
			return m, nil
		}
	}

	return m, nil
}

func (m Model) resolveDropdownMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	if m.zones == nil {
		return m, nil
	}

	for i, item := range m.dropdownItems {
		if m.zones.Get(dropdownZone(i)).InBounds(msg) {
			m.Selected = item.Name
			m.dropdownItems = nil
			return m, tea.Quit
		}
	}

	// Click outside the popup dismisses it
	m.mode = gridMode
	m.dropdownItems = nil
	return m, nil
}

// resolvePathMouseClick handles mouse clicks in path mode and child mode
func (m Model) resolvePathMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.zones == nil {
		return m, nil
	}

	// Path component zones
	for i := range m.path.PathComponents {
		if m.zones.Get(pathComponentZone(i)).InBounds(msg) {
			now := time.Now()
			clickDelta := now.Sub(m.lastClickTime)
			isDoubleClick := clickDelta < 500*time.Millisecond &&
				msg.X == m.lastClickPos.x && msg.Y == m.lastClickPos.y
			m.lastClickTime = now
			m.lastClickPos.x = msg.X
			m.lastClickPos.y = msg.Y

			if m.mode == pathMode && isDoubleClick {
				targetPath := m.path.BuildPathFromComponents(i)
				if err := os.Chdir(targetPath); err == nil {
					m.path.CurrentPath, _ = os.Getwd()
					m.mode = gridMode
					return m, func() tea.Msg { return pathChangedMsg{} }
				}
				m.mode = gridMode
				return m, nil
			}
			m.path.SelectedPathIndex = i
			m.path.ListChildDirs()
			if m.mode != pathMode {
				m.mode = pathMode
			}
			return m, nil
		}
	}

	// Child directory zones
	for i := range m.path.ChildDirs {
		if m.zones.Get(pathChildZone(i)).InBounds(msg) {
			now := time.Now()
			clickDelta := now.Sub(m.lastClickTime)
			isDoubleClick := clickDelta < 500*time.Millisecond &&
				msg.X == m.lastClickPos.x && msg.Y == m.lastClickPos.y
			m.lastClickTime = now
			m.lastClickPos.x = msg.X
			m.lastClickPos.y = msg.Y

			if isDoubleClick {
				parentPath := m.path.BuildPathFromComponents(m.path.SelectedPathIndex)
				targetPath := filepath.Join(parentPath, m.path.ChildDirs[i])
				if err := os.Chdir(targetPath); err == nil {
					m.path.CurrentPath, _ = os.Getwd()
					m.path.UpdatePathComponents()
					m.path.ListChildDirs()
					m.path.SelectedChildIndex = 0
					m.mode = gridMode
					return m, func() tea.Msg { return pathChangedMsg{} }
				}
			}

			m.path.SelectedChildIndex = i
			if m.mode == pathMode {
				m.mode = pickerMode
			}
			return m, nil
		}
	}

	// Click outside known zones while in path/picker — return to grid
	m.mode = gridMode
	return m, nil
}

// ChangeDir is a helper to change directory and update path model
func (pm *PathModel) ChangeDir(targetPath string) error {
	if err := os.Chdir(targetPath); err != nil {
		return err
	}
	pm.CurrentPath, _ = os.Getwd()
	return nil
}
