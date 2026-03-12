package meta

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// Command returns the fully configured "babi meta" cobra command tree.
func Command() *cobra.Command {
	root := buildRoot()

	subs := []*cobra.Command{
		buildPlist(),
		buildRC(),
		buildManifest(),
		buildIni(),
		buildDesktop(),
		buildIco(),
		buildIcns(),
	}

	// Shared flags on all text-generation subcommands (everything except ico/icns).
	textSubs := subs[:5]
	for _, c := range textSubs {
		c.Flags().StringP("name", "n", "", "application / product name")
		c.Flags().String("version", "", "version string (default: 1.0.0)")
		c.Flags().String("description", "", "short description")
		c.Flags().StringP("output", "o", "", "output file path (default varies by subcommand)")
	}

	root.AddCommand(subs...)
	return root
}

// ─── root ─────────────────────────────────────────────────────────────────────

func buildRoot() *cobra.Command {
	return &cobra.Command{
		Use:   "meta",
		Short: "Generate platform metadata files (plist, rc, manifest, ini, desktop, ico, icns)",
		Long: `Generate boilerplate platform-specific metadata files.

  babi meta plist      # macOS Info.plist
  babi meta rc         # Windows resource script (.rc)
  babi meta manifest   # Windows application manifest (.manifest)
  babi meta ini        # Windows desktop.ini
  babi meta desktop    # Linux XDG .desktop entry
  babi meta ico        # Windows multi-resolution .ico (requires ImageMagick)
  babi meta icns       # macOS multi-resolution .icns (iconutil or ImageMagick)`,
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func getStr(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func getBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func writeTemplate(path, tmplStr string, data any) error {
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("internal template error: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := t.Execute(f, data); err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	fmt.Printf("[babi] wrote %s\n", path)
	return nil
}

// splitVersion splits "1.2.3" or "1.2.3.4" into major, minor, patch strings.
func splitVersion(v string) (major, minor, patch string) {
	parts := strings.SplitN(v, ".", 4)
	get := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return "0"
	}
	return get(0), get(1), get(2)
}

// ensureTrailingSemicolon adds a trailing ';' when the string is non-empty and
// lacks one — required by the XDG spec for list values.
func ensureTrailingSemicolon(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ";") {
		return s
	}
	return s + ";"
}

// findMagick returns the ImageMagick binary ("magick" v7, falls back to "convert" v6).
func findMagick() (string, error) {
	for _, bin := range []string{"magick", "convert"} {
		if _, err := exec.LookPath(bin); err == nil {
			return bin, nil
		}
	}
	return "", fmt.Errorf("ImageMagick not found — install it:\n  macOS:  brew install imagemagick\n  Debian: apt install imagemagick\n  Arch:   pacman -S imagemagick")
}

// ─── plist ────────────────────────────────────────────────────────────────────

const plistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>{{.Name}}</string>

	<key>CFBundleIdentifier</key>
	<string>{{.BundleID}}</string>

	<key>CFBundleVersion</key>
	<string>{{.Version}}</string>

	<key>CFBundleShortVersionString</key>
	<string>{{.Version}}</string>

	<key>CFBundleExecutable</key>
	<string>{{.Exe}}</string>

	<key>CFBundlePackageType</key>
	<string>APPL</string>

	<key>CFBundleSignature</key>
	<string>????</string>
{{- if .Category}}

	<key>LSApplicationCategoryType</key>
	<string>{{.Category}}</string>
{{- end}}
{{- if .Description}}

	<key>NSHumanReadableCopyright</key>
	<string>{{.Description}}</string>
{{- end}}

	<key>NSHighResolutionCapable</key>
	<true/>

	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
`

func buildPlist() *cobra.Command {
	c := &cobra.Command{
		Use:   "plist",
		Short: "Generate a macOS Info.plist",
		Long: `Generate a macOS application Info.plist.

  babi meta plist --name MyApp --id com.example.myapp --version 1.0.0
  babi meta plist --name MyApp --id com.example.myapp -o MyApp/Info.plist`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := getStr(cmd, "name")
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			bundleID, _ := cmd.Flags().GetString("id")
			if bundleID == "" {
				return fmt.Errorf("--id (bundle identifier) is required")
			}
			exe, _ := cmd.Flags().GetString("exe")
			if exe == "" {
				exe = name
			}
			ver := getStr(cmd, "version")
			if ver == "" {
				ver = "1.0.0"
			}
			out := getStr(cmd, "output")
			if out == "" {
				out = "Info.plist"
			}
			return writeTemplate(out, plistTmpl, struct {
				Name, BundleID, Version, Exe, Category, Description string
			}{
				Name:        name,
				BundleID:    bundleID,
				Version:     ver,
				Exe:         exe,
				Category:    getStr(cmd, "category"),
				Description: getStr(cmd, "description"),
			})
		},
	}
	c.Flags().String("id", "", "bundle identifier, e.g. com.example.myapp (required)")
	c.Flags().String("exe", "", "executable name inside the bundle (default: --name)")
	c.Flags().String("category", "", "LSApplicationCategoryType, e.g. public.app-category.developer-tools")
	return c
}

// ─── rc ───────────────────────────────────────────────────────────────────────

const rcTmpl = `#include <windows.h>

// Version information
VS_VERSION_INFO VERSIONINFO
FILEVERSION     {{.FileMajor}},{{.FileMinor}},{{.FilePatch}},0
PRODUCTVERSION  {{.FileMajor}},{{.FileMinor}},{{.FilePatch}},0
FILEFLAGSMASK   VS_FFI_FILEFLAGSMASK
FILEFLAGS       0x0L
FILEOS          VOS__WINDOWS32
FILETYPE        VFT_APP
FILESUBTYPE     VFT2_UNKNOWN
BEGIN
    BLOCK "StringFileInfo"
    BEGIN
        BLOCK "040904b0"
        BEGIN
            VALUE "CompanyName",      "{{.Company}}\0"
            VALUE "FileDescription",  "{{.Description}}\0"
            VALUE "FileVersion",      "{{.Version}}\0"
            VALUE "InternalName",     "{{.Name}}\0"
            VALUE "LegalCopyright",   "{{.Copyright}}\0"
            VALUE "OriginalFilename", "{{.Name}}.exe\0"
            VALUE "ProductName",      "{{.Name}}\0"
            VALUE "ProductVersion",   "{{.Version}}\0"
        END
    END
    BLOCK "VarFileInfo"
    BEGIN
        VALUE "Translation", 0x0409, 1200
    END
END
{{- if .Icon}}

// Application icon
1 ICON "{{.Icon}}"
{{- end}}
`

func buildRC() *cobra.Command {
	c := &cobra.Command{
		Use:   "rc",
		Short: "Generate a Windows resource script (.rc)",
		Long: `Generate a Windows resource script (.rc) with version information.
Compile with: rc resource.rc  (produces resource.res)

  babi meta rc --name MyApp --version 1.2.3 --company "Acme Corp"
  babi meta rc --name MyApp --version 1.0.0 --icon app.ico -o resource.rc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := getStr(cmd, "name")
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			ver := getStr(cmd, "version")
			if ver == "" {
				ver = "1.0.0"
			}
			major, minor, patch := splitVersion(ver)
			out := getStr(cmd, "output")
			if out == "" {
				out = "resource.rc"
			}
			return writeTemplate(out, rcTmpl, struct {
				Name, Version, FileMajor, FileMinor, FilePatch string
				Company, Description, Copyright, Icon          string
			}{
				Name:        name,
				Version:     ver,
				FileMajor:   major,
				FileMinor:   minor,
				FilePatch:   patch,
				Company:     getStr(cmd, "company"),
				Description: getStr(cmd, "description"),
				Copyright:   getStr(cmd, "copyright"),
				Icon:        getStr(cmd, "icon"),
			})
		},
	}
	c.Flags().String("company", "", "company name")
	c.Flags().String("copyright", "", `copyright string (e.g. "Copyright © 2025 Acme")`)
	c.Flags().String("icon", "", "path to .ico file to embed")
	return c
}

