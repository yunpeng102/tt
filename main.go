package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/gdamore/tcell/v2"
	_ "github.com/mattn/go-sqlite3"
)

type Task struct {
	ID      string
	Content string
	SPOC    string
	State   string
	Created string
	Closed  sql.NullString
}

func initDB() (*sql.DB, error) {
	// Open SQLite database
	db, err := sql.Open("sqlite3", "./mybase.db")
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	// Execute the embedded schema SQL
	_, err = db.Exec(initDBSQL)
	if err != nil {
		return nil, fmt.Errorf("error creating schema: %v", err)
	}

	return db, nil
}

type Stats struct {
	totalOpen         int
	totalInProgress   int
	totalClosed       int
	totalCancelled    int
	avgCompletionTime float64 // in days
}

type App struct {
	screen     tcell.Screen
	db         *sql.DB
	activeList []Task
	closedList []Task
	stats      Stats
	activeView bool
	cursor     int
	mode       string // 'normal' or 'edit'
	editBuffer string
	editCol    int
}

func NewApp() (*App, error) {
	// Initialize screen
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := screen.Init(); err != nil {
		return nil, err
	}

	// Initialize database
	db, err := initDB()
	if err != nil {
		return nil, err
	}

	return &App{
		screen: screen,
		db:     db,
		cursor: 0,
		mode:   "normal",
	}, nil
}

func (app *App) loadTasks() error {
	// Load active tasks
	activeRows, err := app.db.Query(`
		SELECT id, task_content, task_spoc, task_state, 
			   created_at, closed_at 
		FROM task 
		WHERE task_state IN ('open', 'in_progress')
		ORDER BY id
	`)
	if err != nil {
		return err
	}
	defer activeRows.Close()

	// Load closed tasks
	closedRows, err := app.db.Query(`
		SELECT id, task_content, task_spoc, task_state, 
			   created_at, closed_at 
		FROM task 
		WHERE task_state IN ('closed', 'cancelled')
		ORDER BY id
	`)
	if err != nil {
		return err
	}
	defer closedRows.Close()

	// Calculate stats
	statsRow := app.db.QueryRow(`
		SELECT 
			COUNT(CASE WHEN task_state = 'open' THEN 1 END) as open_count,
			COUNT(CASE WHEN task_state = 'in_progress' THEN 1 END) as in_progress_count,
			COUNT(CASE WHEN task_state = 'closed' THEN 1 END) as closed_count,
			COUNT(CASE WHEN task_state = 'cancelled' THEN 1 END) as cancelled_count,
			AVG(CASE 
				WHEN task_state = 'closed' 
				THEN JULIANDAY(closed_at) - JULIANDAY(created_at) 
			END) as avg_completion_days
		FROM task
	`)

	err = statsRow.Scan(
		&app.stats.totalOpen,
		&app.stats.totalInProgress,
		&app.stats.totalClosed,
		&app.stats.totalCancelled,
		&app.stats.avgCompletionTime,
	)
	if err != nil {
		return err
	}

	app.activeList = []Task{}
	app.closedList = []Task{}

	// Scan active tasks
	for activeRows.Next() {
		var task Task
		err := activeRows.Scan(
			&task.ID,
			&task.Content,
			&task.SPOC,
			&task.State,
			&task.Created,
			&task.Closed,
		)
		if err != nil {
			return err
		}
		app.activeList = append(app.activeList, task)
	}

	// Scan closed tasks
	for closedRows.Next() {
		var task Task
		err := closedRows.Scan(
			&task.ID,
			&task.Content,
			&task.SPOC,
			&task.State,
			&task.Created,
			&task.Closed,
		)
		if err != nil {
			return err
		}
		app.closedList = append(app.closedList, task)
	}

	return nil
}

func (app *App) drawText(x, y int, text string, style tcell.Style) {
	for i, r := range text {
		app.screen.SetContent(x+i, y, r, nil, style)
	}
}

