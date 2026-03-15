## Usage:
  babi [command]

Legend: 🖥️ interactive TUI &nbsp;·&nbsp; ⚡ CLI tool

## Available Commands:

-  ⚡ `cf`          Cut and format text fields from each line
   ```sh
   echo "alice 30" | babi cf ' ' '{part[0]} is {part[1]} years old'
   babi cf ',' '{part[0]}: {part[1]}' data.csv
   babi cf --maxsplit=1 ':' '[{part[0]}] {part[1]}' /etc/passwd
   babi cf --filter='^ERROR' ' ' '{part[1]}: {part[2]}' app.log
   ```

-  ⚡ `check`       Check for required third-party binaries
   ```sh
   babi check
   ```

-  🖥️ `commit`      Open the commitizen commit TUI
   ```sh
   babi commit
   babi commit run "feat: add login page"   # non-interactive
   ```

-  ⚡ `convert`     Convert files between formats (images, video, audio, docs)
   ```sh
   babi convert photo.heic photo.jpg        # image format
   babi convert clip.mov clip.mp4           # video format
   babi convert video.mp4 audio.mp3         # video → audio
   babi convert notes.md notes.pdf          # document (pandoc)
   babi convert crop photo.jpg out.jpg --size 800x600
   babi convert trim video.mp4 out.mp4 --start 00:01:00 --end 00:02:30
   babi convert compress video.mp4 small.mp4
   babi convert frames video.mp4 ./frames/
   babi convert slideshow ./images/ output.mp4
   ```

-  ⚡ `dt`          Date and time utilities
   ```sh
   babi dt                                   # current time + common offsets
   babi dt in 1h                             # time 1 hour from now
   babi dt in -2d                            # time 2 days ago
   babi dt in 5w2d --from 03/02/2026         # offset from a specific date (DD/MM/YYYY)
   babi dt age 1990-06-15                    # age from birthdate
   babi dt tz America/New_York               # current time in timezone
   babi dt ntp                               # query NTP servers
   babi dt ntp --sync                        # sync system clock
   ```

-  🖥️ `edit`        Open the text editor
   ```sh
   babi edit
   babi edit notes.txt
   ```

-  ⚡ `encode`      Encode and decode data (base64, hex, URL)
   ```sh
   babi encode b64  "hello"           # base64 encode
   babi encode b64d "aGVsbG8="        # base64 decode
   babi encode hex  "hello"           # hex encode
   babi encode hexd "68656c6c6f"      # hex decode
   babi encode url  "hello world"     # URL encode
   babi encode urld "hello%20world"   # URL decode
   echo "hello" | babi encode b64     # read from stdin
   ```

-  🖥️ `fm`          Open the two-pane file manager
   ```sh
   babi fm
   babi fm ~/Documents
   ```

-  ⚡ `gen`         Generate UUIDs, passwords, and random strings
   ```sh
   babi gen uuid                  # random UUID v4
   babi gen uuid -n 5             # generate 5 UUIDs
   babi gen pass                  # 20-char password
   babi gen pass -l 32 -s         # 32-char with symbols
   babi gen str                   # 16-char alphanumeric string
   babi gen str -l 32 -c hex      # 32-char hex string
   ```

-  ⚡ `hash`        Hash files or strings
   ```sh
   babi hash file.zip             # sha256 (default)
   babi hash -a md5 file.zip      # md5
   babi hash -a sha1 -s "hello"   # sha1 of a string
   babi hash *.go                 # hash multiple files
   ```

-  ⚡ `help`        Help about any command
   ```sh
   babi help
   babi help commit
   ```

-  🖥️ `hex`         Open the hex editor
   ```sh
   babi hex binary.bin
   ```

-  ⚡ `ip`          Show local IP address for the internet-facing interface
   ```sh
   babi ip            # primary interface IP
   babi ip --all      # list all non-loopback interfaces
   ```

-  🖥️ `log`         Interactive git log viewer
   ```sh
   babi log
   ```

