// Package convert wraps ffmpeg, ImageMagick, and pandoc into simple conversion helpers.
// All functions check for the required binary before running.
package convert

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	cc "github.com/jadogg/babi/internal/clicolor"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ─── Tool discovery ───────────────────────────────────────────────────────────

// FindBinary searches every directory in PATH for an executable named name.
func FindBinary(name string) (string, error) {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	suffixes := []string{""}
	if runtime.GOOS == "windows" {
		suffixes = []string{".exe", ".cmd", ".bat", ""}
	}
	for _, dir := range dirs {
		for _, s := range suffixes {
			p := filepath.Join(dir, name+s)
			info, err := os.Stat(p)
			if err != nil || info.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
				continue
			}
			return p, nil
		}
	}
	return "", fmt.Errorf("%q not found in PATH", name)
}

func require(name string) (string, error) {
	p, err := FindBinary(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed or not in PATH\n  install: %s", name, installHint(name))
	}
	return p, nil
}

// imBin returns the ImageMagick binary: prefers magick (v7), falls back to convert (v6).
func imBin() (string, error) {
	if p, err := FindBinary("magick"); err == nil {
		return p, nil
	}
	if p, err := FindBinary("convert"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("ImageMagick not found (tried 'magick' and 'convert')\n  install: %s", installHint("magick"))
}

func ffmpegBin() (string, error) { return require("ffmpeg") }
func pandocBin() (string, error) { return require("pandoc") }

func installHint(name string) string {
	switch name {
	case "ffmpeg":
		return "brew install ffmpeg  |  apt install ffmpeg  |  https://ffmpeg.org/download.html"
	case "magick", "convert":
		return "brew install imagemagick  |  apt install imagemagick  |  https://imagemagick.org"
	case "pandoc":
		return "brew install pandoc  |  apt install pandoc  |  https://pandoc.org/installing.html"
	}
	return "check https://command-not-found.com/" + name
}

// ─── File type classification ─────────────────────────────────────────────────

// FileType represents the category of a file based on its extension.
type FileType int

const (
	TypeImage   FileType = iota
	TypeGIF              // animated or static GIF (treated separately for tool selection)
	TypeVideo
	TypeAudio
	TypeDoc
	TypeUnknown
)

func (t FileType) String() string {
	switch t {
	case TypeImage:
		return "image"
	case TypeGIF:
		return "gif"
	case TypeVideo:
		return "video"
	case TypeAudio:
		return "audio"
	case TypeDoc:
		return "document"
	}
	return "unknown"
}

var (
	imageExts = extSet(".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff", ".tif", ".avif", ".heic", ".heif", ".ico")
	videoExts = extSet(".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv", ".m4v", ".ts", ".mts", ".m2ts", ".3gp")
	audioExts = extSet(".mp3", ".wav", ".ogg", ".flac", ".aac", ".m4a", ".opus", ".wma", ".aiff")
	docExts   = extSet(".md", ".rst", ".html", ".htm", ".pdf", ".docx", ".odt", ".tex", ".epub", ".txt", ".adoc")
)

func extSet(exts ...string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// Classify returns the FileType for the given path based on its extension.
func Classify(path string) FileType {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".gif" {
		return TypeGIF
	}
	switch {
	case imageExts[ext]:
		return TypeImage
	case videoExts[ext]:
		return TypeVideo
	case audioExts[ext]:
		return TypeAudio
	case docExts[ext]:
		return TypeDoc
	}
	return TypeUnknown
}

// ─── Command runner ───────────────────────────────────────────────────────────

func run(bin string, args ...string) error {
	fmt.Printf("  %s %s %s\n", cc.Dim("$"), cc.Dim(filepath.Base(bin)), cc.Dim(strings.Join(args, " ")))
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ─── Auto-convert ─────────────────────────────────────────────────────────────

// AutoConvert converts input to output, choosing the right tool based on file extensions.
// fps and scale are only used when the output is a GIF (0 = use defaults).
func AutoConvert(input, output string, fps, scale int) error {
	in := Classify(input)
	out := Classify(output)

	arrow := cc.BoldCyan("→")
	fmt.Printf("[babi] %s %s %s  (%s %s %s)\n",
		input, arrow, output,
		cc.Dim(in.String()), cc.Dim("→"), cc.Dim(out.String()))

	switch {
	// image/gif ↔ image/gif  (ImageMagick)
	case (in == TypeImage || in == TypeGIF) && (out == TypeImage || out == TypeGIF):
		return convertImage(input, output)

	// video/gif → first frame (ffmpeg)
	case (in == TypeVideo || in == TypeGIF) && out == TypeImage:
		return extractFirstFrame(input, output)

	// video → gif  (ffmpeg, high-quality palette)
	case in == TypeVideo && out == TypeGIF:
		if fps == 0 {
			fps = 10
		}
		if scale == 0 {
			scale = 480
		}
		return VideoToGif(input, output, fps, scale)

	// gif → video  (ffmpeg)
	case in == TypeGIF && out == TypeVideo:
		return GifToVideo(input, output)

	// video ↔ video  (ffmpeg)
	case (in == TypeVideo || in == TypeGIF) && out == TypeVideo:
		return convertVideo(input, output)

	// video → audio  (ffmpeg)
	case in == TypeVideo && out == TypeAudio:
		return VideoToAudio(input, output)

	// audio ↔ audio  (ffmpeg)
	case in == TypeAudio && out == TypeAudio:
		return convertAudio(input, output)

	// doc ↔ doc  (pandoc)
	case in == TypeDoc && out == TypeDoc:
		return ConvertDoc(input, output)

	default:
		return fmt.Errorf("unsupported conversion: %s (%s) → %s (%s)\nrun 'babi convert --help' for supported combinations",
			filepath.Ext(input), in, filepath.Ext(output), out)
	}
}

// ─── Image operations ─────────────────────────────────────────────────────────

func convertImage(input, output string) error {
	im, err := imBin()
	if err != nil {
		return err
	}
	return run(im, input, output)
}

// CropImage crops an image to WxH at offset X,Y.
// size: "800x600"  pos: "100,50" (empty pos defaults to 0,0).
func CropImage(input, output, size, pos string) error {
	im, err := imBin()
	if err != nil {
		return err
	}
	return run(im, input, "-crop", imGeometry(size, pos), "+repage", output)
}

// CompressImage re-encodes an image at the given quality (0–100, higher = better).
func CompressImage(input, output string, quality int) error {
	im, err := imBin()
	if err != nil {
		return err
	}
	return run(im, input, "-quality", fmt.Sprintf("%d", quality), output)
}

// ─── Video operations ─────────────────────────────────────────────────────────

func convertVideo(input, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, output)
}

// CropVideo crops video frames to WxH at position X,Y.
// size: "1280x720"  pos: "320,180" (empty pos defaults to 0,0).
func CropVideo(input, output, size, pos string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	w, h := splitWH(size)
	x, y := "0", "0"
	if pos != "" {
		x, y = splitXY(pos)
	}
	return run(ff, "-y", "-i", input, "-vf", fmt.Sprintf("crop=%s:%s:%s:%s", w, h, x, y), output)
}

// TrimVideo cuts a video or audio file by time range using stream copy (no re-encode).
// start, end, duration are HH:MM:SS, MM:SS, or plain seconds. end and duration are mutually exclusive.
func TrimVideo(input, output, start, end, duration string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	args := []string{"-y"}
	if start != "" {
		args = append(args, "-ss", start)
	}
	args = append(args, "-i", input)
	if end != "" {
		args = append(args, "-to", end)
	} else if duration != "" {
		args = append(args, "-t", duration)
	}
	args = append(args, "-c", "copy", output)
	return run(ff, args...)
}

// CompressVideo re-encodes a video with H.264 at the given CRF (0–51; lower = better; 28 = good compression).
func CompressVideo(input, output string, crf int) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input,
		"-c:v", "libx264",
		"-crf", fmt.Sprintf("%d", crf),
		"-preset", "slow",
		"-c:a", "aac",
		output,
	)
}