func (app *App) drawScreen() {
	width, height := app.screen.Size()
	halfWidth := width / 2
	halfHeight := (height - 6) / 2 // Reserve space for stats at bottom

	style := tcell.StyleDefault
	headerStyle := tcell.StyleDefault.Bold(true)

	// Draw vertical separator
	for y := 0; y < height; y++ {
		app.screen.SetContent(halfWidth, y, '│', nil, style)
	}

	// Draw horizontal separator on right side
	for x := halfWidth + 1; x < width; x++ {
		app.screen.SetContent(x, halfHeight, '─', nil, style)
	}

	// Draw titles
	app.drawText(2, 0, "Active Tasks", headerStyle)
	app.drawText(halfWidth+2, 0, "Completed Tasks", headerStyle)
	app.drawText(halfWidth+2, halfHeight+1, "Statistics", headerStyle)

	// Draw active tasks
	app.drawTaskList(1, 2, halfWidth-2, app.activeList, app.activeView)

	// Draw completed tasks
	app.drawTaskList(halfWidth+1, 2, width-halfWidth-2, app.closedList, !app.activeView)

	// Draw statistics
	statsY := halfHeight + 2
	app.drawText(halfWidth+2, statsY, fmt.Sprintf("Open Tasks: %d", app.stats.totalOpen), style)
	app.drawText(halfWidth+2, statsY+1, fmt.Sprintf("In Progress: %d", app.stats.totalInProgress), style)
	app.drawText(halfWidth+2, statsY+2, fmt.Sprintf("Completed: %d", app.stats.totalClosed), style)
	app.drawText(halfWidth+2, statsY+3, fmt.Sprintf("Cancelled: %d", app.stats.totalCancelled), style)
	app.drawText(halfWidth+2, statsY+4, fmt.Sprintf("Avg Completion Time: %.1f days", app.stats.avgCompletionTime), style)

	// Update instructions based on which pane is active
	if app.activeView {
		app.drawText(1, height-1, "↑/↓: Move cursor | Tab: Switch view | h: Hide | j: Next | k: Previous | i: Edit | q: Quit", style)
	} else {
		app.drawText(1, height-1, "↑/↓: Move cursor | Tab: Switch view | h: Hide | j: Next | k: Previous | i: Edit | q: Quit", style)
	}
}

func (app *App) drawTaskList(x, y, width int, tasks []Task, isActive bool) {
	style := tcell.StyleDefault

	// Draw header
	headers := []string{"ID", "Content", "SPOC", "State"}
	headerWidths := []int{4, width - 35, 15, 11}

	currentX := x
	for i, header := range headers {
		app.drawText(currentX, y, header, style.Bold(true))
		currentX += headerWidths[i] + 1
	}

	// Draw tasks
	for i, task := range tasks {
		rowY := y + i + 2
		currentX := x

		// Highlight if this list is active and this row is selected
		rowStyle := style
		if isActive && app.cursor == i {
			rowStyle = style.Background(tcell.ColorGray)
		}

		fields := []string{
			task.ID,
			task.Content,
			task.SPOC,
			task.State,
		}

		for colIdx, field := range fields {
			// If we're editing this specific cell
			if app.mode == "edit" &&
				isActive &&
				app.cursor == i && // Current row
				app.editCol == colIdx { // Current column
				editStyle := style.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack)
				app.drawText(currentX, rowY, app.editBuffer, editStyle)
			} else {
				// Use normal row highlighting
				if i == 1 && len(field) > headerWidths[colIdx] {
					field = field[:headerWidths[colIdx]-3] + "..."
				}
				app.drawText(currentX, rowY, field, rowStyle)
			}
			currentX += headerWidths[colIdx] + 1
		}
	}
}

func (app *App) editCurrentTask(task Task) error {
	app.mode = "edit"
	app.editCol = 1 // Start with content column
	app.editBuffer = task.Content
	return nil
}

