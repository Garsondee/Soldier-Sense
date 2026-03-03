package game

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

// LaboratoryVisualMode renders a laboratory test as a visual scene with
// on-screen descriptions of the stimulus and expected behavior.
type LaboratoryVisualMode struct {
	test   *LaboratoryTest
	ts     *TestSim
	obs    *LaboratoryObservation
	tick   int
	paused bool
	speed  int // 1 = normal, 2 = 2x, 4 = 4x

	// Visual state
	cameraX float64
	cameraY float64
	zoom    float64

	// UI state
	showHelp   bool
	showEvents bool
}

func drawBasicText(screen *ebiten.Image, x, y int, str string, col color.Color) {
	face := text.NewGoXFace(basicfont.Face7x13)
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(col)
	text.Draw(screen, str, face, opts)
}

// NewLaboratoryVisualMode creates a new visual laboratory test runner.
func NewLaboratoryVisualMode(test *LaboratoryTest) *LaboratoryVisualMode {
	ts := test.Setup()
	obs := NewLaboratoryObservation(test.Name)
	obs.Ticks = test.DurationTicks

	// Center camera on soldiers
	centerX := 0.0
	centerY := 0.0
	count := 0
	for _, s := range ts.Soldiers {
		centerX += s.x
		centerY += s.y
		count++
	}
	if count > 0 {
		centerX /= float64(count)
		centerY /= float64(count)
	}

	return &LaboratoryVisualMode{
		test:       test,
		ts:         ts,
		obs:        obs,
		tick:       0,
		paused:     false,
		speed:      1,
		cameraX:    centerX,
		cameraY:    centerY,
		zoom:       1.0,
		showHelp:   true,
		showEvents: true,
	}
}

// Update advances the laboratory test simulation.
func (lv *LaboratoryVisualMode) Update() error {
	// Handle input
	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		lv.paused = !lv.paused
	}
	if ebiten.IsKeyPressed(ebiten.Key1) {
		lv.speed = 1
	}
	if ebiten.IsKeyPressed(ebiten.Key2) {
		lv.speed = 2
	}
	if ebiten.IsKeyPressed(ebiten.Key4) {
		lv.speed = 4
	}
	if ebiten.IsKeyPressed(ebiten.KeyH) {
		lv.showHelp = !lv.showHelp
	}
	if ebiten.IsKeyPressed(ebiten.KeyE) {
		lv.showEvents = !lv.showEvents
	}

	// Camera controls
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		lv.cameraX -= 5.0 / lv.zoom
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		lv.cameraX += 5.0 / lv.zoom
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		lv.cameraY -= 5.0 / lv.zoom
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		lv.cameraY += 5.0 / lv.zoom
	}
	if ebiten.IsKeyPressed(ebiten.KeyEqual) || ebiten.IsKeyPressed(ebiten.KeyNumpadAdd) {
		lv.zoom *= 1.02
	}
	if ebiten.IsKeyPressed(ebiten.KeyMinus) || ebiten.IsKeyPressed(ebiten.KeyNumpadSubtract) {
		lv.zoom /= 1.02
	}

	// Run simulation
	if !lv.paused && lv.tick < lv.test.DurationTicks {
		for i := 0; i < lv.speed; i++ {
			if lv.tick >= lv.test.DurationTicks {
				break
			}

			// Apply stimulus
			if lv.test.Stimulus != nil {
				lv.test.Stimulus(lv.ts, lv.tick)
			}

			// Advance simulation
			lv.ts.RunTicks(1)

			// Track observations (simplified version)
			for _, s := range lv.ts.Soldiers {
				if len(s.vision.KnownContacts) > 0 && lv.obs.FirstContactTick < 0 {
					lv.obs.FirstContactTick = lv.tick
					lv.obs.AddEvent(lv.tick, s.id, s.label, "first_contact",
						fmt.Sprintf("%d enemies spotted", len(s.vision.KnownContacts)), 0)
				}

				if s.blackboard.PanicRetreatActive && lv.obs.FirstPanicTick < 0 {
					lv.obs.FirstPanicTick = lv.tick
					lv.obs.AddEvent(lv.tick, s.id, s.label, "panic_retreat", "panic retreat activated", 0)
				}

				if s.profile.Psych.Fear > lv.obs.MaxFear {
					lv.obs.MaxFear = s.profile.Psych.Fear
					lv.obs.MaxFearTick = lv.tick
				}
			}

			// Custom measurements
			if lv.test.Measure != nil {
				lv.test.Measure(lv.ts, lv.tick, lv.obs)
			}

			lv.tick++
		}
	}

	return nil
}

