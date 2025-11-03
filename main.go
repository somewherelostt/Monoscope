package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	ASCII_CHARS = " .:-=+*#%@"
	FPS         = 24
)

func getTerminalSize() (int, int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 160, 50 // Default fallback
	}
	return width, height - 2 // Leave space for status line
}

func rgbToAnsi(r, g, b uint8) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

func brightnessToASCII(brightness float64) byte {
	idx := int(brightness * float64(len(ASCII_CHARS)-1))
	if idx >= len(ASCII_CHARS) {
		idx = len(ASCII_CHARS) - 1
	}
	return ASCII_CHARS[idx]
}

func frameToASCII(img image.Image) string {
	var builder strings.Builder
	bounds := img.Bounds()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			r, g, b, _ := c.RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

			brightness := (0.299*float64(r8) + 0.587*float64(g8) + 0.114*float64(b8)) / 255.0
			char := brightnessToASCII(brightness)

			builder.WriteString(rgbToAnsi(r8, g8, b8))
			builder.WriteByte(char)
		}
		builder.WriteString("\033[0m\n")
	}
	return builder.String()
}

func readRawFrame(reader *bufio.Reader, width, height int) (image.Image, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	data := make([]byte, width*height*3)
	
	n := 0
	for n < len(data) {
		read, err := reader.Read(data[n:])
		if err != nil {
			return nil, err
		}
		n += read
	}

	idx := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: data[idx], G: data[idx+1], B: data[idx+2], A: 255})
			idx += 3
		}
	}
	return img, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <video_file>")
		os.Exit(1)
	}

	videoPath := os.Args[1]
	frameDuration := time.Duration(1000000/FPS) * time.Microsecond

	// Get terminal size automatically
	WIDTH, HEIGHT := getTerminalSize()

	cmd := exec.Command("ffmpeg", "-i", videoPath, 
		"-vf", fmt.Sprintf("fps=%d,scale=%d:%d", FPS, WIDTH, HEIGHT),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
		os.Exit(1)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating stderr pipe: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting ffmpeg: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure ffmpeg is installed and the video file exists\n")
		os.Exit(1)
	}
	defer cmd.Process.Kill()

	// Discard FFmpeg stderr output in background
	go func() {
		buf := make([]byte, 4096)
		for {
			stderr.Read(buf)
		}
	}()

	reader := bufio.NewReader(stdout)
	
	// Enter alternate screen buffer and hide cursor
	fmt.Print("\033[?1049h\033[?25l")
	defer func() {
		fmt.Print("\033[?1049l\033[?25h")
		fmt.Println()
	}()

	frameCount := 0
	// THE ONE LOOP - reads video frames, converts to ASCII, displays
	for {
		startTime := time.Now()

		img, err := readRawFrame(reader, WIDTH, HEIGHT)
		if err != nil {
			if frameCount == 0 {
				fmt.Print("\033[?1049l\033[?25h")
				fmt.Fprintf(os.Stderr, "Error: Could not read any frames. Check if video file is valid.\n")
				os.Exit(1)
			}
			break
		}

		asciiFrame := frameToASCII(img)
		
		// Move cursor to home position and print frame
		fmt.Print("\033[H" + asciiFrame)
		fmt.Printf("\033[0mFrame: %d | FPS: %d | Press Ctrl+C to exit", frameCount, FPS)
		
		frameCount++

		elapsed := time.Since(startTime)
		if elapsed < frameDuration {
			time.Sleep(frameDuration - elapsed)
		}
	}

	fmt.Print("\033[H\033[2J\033[32m")
	fmt.Printf("Video complete! %d frames played.\nPress Enter to exit...\033[0m", frameCount)
	
	// Wait for user input before exiting
	fmt.Scanln()
}
