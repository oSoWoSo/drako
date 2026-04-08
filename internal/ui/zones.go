package ui

import "fmt"

func gridCellZone(row, col int) string         { return fmt.Sprintf("grid-%d-%d", row, col) }
func profileZone(i int) string                 { return fmt.Sprintf("profile-%d", i) }
func pathComponentZone(i int) string           { return fmt.Sprintf("path-component-%d", i) }
func pathChildZone(i int) string               { return fmt.Sprintf("path-child-%d", i) }
func inventoryListZone(listID int) string      { return fmt.Sprintf("inventory-list-%d", listID) }
func inventoryItemZone(listID, idx int) string { return fmt.Sprintf("inventory-item-%d-%d", listID, idx) }
func dropdownZone(i int) string                { return fmt.Sprintf("dropdown-%d", i) }

// markZone wraps v with a zone marker. Safe to call when m.zones is nil (e.g. in tests).
func (m Model) markZone(id, v string) string {
	if m.zones == nil {
		return v
	}
	return m.zones.Mark(id, v)
}