// Draw renders the laboratory test scene.
func (lv *LaboratoryVisualMode) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 15, G: 20, B: 15, A: 255})

	// Calculate viewport
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	viewportW := float64(screenW) / lv.zoom
	viewportH := float64(screenH) / lv.zoom

	// Draw world (buildings, soldiers)
	lv.drawWorld(screen, viewportW, viewportH)

	// Draw UI overlays
	lv.drawTestInfo(screen)
	lv.drawStatus(screen)
	if lv.showEvents {
		lv.drawEvents(screen)
	}
	if lv.showHelp {
		lv.drawHelp(screen)
	}
}

// drawWorld renders the battlefield and soldiers.
func (lv *LaboratoryVisualMode) drawWorld(screen *ebiten.Image, viewportW, viewportH float64) {
	screenW, screenH := float32(screen.Bounds().Dx()), float32(screen.Bounds().Dy())

	// Draw buildings
	for _, b := range lv.ts.buildings {
		wx := float32(b.x) - float32(lv.cameraX) + float32(viewportW)/2
		wy := float32(b.y) - float32(lv.cameraY) + float32(viewportH)/2
		wx = wx * float32(lv.zoom)
		wy = wy * float32(lv.zoom)
		bw := float32(b.w) * float32(lv.zoom)
		bh := float32(b.h) * float32(lv.zoom)

		if wx+bw >= 0 && wx < screenW && wy+bh >= 0 && wy < screenH {
			vector.FillRect(screen, wx, wy, bw, bh, color.RGBA{R: 60, G: 60, B: 60, A: 255}, false)
			vector.StrokeRect(screen, wx, wy, bw, bh, 1, color.RGBA{R: 100, G: 100, B: 100, A: 255}, false)
		}
	}

	// Draw soldiers
	for _, s := range lv.ts.Soldiers {
		wx := float32(s.x) - float32(lv.cameraX) + float32(viewportW)/2
		wy := float32(s.y) - float32(lv.cameraY) + float32(viewportH)/2
		wx = wx * float32(lv.zoom)
		wy = wy * float32(lv.zoom)

		if wx >= -20 && wx < screenW+20 && wy >= -20 && wy < screenH+20 {
			// Color based on team and state
			var col color.RGBA
			if s.team == TeamRed {
				col = color.RGBA{R: 200, G: 50, B: 50, A: 255}
			} else {
				col = color.RGBA{R: 50, G: 100, B: 200, A: 255}
			}

			if s.state == SoldierStateDead {
				col = color.RGBA{R: 80, G: 80, B: 80, A: 255}
			} else if s.blackboard.PanicRetreatActive {
				col = color.RGBA{R: 255, G: 200, B: 0, A: 255}
			} else if s.blackboard.DisobeyingOrders {
				col = color.RGBA{R: 255, G: 150, B: 0, A: 255}
			}

			radius := float32(8 * lv.zoom)
			vector.FillCircle(screen, wx, wy, radius, col, false)

			// Draw label
			if lv.zoom > 0.5 {
				drawBasicText(screen, int(wx)-10, int(wy)-15, s.label, color.White)
			}

			// Draw fear indicator
			if s.profile.Psych.Fear > 0.1 {
				barW := float32(20 * lv.zoom)
				barH := float32(3 * lv.zoom)
				barX := wx - barW/2
				barY := wy + radius + 5

				// Background
				vector.FillRect(screen, barX, barY, barW, barH, color.RGBA{R: 40, G: 40, B: 40, A: 200}, false)
				// Fear level
				fearW := barW * float32(s.profile.Psych.Fear)
				vector.FillRect(screen, barX, barY, fearW, barH, color.RGBA{R: 255, G: 100, B: 100, A: 255}, false)
			}
		}
	}
}

