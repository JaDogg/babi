package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Command returns the "babi convert" cobra command tree.
func Command() *cobra.Command {
	var cvFPS, cvScale int

	convertCmd := &cobra.Command{
		Use:   "convert <input> <output>",
		Short: "Convert files between formats (images, video, audio, docs)",
		Long: `Convert files between formats using ffmpeg, ImageMagick, and pandoc.
The right tool is chosen automatically based on file extensions.

  babi convert photo.heic       photo.jpg        # image format
  babi convert clip.mov        clip.mp4         # video format
  babi convert video.mp4       audio.mp3        # video → audio
  babi convert video.mp4       animation.gif    # video → gif
  babi convert animation.gif   video.mp4        # gif → video
  babi convert recording.m4a   recording.mp3   # audio format
  babi convert notes.md        notes.pdf        # document (pandoc)

Use subcommands for more control:
  crop      Crop image or video to a size
  trim      Cut video/audio by time range
  compress  Reduce file size
  frames    Extract frames from video or gif
  slideshow Create video or gif from an image directory`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return cmd.Help()
			}
			return AutoConvert(args[0], args[1], cvFPS, cvScale)
		},
	}
	convertCmd.Flags().IntVar(&cvFPS, "fps", 0, "frames per second for gif output (default 10)")
	convertCmd.Flags().IntVar(&cvScale, "scale", 0, "output width in pixels for gif (default 480)")

	// ── crop ──────────────────────────────────────────────────────────────────
	var cvCropSize, cvCropPos string
	cropCmd := &cobra.Command{
		Use:   "crop <input> <output> --size WxH [--pos X,Y]",
		Short: "Crop an image or video to a given size",
		Long: `Crop an image or video frame to the specified dimensions.
--size is required. --pos sets the top-left corner (default 0,0).

  babi convert crop photo.jpg  out.jpg  --size 800x600
  babi convert crop photo.jpg  out.jpg  --size 800x600 --pos 100,50
  babi convert crop video.mp4  out.mp4  --size 1280x720
  babi convert crop video.mp4  out.mp4  --size 1280x720 --pos 320,180`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cvCropSize == "" {
				return fmt.Errorf("--size is required (e.g. --size 1280x720)")
			}
			input, output := args[0], args[1]
			t := Classify(input)
			fmt.Printf("[babi] crop %s %s → %s  size=%s pos=%s\n",
				t, input, output, cvCropSize, cvCropPos)
			switch t {
			case TypeImage, TypeGIF:
				return CropImage(input, output, cvCropSize, cvCropPos)
			case TypeVideo:
				return CropVideo(input, output, cvCropSize, cvCropPos)
			default:
				return fmt.Errorf("crop not supported for %s files", filepath.Ext(input))
			}
		},
	}
	cropCmd.Flags().StringVar(&cvCropSize, "size", "", "crop size as WxH (e.g. 1280x720)")
	cropCmd.Flags().StringVar(&cvCropPos, "pos", "", "top-left corner as X,Y (default 0,0)")

	// ── trim ──────────────────────────────────────────────────────────────────
	var cvTrimStart, cvTrimEnd, cvTrimDuration string
	trimCmd := &cobra.Command{
		Use:   "trim <input> <output>",
		Short: "Cut video or audio by time range (stream copy, no re-encode)",
		Long: `Trim a video or audio file to a time range.
Uses stream copy — fast and lossless (no re-encoding).
Time format: HH:MM:SS, MM:SS, or plain seconds.

  babi convert trim video.mp4  out.mp4  --start 00:01:00 --end 00:02:30
  babi convert trim video.mp4  out.mp4  --start 30 --duration 60
  babi convert trim podcast.mp3 clip.mp3 --start 00:05:00 --end 00:10:00`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cvTrimStart == "" && cvTrimEnd == "" && cvTrimDuration == "" {
				return fmt.Errorf("provide at least one of: --start, --end, --duration")
			}
			fmt.Printf("[babi] trim %s → %s\n", args[0], args[1])
			return TrimVideo(args[0], args[1], cvTrimStart, cvTrimEnd, cvTrimDuration)
		},
	}
	trimCmd.Flags().StringVar(&cvTrimStart, "start", "", "start time (HH:MM:SS or seconds)")
	trimCmd.Flags().StringVar(&cvTrimEnd, "end", "", "end time (HH:MM:SS or seconds)")
	trimCmd.Flags().StringVar(&cvTrimDuration, "duration", "", "duration (HH:MM:SS or seconds)")

	// ── compress ──────────────────────────────────────────────────────────────
	var cvCompressQuality, cvCompressCRF int
	var cvCompressBitrate string
	compressCmd := &cobra.Command{
		Use:   "compress <input> <output>",
		Short: "Reduce file size (image quality, video CRF, audio bitrate)",
		Long: `Re-encode a file at lower quality to reduce its size.
Defaults: image quality=85, video CRF=28 (H.264), audio bitrate=128k.
CRF scale: 0=lossless, 23=default, 28=good compression, 51=worst.

  babi convert compress photo.jpg     small.jpg
  babi convert compress photo.jpg     small.jpg  --quality 70
  babi convert compress video.mp4     small.mp4
  babi convert compress video.mp4     small.mp4  --crf 32
  babi convert compress podcast.mp3   small.mp3
  babi convert compress podcast.mp3   small.mp3  --bitrate 96k`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, output := args[0], args[1]
			switch t := Classify(input); t {
			case TypeImage, TypeGIF:
				q := cvCompressQuality
				if q == 0 {
					q = 85
				}
				fmt.Printf("[babi] compress image %s → %s  quality=%d\n", input, output, q)
				return CompressImage(input, output, q)
			case TypeVideo:
				crf := cvCompressCRF
				if crf == 0 {
					crf = 28
				}
				fmt.Printf("[babi] compress video %s → %s  crf=%d\n", input, output, crf)
				return CompressVideo(input, output, crf)
			case TypeAudio:
				br := cvCompressBitrate
				if br == "" {
					br = "128k"
				}
				fmt.Printf("[babi] compress audio %s → %s  bitrate=%s\n", input, output, br)
				return CompressAudio(input, output, br)
			default:
				return fmt.Errorf("compress not supported for %s", filepath.Ext(input))
			}
		},
	}
	compressCmd.Flags().IntVar(&cvCompressQuality, "quality", 0, "image quality 0-100 (default 85)")
	compressCmd.Flags().IntVar(&cvCompressCRF, "crf", 0, "video CRF 0-51, lower=better (default 28)")
	compressCmd.Flags().StringVar(&cvCompressBitrate, "bitrate", "", "audio bitrate (default 128k)")

	// ── frames ────────────────────────────────────────────────────────────────
	var cvFramesFPS int
	framesCmd := &cobra.Command{
		Use:   "frames <input> <output-dir>",
		Short: "Extract frames from a video or GIF into a directory",
		Long: `Save every frame of a video or animated GIF as a PNG image.
By default all frames are extracted. Use --fps to sample fewer.

  babi convert frames animation.gif  ./frames/
  babi convert frames video.mp4      ./frames/
  babi convert frames video.mp4      ./frames/  --fps 1   # one per second`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("[babi] extracting frames %s → %s/\n", args[0], args[1])
			return ExtractFrames(args[0], args[1], cvFramesFPS)
		},
	}
	framesCmd.Flags().IntVar(&cvFramesFPS, "fps", 0, "frames per second to extract (default: all frames)")

	// ── slideshow ─────────────────────────────────────────────────────────────
	var cvSlideshowFPS int
	slideshowCmd := &cobra.Command{
		Use:   "slideshow <input-dir> <output>",
		Short: "Create a video or GIF from a directory of images",
		Long: `Build a video or animated GIF from all images in a directory.
Images are sorted alphabetically. Output format is determined by extension.
Defaults: 24 fps for video, 10 fps for GIF.

  babi convert slideshow ./photos/  timelapse.mp4
  babi convert slideshow ./photos/  timelapse.mp4  --fps 30
  babi convert slideshow ./frames/  animation.gif
  babi convert slideshow ./frames/  animation.gif  --fps 15`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir, output := args[0], args[1]
			outType := Classify(output)
			fps := cvSlideshowFPS
			if fps == 0 {
				if outType == TypeGIF {
					fps = 10
				} else {
					fps = 24
				}
			}
			fmt.Printf("[babi] slideshow %s/ → %s  fps=%d\n", inputDir, output, fps)
			if outType == TypeGIF {
				return DirToGif(inputDir, output, fps)
			}
			return Slideshow(inputDir, output, fps)
		},
	}
	slideshowCmd.Flags().IntVar(&cvSlideshowFPS, "fps", 0, "fps (default 24 for video, 10 for gif)")

	// ── spritesheet ───────────────────────────────────────────────────────────
	var cvSpriteCols, cvSpriteTileW, cvSpriteTileH, cvSpritePadding int
	spritesheetCmd := &cobra.Command{
		Use:   "spritesheet <output> <image-or-dir...>",
		Short: "Pack images into a spritesheet (ImageMagick)",
		Long: `Pack one or more images (or all images in a directory) into a single spritesheet.
Images are sorted alphabetically. Output format is determined by extension.

  babi convert spritesheet sheet.png  ./icons/
  babi convert spritesheet sheet.png  ./icons/  --cols 8
  babi convert spritesheet sheet.png  ./icons/  --cols 4 --tile 64x64
  babi convert spritesheet sheet.png  a.png b.png c.png  --tile 32x32 --padding 2`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			output := args[0]
			inputs := args[1:]

			if tile, _ := cmd.Flags().GetString("tile"); tile != "" {
				parts := strings.SplitN(strings.ToLower(tile), "x", 2)
				if len(parts) == 2 {
					if w, err := strconv.Atoi(parts[0]); err == nil {
						cvSpriteTileW = w
					}
					if h, err := strconv.Atoi(parts[1]); err == nil {
						cvSpriteTileH = h
					}
				}
			}

			if len(inputs) == 1 {
				info, err := os.Stat(inputs[0])
				if err != nil {
					return err
				}
				if info.IsDir() {
					fmt.Printf("[babi] spritesheet %s/ → %s\n", inputs[0], output)
					return DirToSpritesheet(inputs[0], output, cvSpriteCols, cvSpriteTileW, cvSpriteTileH, cvSpritePadding)
				}
			}
			fmt.Printf("[babi] spritesheet %d images → %s\n", len(inputs), output)
			return Spritesheet(inputs, output, cvSpriteCols, cvSpriteTileW, cvSpriteTileH, cvSpritePadding)
		},
	}
	spritesheetCmd.Flags().IntVar(&cvSpriteCols, "cols", 0, "number of columns (default: auto-square)")
	spritesheetCmd.Flags().IntVar(&cvSpriteTileW, "tile-w", 0, "cell width in px (default: natural size)")
	spritesheetCmd.Flags().IntVar(&cvSpriteTileH, "tile-h", 0, "cell height in px (default: natural size)")
	spritesheetCmd.Flags().IntVar(&cvSpritePadding, "padding", 0, "gap between cells in px (default 0)")
	spritesheetCmd.Flags().StringP("tile", "t", "", "shorthand for --tile-w and --tile-h as WxH (e.g. 64x64)")

	// ── merge ─────────────────────────────────────────────────────────────────
	mergeCmd := &cobra.Command{
		Use:   "merge <output> <file1> <file2> [more...]",
		Short: "Concatenate audio/video files, or mux a video+audio pair",
		Long: `Merge multiple media files into one.

Concat mode  — all inputs are the same type (all audio or all video):
  babi convert merge out.mp3  part1.mp3 part2.mp3 part3.mp3
  babi convert merge full.mp4 clip1.mp4 clip2.mp4

Mux mode  — exactly one video and one audio input (combined into one file):
  babi convert merge out.mp4  video.mp4 audio.mp3
  babi convert merge out.mkv  video.mkv score.flac`,
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			output := args[0]
			inputs := args[1:]

			var videos, audios []string
			for _, f := range inputs {
				switch Classify(f) {
				case TypeVideo, TypeGIF:
					videos = append(videos, f)
				case TypeAudio:
					audios = append(audios, f)
				default:
					return fmt.Errorf("unsupported file type for merge: %s", f)
				}
			}

			switch {
			case len(videos) == 1 && len(audios) == 1:
				fmt.Printf("[babi] mux %s + %s → %s\n", videos[0], audios[0], output)
				return MuxVideoAudio(videos[0], audios[0], output)
			case len(audios) == 0 && len(videos) >= 2:
				fmt.Printf("[babi] merge %d video files → %s\n", len(videos), output)
				return MergeMedia(videos, output)
			case len(videos) == 0 && len(audios) >= 2:
				fmt.Printf("[babi] merge %d audio files → %s\n", len(audios), output)
				return MergeMedia(audios, output)
			default:
				return fmt.Errorf("ambiguous inputs: got %d video(s) and %d audio file(s)\n"+
					"for mux use exactly 1 video + 1 audio; for concat use files of the same type",
					len(videos), len(audios))
			}
		},
	}

	convertCmd.AddCommand(cropCmd, trimCmd, compressCmd, framesCmd, slideshowCmd, spritesheetCmd, mergeCmd)
	return convertCmd
}