// ─── manifest ─────────────────────────────────────────────────────────────────

const manifestTmpl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">

  <assemblyIdentity
    version="{{.Version}}"
    processorArchitecture="*"
    name="{{.Name}}"
    type="win32"
  />
{{- if .Description}}

  <description>{{.Description}}</description>
{{- end}}

  <!-- Visual Styles: use the current Windows theme for controls -->
  <dependency>
    <dependentAssembly>
      <assemblyIdentity
        type="win32"
        name="Microsoft.Windows.Common-Controls"
        version="6.0.0.0"
        processorArchitecture="*"
        publicKeyToken="6595b64144ccf1df"
        language="*"
      />
    </dependentAssembly>
  </dependency>

  <!-- UAC privilege level -->
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="{{.UAC}}" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>

  <!-- DPI awareness -->
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </windowsSettings>
  </application>

</assembly>
`

func buildManifest() *cobra.Command {
	c := &cobra.Command{
		Use:   "manifest",
		Short: "Generate a Windows application manifest (.manifest)",
		Long: `Generate a Windows application manifest XML file.
Embed with: mt -manifest app.manifest -outputresource:app.exe;1

  babi meta manifest --name MyApp --version 1.0.0.0
  babi meta manifest --name MyApp --uac requireAdministrator -o app.manifest`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := getStr(cmd, "name")
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			ver := getStr(cmd, "version")
			if ver == "" {
				ver = "1.0.0.0"
			}
			uac := getStr(cmd, "uac")
			if uac == "" {
				uac = "asInvoker"
			}
			validUAC := map[string]bool{
				"asInvoker": true, "highestAvailable": true, "requireAdministrator": true,
			}
			if !validUAC[uac] {
				return fmt.Errorf("--uac must be one of: asInvoker, highestAvailable, requireAdministrator")
			}
			out := getStr(cmd, "output")
			if out == "" {
				out = "app.manifest"
			}
			return writeTemplate(out, manifestTmpl, struct {
				Name, Version, Description, UAC string
			}{name, ver, getStr(cmd, "description"), uac})
		},
	}
	c.Flags().String("uac", "asInvoker", "UAC level: asInvoker, highestAvailable, requireAdministrator")
	return c
}

// ─── ini ──────────────────────────────────────────────────────────────────────

const desktopIniTmpl = `[.ShellClassInfo]
{{- if .Icon}}
IconResource={{.Icon}}
{{- end}}
{{- if .Label}}
LocalizedResourceName={{.Label}}
{{- end}}
{{- if .InfoTip}}
InfoTip={{.InfoTip}}
{{- end}}
{{- if .View}}