// drawTestInfo renders the test description panel.
func (lv *LaboratoryVisualMode) drawTestInfo(screen *ebiten.Image) {
	panelX := float32(10)
	panelY := float32(10)
	panelW := float32(500)
	panelH := float32(140)

	// Panel background
	vector.FillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{R: 20, G: 25, B: 30, A: 230}, false)
	vector.StrokeRect(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{R: 100, G: 120, B: 140, A: 255}, false)

	// Title
	titleY := int(panelY) + 20
	drawBasicText(screen, int(panelX)+10, titleY, lv.test.Name, color.RGBA{R: 200, G: 220, B: 255, A: 255})

	// Description
	descY := titleY + 20
	lines := wrapText(lv.test.Description, 60)
	for i, line := range lines {
		drawBasicText(screen, int(panelX)+10, descY+i*15, line, color.RGBA{R: 180, G: 180, B: 180, A: 255})
	}

	// Expected behavior
	expectedY := descY + len(lines)*15 + 10
	drawBasicText(screen, int(panelX)+10, expectedY, "Expected:", color.RGBA{R: 150, G: 255, B: 150, A: 255})
	expectedLines := wrapText(lv.test.Expected, 60)
	for i, line := range expectedLines {
		drawBasicText(screen, int(panelX)+10, expectedY+15+i*15, line, color.RGBA{R: 150, G: 200, B: 150, A: 255})
	}
}

// drawStatus renders the simulation status panel.
func (lv *LaboratoryVisualMode) drawStatus(screen *ebiten.Image) {
	screenW := screen.Bounds().Dx()
	panelW := float32(300)
	panelH := float32(180)
	panelX := float32(screenW) - panelW - 10
	panelY := float32(10)

	// Panel background
	vector.FillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{R: 20, G: 25, B: 30, A: 230}, false)
	vector.StrokeRect(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{R: 100, G: 120, B: 140, A: 255}, false)

	y := int(panelY) + 20
	x := int(panelX) + 10

	// Progress
	progress := float64(lv.tick) / float64(lv.test.DurationTicks) * 100
	statusText := "Running"
	if lv.paused {
		statusText = "Paused"
	}
	if lv.tick >= lv.test.DurationTicks {
		statusText = "Complete"
	}

	drawBasicText(screen, x, y, fmt.Sprintf("Status: %s", statusText), color.White)
	y += 15
	drawBasicText(screen, x, y, fmt.Sprintf("Tick: %d / %d (%.1f%%)", lv.tick, lv.test.DurationTicks, progress), color.White)
	y += 15
	drawBasicText(screen, x, y, fmt.Sprintf("Speed: %dx", lv.speed), color.White)
	y += 20

	// Key observations
	drawBasicText(screen, x, y, "Observations:", color.RGBA{R: 200, G: 220, B: 255, A: 255})
	y += 15

	if lv.obs.FirstContactTick >= 0 {
		drawBasicText(screen, x, y, fmt.Sprintf("First Contact: T=%d", lv.obs.FirstContactTick), color.RGBA{R: 150, G: 200, B: 150, A: 255})
		y += 15
	}

	if lv.obs.MaxFear > 0 {
		drawBasicText(screen, x, y, fmt.Sprintf("Max Fear: %.2f (T=%d)", lv.obs.MaxFear, lv.obs.MaxFearTick), color.RGBA{R: 255, G: 150, B: 150, A: 255})
		y += 15
	}

	if lv.obs.FirstPanicTick >= 0 {
		drawBasicText(screen, x, y, fmt.Sprintf("Panic: T=%d", lv.obs.FirstPanicTick), color.RGBA{R: 255, G: 200, B: 100, A: 255})
		y += 15
	}

	if lv.obs.CohesionBreakTick >= 0 {
		drawBasicText(screen, x, y, fmt.Sprintf("Cohesion Break: T=%d", lv.obs.CohesionBreakTick), color.RGBA{R: 255, G: 100, B: 100, A: 255})
		y += 15
	}

	// Test result
	if lv.tick >= lv.test.DurationTicks && lv.test.Validate != nil {
		passed, reason := lv.test.Validate(lv.obs)
		y += 10
		if passed {
			drawBasicText(screen, x, y, "PASSED", color.RGBA{R: 100, G: 255, B: 100, A: 255})
		} else {
			drawBasicText(screen, x, y, "FAILED", color.RGBA{R: 255, G: 100, B: 100, A: 255})
		}
		y += 15
		reasonLines := wrapText(reason, 35)
		for _, line := range reasonLines {
			drawBasicText(screen, x, y, line, color.RGBA{R: 200, G: 200, B: 200, A: 255})
			y += 13
		}
	}
}