// VideoToAudio extracts the audio track from a video file.
func VideoToAudio(input, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, "-vn", output)
}

// VideoToGif converts a video to a high-quality animated GIF using a two-pass palette.
// fps: playback speed (e.g. 10), scale: output width in pixels (e.g. 480; -1 = keep aspect).
func VideoToGif(input, output string, fps, scale int) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	filter := fmt.Sprintf(
		"fps=%d,scale=%d:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
		fps, scale,
	)
	return run(ff, "-y", "-i", input, "-vf", filter, "-loop", "0", output)
}

// GifToVideo converts an animated GIF to a web-compatible video.
func GifToVideo(input, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, "-movflags", "faststart", "-pix_fmt", "yuv420p", output)
}

// ExtractFrames saves every frame from a video or GIF as a PNG in outputDir.
// fps <= 0 extracts all frames; fps > 0 samples at that rate.
func ExtractFrames(input, outputDir string, fps int) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	pattern := filepath.Join(outputDir, "frame_%04d.png")
	if fps > 0 {
		return run(ff, "-y", "-i", input, "-vf", fmt.Sprintf("fps=%d", fps), pattern)
	}
	return run(ff, "-y", "-i", input, pattern)
}

// Slideshow creates a video from all images in inputDir, sorted alphabetically.
func Slideshow(inputDir, output string, fps int) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	pattern, err := imageGlob(inputDir)
	if err != nil {
		return err
	}
	return run(ff, "-y",
		"-framerate", fmt.Sprintf("%d", fps),
		"-pattern_type", "glob",
		"-i", pattern,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		output,
	)
}

