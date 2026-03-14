// Package pack implements babi pack / babi unpack.
//
// Formats and methods:
//
//	.zip              – Go stdlib (archive/zip)
//	.tar              – Go stdlib (archive/tar)
//	.tar.gz / .tgz    – Go stdlib (archive/tar + compress/gzip)
//	.tar.bz2 / .tbz2  – pack: external tar -j   unpack: Go stdlib (compress/bzip2)
//	.tar.xz  / .txz   – external tar -J
//	.tar.lzma         – external tar --lzma  (Windows: 7z)
//	.tar.zst / .tzst  – external tar --zstd  (Windows: 7z)
//	.7z               – external 7z / 7za
package pack

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	cc "github.com/jadogg/babi/internal/clicolor"
)

// ─── format detection ────────────────────────────────────────────────────────

type fmtSpec struct {
	name       string
	exts       []string
	stdlibPack bool // Go stdlib can pack this format
	stdlibUnpack bool // Go stdlib can unpack this format
}

var specs = []fmtSpec{
	{name: "zip",      exts: []string{".zip"},              stdlibPack: true,  stdlibUnpack: true},
	{name: "tar",      exts: []string{".tar"},              stdlibPack: true,  stdlibUnpack: true},
	{name: "tar.gz",   exts: []string{".tar.gz", ".tgz"},   stdlibPack: true,  stdlibUnpack: true},
	{name: "tar.bz2",  exts: []string{".tar.bz2", ".tbz2"}, stdlibPack: false, stdlibUnpack: true},
	{name: "tar.xz",   exts: []string{".tar.xz", ".txz"},   stdlibPack: false, stdlibUnpack: false},
	{name: "tar.lzma", exts: []string{".tar.lzma"},          stdlibPack: false, stdlibUnpack: false},
	{name: "tar.zst",  exts: []string{".tar.zst", ".tzst"},  stdlibPack: false, stdlibUnpack: false},
	{name: "7z",       exts: []string{".7z"},               stdlibPack: false, stdlibUnpack: false},
}

func detectFormat(path string) (fmtSpec, error) {
	lp := strings.ToLower(path)
	for _, s := range specs {
		for _, ext := range s.exts {
			if strings.HasSuffix(lp, ext) {
				return s, nil
			}
		}
	}
	return fmtSpec{}, fmt.Errorf(
		"unsupported format %q\n\nSupported: .zip  .tar  .tar.gz/.tgz  .tar.bz2  .tar.xz  .tar.lzma  .tar.zst  .7z",
		filepath.Base(path),
	)
}

// ─── file entry ──────────────────────────────────────────────────────────────

type entry struct {
	disk  string // full path on disk
	arc   string // path inside archive (forward slashes)
	isDir bool
}