func (app *App) saveCurrentEdit() error {
	var currentTask Task

	// Get the correct task based on which view is active
	if app.activeView && len(app.activeList) > 0 {
		currentTask = app.activeList[app.cursor]
	} else if !app.activeView && len(app.closedList) > 0 {
		currentTask = app.closedList[app.cursor]
	} else {
		return nil
	}

	var query string
	var params []interface{}

	switch app.editCol {
	case 1:
		query = "UPDATE task SET task_content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
		params = []interface{}{app.editBuffer, currentTask.ID}
	case 3:
		// Validate state
		validStates := map[string]bool{
			"open":        true,
			"in_progress": true,
			"closed":      true,
			"cancelled":   true,
		}
		if !validStates[app.editBuffer] {
			return fmt.Errorf("invalid state: %s", app.editBuffer)
		}

		query = "UPDATE task SET task_state = ?, updated_at = CURRENT_TIMESTAMP"
		params = []interface{}{app.editBuffer, currentTask.ID}

		// If changing to closed state, set closed_at
		if app.editBuffer == "closed" {
			query += ", closed_at = CURRENT_TIMESTAMP"
		}
		query += " WHERE id = ?"
	}

	_, err := app.db.Exec(query, params...)
	if err != nil {
		return err
	}

	// Reload tasks to refresh the display
	return app.loadTasks()
}

func (app *App) Run() error {
	defer app.screen.Fini()

	if err := app.loadTasks(); err != nil {
		return err
	}

	for {
		app.screen.Clear()
		app.drawScreen()
		app.screen.Show()

		switch ev := app.screen.PollEvent().(type) {
		case *tcell.EventKey:
			if app.mode == "normal" {
				switch ev.Key() {
				case tcell.KeyEscape, tcell.KeyCtrlC:
					return nil
				case tcell.KeyTab:
					app.activeView = !app.activeView
					app.cursor = 0
				case tcell.KeyRune:
					switch ev.Rune() {
					case 'q':
						return nil
					case 'h':
						if app.activeView {
							app.activeView = false
							app.cursor = 0
						}
					case 'l':
						if !app.activeView {
							app.activeView = true
							app.cursor = 0
						}
					case 'j':
						if app.activeView {
							if app.cursor < len(app.activeList)-1 {
								app.cursor++
							}
						} else {
							if app.cursor < len(app.closedList)-1 {
								app.cursor++
							}
						}
					case 'k':
						if app.cursor > 0 {
							app.cursor--
						}
					case 'i':
						// Only enter edit mode in active (left) pane
						if app.activeView && len(app.activeList) > 0 {
							app.mode = "edit"
							app.editCol = 1 // Start with content
							app.editBuffer = app.activeList[app.cursor].Content
						}
					}
				}
			} else { // Edit mode
				switch ev.Key() {
				case tcell.KeyEscape:
					app.mode = "normal"
					app.editBuffer = ""
				case tcell.KeyEnter:
					if err := app.saveCurrentEdit(); err != nil {
						app.mode = "normal"
						return err
					}
					app.mode = "normal"
				case tcell.KeyTab:
					// Toggle between content and state
					if app.editCol == 1 {
						app.editCol = 3
						app.editBuffer = app.activeList[app.cursor].State
					} else {
						app.editCol = 1
						app.editBuffer = app.activeList[app.cursor].Content
					}
				case tcell.KeyBackspace, tcell.KeyBackspace2:
					if len(app.editBuffer) > 0 {
						app.editBuffer = app.editBuffer[:len(app.editBuffer)-1]
					}
				case tcell.KeyRune:
					if app.editCol == 3 {
						// Validate state input
						newState := app.editBuffer + string(ev.Rune())
						if isValidStateInput(newState) {
							app.editBuffer += string(ev.Rune())
						}
					} else {
						app.editBuffer += string(ev.Rune())
					}
				}
			}
		}
	}
}

func isValidStateInput(s string) bool {
	validStates := []string{"open", "in_progress", "closed", "cancelled"}
	for _, state := range validStates {
		if strings.HasPrefix(state, s) {
			return true
		}
	}
	return false
}

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
