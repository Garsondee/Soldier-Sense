// Package main runs the full rendered client for a fixed duration and prints performance stats.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Garsondee/Soldier-Sense/internal/game"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	var seconds int
	var mapSeed int64
	flag.IntVar(&seconds, "seconds", 30, "capture duration in seconds")
	flag.Int64Var(&mapSeed, "map-seed", 0, "explicit map seed for deterministic captures (0 = time-based seed)")
	flag.Parse()

	if seconds <= 0 {
		log.Fatal("-seconds must be > 0")
	}

	ebiten.SetWindowTitle("Soldier Sense - Render Perf Capture")
	ebiten.SetFullscreen(true)

	captureDur := time.Duration(seconds) * time.Second

	var stats game.PerfCaptureStats
	for {
		var g *game.Game
		if mapSeed == 0 {
			g = game.New()
		} else {
			g = game.NewWithMapSeed(mapSeed)
		}
		g.EnableAutoPerfCapture(captureDur)

		err := ebiten.RunGame(g)
		stats = g.AutoPerfCaptureResult()

		switch {
		case err == nil:
			printStats(seconds, stats)
			return
		case errors.Is(err, game.ErrQuit):
			printStats(seconds, stats)
			return
		case errors.Is(err, game.ErrRestart):
			continue
		default:
			log.Fatal(err)
		}
	}
}

func printStats(seconds int, s game.PerfCaptureStats) {
	fmt.Println("=== Render Perf Capture ===")
	fmt.Printf("duration_target_seconds=%d\n", seconds)
	fmt.Printf("duration_actual_seconds=%.3f\n", s.DurationSeconds)
	fmt.Printf("frame_count=%d\n", s.FrameCount)
	fmt.Printf("fps=%.2f\n", s.FPS)
	fmt.Printf("sim_tick_count=%d\n", s.SimTickCount)
	fmt.Printf("avg_sim_ms_per_tick=%.3f\n", s.AvgSimMSPerTick)
	fmt.Printf("avg_world_ms_per_frame=%.3f\n", s.AvgWorldMSPerFrame)
	fmt.Printf("avg_ui_ms_per_frame=%.3f\n", s.AvgUIMSPerFrame)
	fmt.Printf("avg_frame_cpu_ms_buckets=%.3f\n", s.AvgFrameCPUmsBuckets)
}