func extractFirstFrame(input, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, "-vframes", "1", output)
}

// ─── Audio operations ─────────────────────────────────────────────────────────

func convertAudio(input, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, output)
}

// CompressAudio re-encodes audio at the given bitrate (e.g. "128k", "96k").
func CompressAudio(input, output, bitrate string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", input, "-b:a", bitrate, output)
}

// ─── Document operations ──────────────────────────────────────────────────────

// ConvertDoc converts a document using pandoc (md, html, pdf, docx, epub, etc.).
func ConvertDoc(input, output string) error {
	pd, err := pandocBin()
	if err != nil {
		return err
	}
	return run(pd, input, "-o", output)
}

// ─── GIF from image directory ─────────────────────────────────────────────────

// DirToGif creates an animated GIF from all images in inputDir, sorted alphabetically.
func DirToGif(inputDir, output string, fps int) error {
	im, err := imBin()
	if err != nil {
		return err
	}
	delay := 100 / fps // ImageMagick delay is in 1/100ths of a second
	files, err := imageFiles(inputDir)
	if err != nil {
		return err
	}
	args := []string{"-delay", fmt.Sprintf("%d", delay), "-loop", "0"}
	args = append(args, files...)
	args = append(args, output)
	return run(im, args...)
}

// ─── PDF operations ───────────────────────────────────────────────────────────

// MergePDF merges two or more PDF files into outFile (created fresh).
// Pass divider=true to insert a blank separator page between each input.
func MergePDF(inFiles []string, outFile string, divider bool) error {
	if len(inFiles) < 2 {
		return fmt.Errorf("need at least 2 PDF files to merge")
	}
	return pdfapi.MergeCreateFile(inFiles, outFile, divider, model.NewDefaultConfiguration())
}

// SplitPDF splits inFile into chunks of span pages each, writing them into outDir.
// span=1 produces one file per page; span=0 is treated as 1.
func SplitPDF(inFile, outDir string, span int) error {
	if span <= 0 {
		span = 1
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	return pdfapi.SplitFile(inFile, outDir, span, model.NewDefaultConfiguration())
}

// SplitPDFAtPages splits inFile at explicit page boundaries (e.g. [3,6] → parts 1-2, 3-5, 6-end).
func SplitPDFAtPages(inFile, outDir string, pageNrs []int) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	return pdfapi.SplitByPageNrFile(inFile, outDir, pageNrs, model.NewDefaultConfiguration())
}

// ─── Spritesheet ──────────────────────────────────────────────────────────────

