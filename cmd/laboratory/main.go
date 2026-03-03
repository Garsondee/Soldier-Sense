package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Garsondee/Soldier-Sense/internal/game"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	var listTests bool
	var testName string

	flag.BoolVar(&listTests, "list", false, "List all available laboratory tests")
	flag.StringVar(&testName, "test", "", "Run a specific laboratory test by name")
	flag.Parse()

	tests := game.GetAllLaboratoryTests()

	if listTests {
		fmt.Println("Available Laboratory Tests:")
		fmt.Println()
		for i, test := range tests {
			fmt.Printf("%d. %s\n", i+1, test.Name)
			fmt.Printf("   Description: %s\n", test.Description)
			fmt.Printf("   Expected: %s\n", test.Expected)
			fmt.Printf("   Duration: %d ticks (%.1f seconds)\n", test.DurationTicks, float64(test.DurationTicks)/60.0)
			fmt.Println()
		}
		return
	}

	var selectedTest *game.LaboratoryTest

	if testName != "" {
		// Find test by name
		for _, test := range tests {
			if test.Name == testName {
				selectedTest = test
				break
			}
		}
		if selectedTest == nil {
			fmt.Printf("Error: Test '%s' not found\n", testName)
			fmt.Println("Use -list to see available tests")
			os.Exit(1)
		}
	} else {
		// Interactive selection
		fmt.Println("Select a laboratory test:")
		for i, test := range tests {
			fmt.Printf("%d. %s\n", i+1, test.Name)
		}
		fmt.Print("\nEnter test number: ")
		var choice int
		_, err := fmt.Scanf("%d", &choice)
		if err != nil || choice < 1 || choice > len(tests) {
			fmt.Println("Invalid selection")
			os.Exit(1)
		}
		selectedTest = tests[choice-1]
	}

	fmt.Printf("\nStarting laboratory test: %s\n", selectedTest.Name)
	fmt.Printf("Description: %s\n", selectedTest.Description)
	fmt.Printf("Expected: %s\n\n", selectedTest.Expected)
	fmt.Println("Controls:")
	fmt.Println("  SPACE    - Pause/Resume")
	fmt.Println("  1/2/4    - Set speed (1x, 2x, 4x)")
	fmt.Println("  Arrows   - Pan camera")
	fmt.Println("  +/-      - Zoom in/out")
	fmt.Println("  H        - Toggle help")
	fmt.Println("  E        - Toggle events panel")
	fmt.Println("  ESC      - Exit")
	fmt.Println()

	// Create visual mode
	labMode := game.NewLaboratoryVisualMode(selectedTest)

	// Set window properties
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle(fmt.Sprintf("Laboratory Test: %s", selectedTest.Name))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	// Run the test
	if err := ebiten.RunGame(labMode); err != nil {
		log.Fatal(err)
	}
}