// PDFCommand returns the "babi pdf" cobra command tree.
func PDFCommand() *cobra.Command {
	pdfCmd := &cobra.Command{
		Use:   "pdf",
		Short: "PDF utilities (merge, split)",
	}

	mergeCmd := &cobra.Command{
		Use:   "merge <output.pdf> <file1.pdf> <file2.pdf> [more...]",
		Short: "Merge two or more PDF files into one",
		Long: `Merge multiple PDF files into a single output PDF.

  babi pdf merge combined.pdf a.pdf b.pdf c.pdf
  babi pdf merge combined.pdf a.pdf b.pdf --divider`,
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			output := args[0]
			inputs := args[1:]
			divider, _ := cmd.Flags().GetBool("divider")
			fmt.Printf("[babi] pdf merge %d files → %s\n", len(inputs), output)
			return MergePDF(inputs, output, divider)
		},
	}
	mergeCmd.Flags().Bool("divider", false, "insert a blank page between each merged PDF")

	splitCmd := &cobra.Command{
		Use:   "split <input.pdf> <output-dir>",
		Short: "Split a PDF into smaller files",
		Long: `Split a PDF by span (every N pages) or at explicit page boundaries.

  babi pdf split doc.pdf ./parts/                    # one file per page
  babi pdf split doc.pdf ./parts/ --span 5           # every 5 pages
  babi pdf split doc.pdf ./parts/ --pages 3,6,9      # split before pages 3, 6, 9`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, outDir := args[0], args[1]
			span, _ := cmd.Flags().GetInt("span")
			pagesStr, _ := cmd.Flags().GetString("pages")

			if pagesStr != "" {
				var pageNrs []int
				for _, p := range strings.Split(pagesStr, ",") {
					p = strings.TrimSpace(p)
					n, err := strconv.Atoi(p)
					if err != nil || n < 1 {
						return fmt.Errorf("invalid page number %q", p)
					}
					pageNrs = append(pageNrs, n)
				}
				fmt.Printf("[babi] pdf split %s at pages %v → %s/\n", input, pageNrs, outDir)
				return SplitPDFAtPages(input, outDir, pageNrs)
			}

			if span == 0 {
				span = 1
			}
			fmt.Printf("[babi] pdf split %s  span=%d → %s/\n", input, span, outDir)
			return SplitPDF(input, outDir, span)
		},
	}
	splitCmd.Flags().Int("span", 0, "pages per output file (default 1)")
	splitCmd.Flags().String("pages", "", "split before these page numbers, comma-separated (e.g. 3,6,9)")

	pdfCmd.AddCommand(mergeCmd, splitCmd)
	return pdfCmd
}