// Spritesheet packs input image files into a single spritesheet using ImageMagick montage.
// cols: number of columns (0 = auto-square). tileW/tileH: cell size in px (0 = natural size).
// padding: gap between cells in px.
func Spritesheet(inputs []string, output string, cols, tileW, tileH, padding int) error {
	im, err := imBin()
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no input images provided")
	}

	// Build -tile argument: "Cx0" = C columns, auto rows; "0x0" = auto-square
	tileArg := "0x0"
	if cols > 0 {
		tileArg = fmt.Sprintf("%dx0", cols)
	}

	// Build -geometry argument: cell size + padding
	var geomArg string
	switch {
	case tileW > 0 && tileH > 0:
		geomArg = fmt.Sprintf("%dx%d+%d+%d", tileW, tileH, padding, padding)
	case tileW > 0:
		geomArg = fmt.Sprintf("%dx+%d+%d", tileW, padding, padding)
	default:
		geomArg = fmt.Sprintf("+%d+%d", padding, padding)
	}

	// magick montage takes: [inputs...] [options] output
	// For v6 "convert" binary, montage is a separate binary; for v7 "magick" it's a subcommand.
	var args []string
	if filepath.Base(im) == "magick" {
		args = append(args, "montage")
	}
	args = append(args, inputs...)
	args = append(args, "-tile", tileArg, "-geometry", geomArg, "-background", "none", output)
	return run(im, args...)
}

// DirToSpritesheet collects all images in inputDir (sorted) and calls Spritesheet.
func DirToSpritesheet(inputDir, output string, cols, tileW, tileH, padding int) error {
	files, err := imageFiles(inputDir)
	if err != nil {
		return err
	}
	return Spritesheet(files, output, cols, tileW, tileH, padding)
}

// ─── Merge / mux ──────────────────────────────────────────────────────────────

// MergeMedia concatenates multiple audio or video files of the same type into one output.
// Uses ffmpeg concat demuxer (supports all containers). All inputs must be the same codec/format.
func MergeMedia(inputs []string, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	if len(inputs) < 2 {
		return fmt.Errorf("need at least 2 input files to merge")
	}

	// Write a temporary file list for the concat demuxer.
	tmp, err := os.CreateTemp("", "babi-merge-*.txt")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	for _, f := range inputs {
		abs, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		// Escape single quotes in path for ffmpeg concat list format.
		escaped := strings.ReplaceAll(abs, "'", `'\''`)
		fmt.Fprintf(tmp, "file '%s'\n", escaped)
	}
	tmp.Close()

	return run(ff, "-y", "-f", "concat", "-safe", "0", "-i", tmp.Name(), "-c", "copy", output)
}

// MuxVideoAudio combines a video stream and an audio stream into a single file (no re-encode).
func MuxVideoAudio(video, audio, output string) error {
	ff, err := ffmpegBin()
	if err != nil {
		return err
	}
	return run(ff, "-y", "-i", video, "-i", audio, "-c", "copy", "-map", "0:v:0", "-map", "1:a:0", output)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// splitWH parses "WxH", "W:H", or "WXH" into separate width and height strings.
func splitWH(s string) (w, h string) {
	s = strings.NewReplacer("X", "x", ":", "x").Replace(s)
	parts := strings.SplitN(s, "x", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return s, s
}

// splitXY parses "X,Y" or "X:Y" into separate coordinate strings.
func splitXY(s string) (x, y string) {
	s = strings.ReplaceAll(s, ":", ",")
	parts := strings.SplitN(s, ",", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return s, "0"
}

// imGeometry builds an ImageMagick geometry string WxH+X+Y.
func imGeometry(size, pos string) string {
	if pos == "" {
		return size
	}
	x, y := splitXY(pos)
	return fmt.Sprintf("%s+%s+%s", size, x, y)
}

// imageGlob returns a glob pattern for the most common image extension found in dir.
func imageGlob(dir string) (string, error) {
	for _, ext := range []string{"*.jpg", "*.jpeg", "*.png", "*.webp", "*.bmp"} {
		if m, _ := filepath.Glob(filepath.Join(dir, ext)); len(m) > 0 {
			return filepath.Join(dir, ext), nil
		}
	}
	return "", fmt.Errorf("no images found in %s (expected .jpg, .jpeg, .png, .webp, or .bmp)", dir)
}

// imageFiles returns all image files in dir sorted alphabetically.
func imageFiles(dir string) ([]string, error) {
	var files []string
	for _, ext := range []string{"*.jpg", "*.jpeg", "*.png", "*.webp", "*.bmp"} {
		m, _ := filepath.Glob(filepath.Join(dir, ext))
		files = append(files, m...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no images found in %s", dir)
	}
	sort.Strings(files)
	return files, nil
}