[ViewState]
Mode=
Vid=
FolderType={{.View}}
{{- end}}
`

func buildIni() *cobra.Command {
	c := &cobra.Command{
		Use:   "ini",
		Short: "Generate a Windows desktop.ini",
		Long: `Generate a Windows desktop.ini folder customization file.
Place in a folder and set system+hidden attributes:
  attrib +s +h desktop.ini

  babi meta ini --icon myicon.ico,0 --name "My Folder"
  babi meta ini --icon shell32.dll,3 --info-tip "Shared assets" -o desktop.ini`,
		RunE: func(cmd *cobra.Command, args []string) error {
			view := getStr(cmd, "view")
			validViews := map[string]bool{
				"": true, "Documents": true, "Pictures": true,
				"Music": true, "Videos": true, "Generic": true,
			}
			if !validViews[view] {
				return fmt.Errorf("--view must be one of: Documents, Pictures, Music, Videos, Generic")
			}
			out := getStr(cmd, "output")
			if out == "" {
				out = "desktop.ini"
			}
			return writeTemplate(out, desktopIniTmpl, struct {
				Icon, Label, InfoTip, View string
			}{
				Icon:    getStr(cmd, "icon"),
				Label:   getStr(cmd, "name"),
				InfoTip: getStr(cmd, "info-tip"),
				View:    view,
			})
		},
	}
	c.Flags().String("icon", "", "icon path with optional index (e.g. myicon.ico,0 or shell32.dll,3)")
	c.Flags().String("info-tip", "", "tooltip text shown on folder hover")
	c.Flags().String("view", "", "folder view type: Documents, Pictures, Music, Videos, Generic")
	return c
}

// ─── desktop ──────────────────────────────────────────────────────────────────

// xdgMainCategories lists the registered Main Categories from the XDG menu spec.
// https://specifications.freedesktop.org/menu/latest/category-registry.html
var xdgMainCategories = []string{
	"AudioVideo", "Audio", "Video",
	"Development", "Education", "Game", "Graphics",
	"HealthFitness", "Network", "Office", "Science",
	"Settings", "System", "Utility",
}

const desktopTmpl = `[Desktop Entry]
Version=1.5
Type={{.Type}}
Name={{.Name}}
{{- if .GenericName}}
GenericName={{.GenericName}}
{{- end}}
{{- if .Comment}}
Comment={{.Comment}}
{{- end}}
{{- if .Exec}}
Exec={{.Exec}}
{{- end}}
{{- if .TryExec}}
TryExec={{.TryExec}}
{{- end}}
{{- if .Icon}}
Icon={{.Icon}}
{{- end}}
{{- if .Path}}
Path={{.Path}}
{{- end}}
Terminal={{.Terminal}}
{{- if .Categories}}
Categories={{.Categories}}
{{- end}}
{{- if .Keywords}}
Keywords={{.Keywords}}
{{- end}}
{{- if .MimeType}}
MimeType={{.MimeType}}
{{- end}}
StartupNotify={{.StartupNotify}}
{{- if .NoDisplay}}
NoDisplay=true
{{- end}}
{{- if .OnlyShowIn}}
OnlyShowIn={{.OnlyShowIn}}
{{- end}}
{{- if .NotShowIn}}
NotShowIn={{.NotShowIn}}
{{- end}}
`

func validateMainCategory(cats string) error {
	mainSet := make(map[string]bool, len(xdgMainCategories))
	for _, c := range xdgMainCategories {
		mainSet[c] = true
	}
	for _, c := range strings.Split(strings.TrimSuffix(cats, ";"), ";") {
		if mainSet[strings.TrimSpace(c)] {
			return nil
		}
	}
	return fmt.Errorf(
		"categories %q contains no recognised Main Category\nValid main categories: %s",
		cats, strings.Join(xdgMainCategories, ", "),
	)
}

func buildDesktop() *cobra.Command {
	c := &cobra.Command{
		Use:   "desktop",
		Short: "Generate a Linux XDG .desktop entry file",
		Long: `Generate a FreeDesktop.org XDG .desktop entry file.