// drawEvents renders recent events.
func (lv *LaboratoryVisualMode) drawEvents(screen *ebiten.Image) {
	screenH := screen.Bounds().Dy()
	panelW := float32(500)
	panelH := float32(200)
	panelX := float32(10)
	panelY := float32(screenH) - panelH - 10

	// Panel background
	vector.FillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{R: 20, G: 25, B: 30, A: 230}, false)
	vector.StrokeRect(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{R: 100, G: 120, B: 140, A: 255}, false)

	y := int(panelY) + 20
	x := int(panelX) + 10

	drawBasicText(screen, x, y, "Recent Events:", color.RGBA{R: 200, G: 220, B: 255, A: 255})
	y += 20

	// Show last 10 events
	startIdx := len(lv.obs.Events) - 10
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(lv.obs.Events) && y < int(panelY+panelH)-10; i++ {
		evt := lv.obs.Events[i]
		eventText := fmt.Sprintf("[T=%03d] %s: %s", evt.Tick, evt.SoldierLabel, evt.Description)
		if len(eventText) > 60 {
			eventText = eventText[:57] + "..."
		}

		eventColor := color.RGBA{R: 180, G: 180, B: 180, A: 255}
		if evt.EventType == "panic_retreat" || evt.EventType == "disobedience" {
			eventColor = color.RGBA{R: 255, G: 200, B: 100, A: 255}
		} else if evt.EventType == "first_contact" {
			eventColor = color.RGBA{R: 255, G: 150, B: 150, A: 255}
		}

		drawBasicText(screen, x, y, eventText, eventColor)
		y += 13
	}
}

// drawHelp renders the help overlay.
func (lv *LaboratoryVisualMode) drawHelp(screen *ebiten.Image) {
	screenW, screenH := screen.Bounds().Dx(), screen.Bounds().Dy()
	panelW := float32(350)
	panelH := float32(220)
	panelX := float32(screenW)/2 - panelW/2
	panelY := float32(screenH)/2 - panelH/2

	// Panel background
	vector.FillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{R: 30, G: 35, B: 40, A: 250}, false)
	vector.StrokeRect(screen, panelX, panelY, panelW, panelH, 3, color.RGBA{R: 150, G: 170, B: 190, A: 255}, false)

	y := int(panelY) + 25
	x := int(panelX) + 15

	drawBasicText(screen, x, y, "Laboratory Test Controls", color.RGBA{R: 200, G: 220, B: 255, A: 255})
	y += 25

	helpLines := []string{
		"SPACE    - Pause/Resume",
		"1/2/4    - Set speed (1x, 2x, 4x)",
		"Arrows   - Pan camera",
		"+/-      - Zoom in/out",
		"H        - Toggle this help",
		"E        - Toggle events panel",
		"ESC      - Exit to menu",
		"",
		"Visual Indicators:",
		"  Red circles    - Red team soldiers",
		"  Blue circles   - Blue team soldiers",
		"  Yellow circles - Panicking soldiers",
		"  Orange circles - Disobeying soldiers",
		"  Red bars       - Fear level",
	}

	for _, line := range helpLines {
		drawBasicText(screen, x, y, line, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		y += 14
	}
}

// Layout returns the screen dimensions.
func (lv *LaboratoryVisualMode) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// wrapText breaks a string into lines of maximum width.
func wrapText(s string, maxWidth int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{}
	currentLine := words[0]

	for i := 1; i < len(words); i++ {
		if len(currentLine)+1+len(words[i]) <= maxWidth {
			currentLine += " " + words[i]
		} else {
			lines = append(lines, currentLine)
			currentLine = words[i]
		}
	}
	lines = append(lines, currentLine)

	return lines
}