// collectInputs gathers (disk, archive-path) pairs for each input file/dir.
func collectInputs(inputs []string) ([]entry, error) {
	var out []entry
	for _, inp := range inputs {
		fi, err := os.Stat(inp)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			out = append(out, entry{disk: inp, arc: filepath.ToSlash(filepath.Base(inp))})
			continue
		}
		// walk directory; paths stored relative to parent of the input dir
		root := filepath.Dir(inp)
		err = filepath.Walk(inp, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			out = append(out, entry{disk: path, arc: filepath.ToSlash(rel), isDir: fi.IsDir()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// safePath joins dest+name and rejects paths that escape dest (zip-slip guard).
func safePath(dest, name string) (string, error) {
	clean := filepath.Join(dest, filepath.FromSlash(name))
	destAbs, _ := filepath.Abs(dest)
	cleanAbs, _ := filepath.Abs(clean)
	sep := string(os.PathSeparator)
	if cleanAbs != destAbs && !strings.HasPrefix(cleanAbs, destAbs+sep) {
		return "", fmt.Errorf("path traversal blocked: %q", name)
	}
	return clean, nil
}

// ─── stdlib: zip ─────────────────────────────────────────────────────────────

func packZip(out string, inputs []string, quiet bool) error {
	entries, err := collectInputs(inputs)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	for _, e := range entries {
		if e.isDir {
			if _, err := zw.Create(e.arc + "/"); err != nil {
				return err
			}
			continue
		}
		if !quiet {
			fmt.Printf("  %s %s\n", cc.Dim("adding"), e.arc)
		}
		src, err := os.Open(e.disk)
		if err != nil {
			return err
		}
		fi, err := src.Stat()
		if err != nil {
			src.Close()
			return err
		}
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			src.Close()
			return err
		}
		hdr.Name = e.arc
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(w, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func unpackZip(src, dest string, quiet bool) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		target, err := safePath(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if !quiet {
			fmt.Printf("  %s %s\n", cc.Dim("extract"), f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// ─── stdlib: tar ─────────────────────────────────────────────────────────────

func packTarGo(out, format string, inputs []string, quiet bool) error {
	entries, err := collectInputs(inputs)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	var tw *tar.Writer
	if format == "tar.gz" {
		gw := gzip.NewWriter(f)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(f)
	}
	defer tw.Close()

	for _, e := range entries {
		fi, err := os.Lstat(e.disk)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = e.arc
		if e.isDir {
			hdr.Name += "/"
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !quiet {
			fmt.Printf("  %s %s\n", cc.Dim("adding"), e.arc)
		}
		src, err := os.Open(e.disk)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func unpackTarGo(src, dest, format string, quiet bool) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	var tr *tar.Reader
	switch format {
	case "tar.gz":
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	case "tar.bz2":
		tr = tar.NewReader(bzip2.NewReader(f))
	default:
		tr = tar.NewReader(f)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target, err := safePath(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)|0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if !quiet {
				fmt.Printf("  %s %s\n", cc.Dim("extract"), hdr.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			// best-effort symlink; ignore errors on unsupported platforms
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}

// ─── external: tar ───────────────────────────────────────────────────────────

func packTarExternal(out, format string, inputs []string, quiet bool) error {
	// Windows tar may not have lzma/zstd support; fall back to 7z which handles both.
	if runtime.GOOS == "windows" && (format == "tar.lzma" || format == "tar.zst") {
		return pack7z(out, inputs, quiet)
	}
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("pack %s requires 'tar' (not found in PATH)", format)
	}
	var args []string
	switch format {
	case "tar.bz2":
		args = []string{"-cjf", out}
	case "tar.xz":
		args = []string{"-cJf", out}
	case "tar.lzma":
		args = []string{"--lzma", "-cf", out}
	case "tar.zst":
		args = []string{"--zstd", "-cf", out}
	default:
		return fmt.Errorf("unknown tar variant %q", format)
	}
	if !quiet {
		args = append(args, "-v")
	}
	args = append(args, inputs...)
	cmd := exec.Command("tar", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func unpackTarExternal(src, dest, format string, quiet bool) error {
	// Windows tar may not have lzma/zstd support; fall back to 7z which handles both.
	if runtime.GOOS == "windows" && (format == "tar.lzma" || format == "tar.zst") {
		return unpack7z(src, dest, quiet)
	}
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("unpack %s requires 'tar' (not found in PATH)", format)
	}
	var args []string
	switch format {
	case "tar.xz":
		args = []string{"-xJf", src, "-C", dest}
	case "tar.lzma":
		args = []string{"--lzma", "-xf", src, "-C", dest}
	case "tar.zst":
		args = []string{"--zstd", "-xf", src, "-C", dest}
	default:
		return fmt.Errorf("unknown tar variant %q", format)
	}
	if !quiet {
		args = append(args, "-v")
	}
	cmd := exec.Command("tar", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ─── external: 7z ────────────────────────────────────────────────────────────

func find7z() (string, error) {
	for _, name := range []string{"7z", "7za", "7zz"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("7z not found; install p7zip:\n  macOS: brew install p7zip\n  Debian: apt install p7zip-full\n  Arch: pacman -S p7zip")
}

func pack7z(out string, inputs []string, quiet bool) error {
	bin, err := find7z()
	if err != nil {
		return err
	}
	args := []string{"a", out}
	args = append(args, inputs...)
	cmd := exec.Command(bin, args...)
	if !quiet {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func unpack7z(src, dest string, quiet bool) error {
	bin, err := find7z()
	if err != nil {
		return err
	}
	args := []string{"x", src, "-o" + dest, "-y"}
	cmd := exec.Command(bin, args...)
	if !quiet {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ─── cobra commands ──────────────────────────────────────────────────────────

// PackCommand returns the cobra command for "babi pack".
func PackCommand() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "pack <output.ext> <file|dir> [file|dir...]",
		Short: "Create an archive from files/directories",
		Long: `Create an archive from one or more files or directories.

The output format is determined by the file extension:

  .zip              zip (deflate)
  .tar              uncompressed tar
  .tar.gz / .tgz    gzip-compressed tar
  .tar.bz2          bzip2-compressed tar  (requires tar command)
  .tar.xz / .txz    xz-compressed tar     (requires tar command)
  .tar.lzma         lzma-compressed tar   (requires tar command)
  .tar.zst / .tzst  zstd-compressed tar   (requires tar command)
  .7z               7-Zip archive         (requires 7z / p7zip)`,
		Example: `  babi pack archive.zip src/ README.md
  babi pack release.tar.gz dist/
  babi pack backup.tar.bz2 ~/Documents`,
		Args:              cobra.MinimumNArgs(2),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := args[0]
			inputs := args[1:]

			spec, err := detectFormat(out)
			if err != nil {
				return err
			}

			if !quiet {
				fmt.Printf("%s %s → %s\n",
					cc.Bold("pack"),
					cc.Dim(fmt.Sprintf("[%s]", spec.name)),
					cc.Cyan(out),
				)
			}

			if spec.stdlibPack {
				switch spec.name {
				case "zip":
					err = packZip(out, inputs, quiet)
				case "tar", "tar.gz":
					err = packTarGo(out, spec.name, inputs, quiet)
				}
			} else if spec.name == "7z" {
				err = pack7z(out, inputs, quiet)
			} else {
				err = packTarExternal(out, spec.name, inputs, quiet)
			}

			if err != nil {
				return err
			}
			if !quiet {
				fi, _ := os.Stat(out)
				if fi != nil {
					fmt.Printf("%s %s (%s)\n",
						cc.BoldGreen("✓"),
						cc.BrightWhite(out),
						formatSize(fi.Size()),
					)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress per-file output")
	return cmd
}

// UnpackCommand returns the cobra command for "babi unpack".
func UnpackCommand() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "unpack <archive> [dest-dir]",
		Short: "Extract an archive",
		Long: `Extract an archive into a destination directory.

If dest-dir is omitted the current directory is used.
Supported formats: .zip  .tar  .tar.gz/.tgz  .tar.bz2  .tar.xz  .tar.lzma  .tar.zst  .7z`,
		Example: `  babi unpack archive.zip
  babi unpack release.tar.gz /tmp/out
  babi unpack backup.tar.bz2 ~/restore`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			dest := "."
			if len(args) == 2 {
				dest = args[1]
			}

			spec, err := detectFormat(src)
			if err != nil {
				return err
			}

			if err := os.MkdirAll(dest, 0755); err != nil {
				return fmt.Errorf("create dest dir: %w", err)
			}

			if !quiet {
				fmt.Printf("%s %s → %s\n",
					cc.Bold("unpack"),
					cc.Dim(fmt.Sprintf("[%s]", spec.name)),
					cc.Cyan(dest),
				)
			}

			if spec.stdlibUnpack {
				switch spec.name {
				case "zip":
					err = unpackZip(src, dest, quiet)
				case "tar", "tar.gz", "tar.bz2":
					err = unpackTarGo(src, dest, spec.name, quiet)
				}
			} else if spec.name == "7z" {
				err = unpack7z(src, dest, quiet)
			} else {
				err = unpackTarExternal(src, dest, spec.name, quiet)
			}

			if err != nil {
				return err
			}
			if !quiet {
				fmt.Printf("%s done\n", cc.BoldGreen("✓"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress per-file output")
	return cmd
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