-  ⚡ `meta`        Generate platform metadata files (plist, rc, manifest, ini, desktop, ico, icns)
   ```sh
   babi meta plist                # macOS Info.plist
   babi meta rc                   # Windows .rc resource script
   babi meta manifest             # Windows .manifest XML
   babi meta ini                  # Windows desktop.ini
   babi meta desktop              # Linux XDG .desktop entry
   babi meta ico icon.png         # Windows .ico from image
   babi meta icns icon.png        # macOS .icns from image
   ```

-  ⚡ `new`        Scaffold a project
   ```sh
   babi new python --name banana --version 1.0.0   # Python + uv 
   ```

-  ⚡ `pack`        Create an archive from files/directories
   ```sh
   babi pack out.zip src/ readme.md
   babi pack out.tar.gz dist/
   babi pack out.tar.zst build/
   babi pack out.7z assets/
   ```

-  ⚡ `pdf`         PDF utilities (merge, split)
   ```sh
   babi pdf merge combined.pdf a.pdf b.pdf c.pdf
   babi pdf split doc.pdf ./parts/
   babi pdf split doc.pdf ./parts/ --span 5
   babi pdf split doc.pdf ./parts/ --pages 3,6,9
   ```

-  ⚡ `port`        Show what process is using a port
   ```sh
   babi port 3000           # show process on port 3000
   babi port 3000 --kill    # kill the process on port 3000
   babi port list           # list all listening ports
   ```

-  ⚡ `replace`     Find and replace across files
   ```sh
   babi replace "foo" "bar"
   babi replace "foo" "bar" ./src -t go
   babi replace "foo" "bar" --dry-run
   babi replace -F "literal.string" "replacement" ./docs
   ```

-  ⚡ `search`      Search for a pattern across files
   ```sh
   babi search "TODO"
   babi search "func.*Error" ./internal -t go
   babi search "password" -i -l
   babi search "error" app.log -C 2
   ```

-  ⚡ `serve`       HTTP server utilities
   ```sh
   babi serve web              # static file server (current dir)
   babi serve web ./dist       # static file server (custom dir)
   babi serve dir              # interactive directory browser
   babi serve dir ~/Videos     # browser with video streaming
   ```

-  🖥️ `stash`       Interactive git stash manager
   ```sh
   babi stash
   ```

-  🖥️ `sync`        Open the file-sync TUI
   ```sh
   babi sync
   babi sync add backup ~/Documents /mnt/backup/docs
   babi sync list
   babi sync run backup
   babi sync remove backup
   ```

-  ⚡ `tag`         Bump the version tag and push it
   ```sh
   babi tag patch             # v1.2.3 → v1.2.4
   babi tag minor             # v1.2.3 → v1.3.0
   babi tag major             # v1.2.3 → v2.0.0
   babi tag set v1.0.0        # create exact tag
   babi tag undo              # delete last semver tag
   ```

-  🖥️ `type`        Touch-typing tutor (fast / slow / normal method)
   Based on code - https://github.com/pb-/gotypist
   ```sh
   babi type                      # random words from built-in dictionary
   babi type hello world          # type a specific phrase
   babi type -f wordlist.txt      # use a custom word list
   babi type -c -f mycode.go      # code-line mode (sequential)
   babi type -n 0.2               # mix in 20% random numbers
   ```

-  ⚡ `tree`        Display directory contents as a tree
   ```sh
   babi tree
   babi tree ./src
   ```

-  ⚡ `unpack`      Extract an archive
   ```sh
   babi unpack archive.zip
   babi unpack archive.tar.gz ./output/
   babi unpack release.7z ./dist/
   ```

## Flags:
-  `-h, --help`            help for babi
-  `-v, --version`         version for babi

Use `"babi [command] --help"` for more information about a command.

----------

Copyright (C) 2026 Bhathiya Perera

This work is free. You can redistribute it and/or modify it under the
terms of the WTFPL, Version 2.
See the LICENSE file for more details.