Install to ~/.local/share/applications/ (per-user) or /usr/share/applications/ (system-wide).
After install, run: update-desktop-database ~/.local/share/applications/

  babi meta desktop --name MyApp --exec myapp --icon myapp --categories Utility
  babi meta desktop --name MyApp --exec "myapp %F" --categories "Development;Utility" --terminal
  babi meta desktop --name MyApp --exec myapp --categories Office --mime "text/plain;application/pdf"

Main Categories (pick at least one):
  AudioVideo  Audio       Video       Development  Education
  Game        Graphics    HealthFitness  Network   Office
  Science     Settings    System      Utility

Common Additional Categories:
  TextEditor  FileManager  TerminalEmulator  WebBrowser  IDE
  Building    Debugger     RevisionControl   Profiling
  Player      Recorder     Mixer             Sequencer
  2DGraphics  3DGraphics   Photography       Viewer
  Calendar    Email        Spreadsheet       WordProcessor
  InstantMessaging  VideoConference  FileTransfer  RemoteAccess
  ConsoleOnly  Archiving  Compression  Security  Monitor`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := getStr(cmd, "name")
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			entryType := getStr(cmd, "type")
			if entryType == "" {
				entryType = "Application"
			}
			validTypes := map[string]bool{"Application": true, "Link": true, "Directory": true}
			if !validTypes[entryType] {
				return fmt.Errorf("--type must be one of: Application, Link, Directory")
			}
			execStr := getStr(cmd, "exec")
			if execStr == "" && entryType == "Application" {
				return fmt.Errorf("--exec is required for Type=Application")
			}

			cats := ensureTrailingSemicolon(getStr(cmd, "categories"))
			if entryType == "Application" && cats != "" {
				if err := validateMainCategory(cats); err != nil {
					return err
				}
			}

			out := getStr(cmd, "output")
			if out == "" {
				out = strings.ToLower(strings.ReplaceAll(name, " ", "-")) + ".desktop"
			}

			return writeTemplate(out, desktopTmpl, struct {
				Type, Name, GenericName, Comment   string
				Exec, TryExec, Icon, Path          string
				Terminal, StartupNotify, NoDisplay bool
				Categories, Keywords, MimeType     string
				OnlyShowIn, NotShowIn              string
			}{
				Type:          entryType,
				Name:          name,
				GenericName:   getStr(cmd, "generic-name"),
				Comment:       getStr(cmd, "description"),
				Exec:          execStr,
				TryExec:       getStr(cmd, "try-exec"),
				Icon:          getStr(cmd, "icon"),
				Path:          getStr(cmd, "path"),
				Terminal:      getBool(cmd, "terminal"),
				StartupNotify: getBool(cmd, "startup-notify"),
				NoDisplay:     getBool(cmd, "no-display"),
				Categories:    cats,
				Keywords:      ensureTrailingSemicolon(getStr(cmd, "keywords")),
				MimeType:      ensureTrailingSemicolon(getStr(cmd, "mime")),
				OnlyShowIn:    ensureTrailingSemicolon(getStr(cmd, "only-show-in")),
				NotShowIn:     ensureTrailingSemicolon(getStr(cmd, "not-show-in")),
			})
		},
	}
	c.Flags().String("generic-name", "", `generic descriptor, e.g. "Web Browser"`)
	c.Flags().String("exec", "", `command to execute, e.g. "myapp %F"`)
	c.Flags().String("icon", "", "icon name (theme lookup) or absolute path")
	c.Flags().String("type", "Application", "entry type: Application, Link, Directory")
	c.Flags().String("categories", "", `semicolon-separated XDG categories, e.g. "Utility;TextEditor"`)
	c.Flags().String("keywords", "", "semicolon-separated search keywords")
	c.Flags().String("mime", "", `semicolon-separated MIME types, e.g. "text/plain;image/png"`)
	c.Flags().Bool("terminal", false, "run inside a terminal emulator")
	c.Flags().Bool("startup-notify", false, "send startup notification")
	c.Flags().Bool("no-display", false, "hide from menus (keeps MIME associations active)")
	c.Flags().String("try-exec", "", "path to check app is installed before showing entry")
	c.Flags().String("path", "", "working directory when launching")
	c.Flags().String("only-show-in", "", `semicolon-separated DEs to show in, e.g. "GNOME;KDE"`)
	c.Flags().String("not-show-in", "", "semicolon-separated DEs to hide from")
	return c
}

// ─── ico ──────────────────────────────────────────────────────────────────────

func buildIco() *cobra.Command {
	c := &cobra.Command{
		Use:   "ico <input> [output.ico]",
		Short: "Generate a Windows multi-resolution .ico from an image",
		Long: `Convert a source image (PNG recommended) into a Windows .ico file
containing multiple resolutions. Requires ImageMagick (magick).

  babi meta ico app.png
  babi meta ico app.png app.ico
  babi meta ico app.png app.ico --sizes 16,32,48,256`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]
			output := "app.ico"
			if len(args) == 2 {
				output = args[1]
			}

			sizes, _ := cmd.Flags().GetString("sizes")
			if sizes == "" {
				sizes = "16,24,32,48,64,128,256"
			}

			magick, err := findMagick()
			if err != nil {
				return err
			}
			c := exec.Command(magick, input, "-define", "icon:auto-resize="+sizes, output)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("magick failed: %w", err)
			}
			fmt.Printf("[babi] wrote %s  (sizes: %s)\n", output, sizes)
			return nil
		},
	}
	c.Flags().String("sizes", "", "comma-separated pixel sizes (default: 16,24,32,48,64,128,256)")
	return c
}

// ─── icns ─────────────────────────────────────────────────────────────────────

func buildIcns() *cobra.Command {
	c := &cobra.Command{
		Use:   "icns <input> [output.icns]",
		Short: "Generate a macOS multi-resolution .icns from an image",
		Long: `Convert a source image (PNG recommended, 1024×1024 or larger) into a
macOS .icns file. On macOS uses iconutil (preferred); falls back to ImageMagick
(magick) on other platforms or when iconutil is unavailable.

  babi meta icns app.png
  babi meta icns app.png app.icns
  babi meta icns app.png app.icns --sizes 16,32,128,256,512,1024`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]
			output := "app.icns"
			if len(args) == 2 {
				output = args[1]
			}

			if runtime.GOOS == "darwin" {
				if _, err := exec.LookPath("iconutil"); err == nil {
					return buildIcnsWithIconutil(input, output)
				}
			}

			sizes, _ := cmd.Flags().GetString("sizes")
			if sizes == "" {
				sizes = "16,32,64,128,256,512,1024"
			}
			magick, err := findMagick()
			if err != nil {
				return err
			}
			c := exec.Command(magick, input, "-define", "icon:auto-resize="+sizes, output)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("magick failed: %w", err)
			}
			fmt.Printf("[babi] wrote %s  (sizes: %s)\n", output, sizes)
			return nil
		},
	}
	c.Flags().String("sizes", "", "comma-separated pixel sizes for magick fallback (default: 16,32,64,128,256,512,1024)")
	return c
}

// buildIcnsWithIconutil creates a temporary .iconset directory, populates it
// with the required PNG sizes via ImageMagick, then runs iconutil.
func buildIcnsWithIconutil(input, output string) error {
	magick, err := findMagick()
	if err != nil {
		return err
	}

	type entry struct {
		name string
		size int
	}
	entries := []entry{
		{"icon_16x16.png", 16},
		{"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32},
		{"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128},
		{"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256},
		{"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512},
		{"icon_512x512@2x.png", 1024},
	}

	iconsetDir := strings.TrimSuffix(output, filepath.Ext(output)) + ".iconset"
	if err := os.MkdirAll(iconsetDir, 0o755); err != nil {
		return fmt.Errorf("create iconset dir: %w", err)
	}
	defer os.RemoveAll(iconsetDir)

	fmt.Printf("[babi] building iconset in %s\n", iconsetDir)
	for _, e := range entries {
		dest := filepath.Join(iconsetDir, e.name)
		sizeStr := fmt.Sprintf("%dx%d", e.size, e.size)
		c := exec.Command(magick, input, "-resize", sizeStr, dest)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("resize to %s: %w", sizeStr, err)
		}
	}

	c := exec.Command("iconutil", "-c", "icns", "-o", output, iconsetDir)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("iconutil failed: %w", err)
	}
	fmt.Printf("[babi] wrote %s\n", output)
	return nil
}
