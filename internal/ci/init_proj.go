package ci

// Templates for 'babi ci init' — creates a hello world C/CMake project
// with a babi_build.py cross-platform build script.

const cmakelists = `cmake_minimum_required(VERSION 3.15)
project(hello C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)

add_executable(hello src/main.c)

install(TARGETS hello RUNTIME DESTINATION .)
`

const mainC = `#include <stdio.h>

int main(void) {
    printf("Hello, babi!\n");
    return 0;
}
`

const babiBuildPy = `#!/usr/bin/env python3
"""
babi_build.py - Cross-platform build script for babi CI.

Usage:
    python3 babi_build.py [params.json]

Reads params.json for optional configuration overrides.

Platforms handled:
  macOS   -> macos-universal-hello.zip  (arm64 + x86_64 lipo'd)
             linux-arm64-hello.zip      (via docker linux/arm64)
  Windows -> windows-x86_64-hello.zip  (native MSVC/MinGW x64)
             linux-x86_64-hello.zip    (via docker linux/amd64)
  Linux   -> linux-{arch}-hello.zip    (native build, used inside docker)
"""

import json
import os
import platform
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

OUTPUT_DIR = Path("output")
PROJECT_NAME = "hello"  # overridden at runtime from params.json


def run(cmd, cwd=None):
    print(f"[build] + {' '.join(str(c) for c in cmd)}", flush=True)
    subprocess.run(cmd, check=True, cwd=cwd)


def cmake_build(build_dir, cmake_args=None):
    src = str(Path(".").resolve())
    build_dir = Path(build_dir)
    build_dir.mkdir(parents=True, exist_ok=True)

    configure = ["cmake", src, "-B", str(build_dir), "-DCMAKE_BUILD_TYPE=Release"]
    if cmake_args:
        configure.extend(cmake_args)
    run(configure)
    run(["cmake", "--build", str(build_dir), "--config", "Release"])


def find_binary(build_dir, name):
    """Find the compiled binary in common CMake output locations."""
    for candidate in [
        Path(build_dir) / name,
        Path(build_dir) / f"{name}.exe",
        Path(build_dir) / "Release" / name,
        Path(build_dir) / "Release" / f"{name}.exe",
        Path(build_dir) / "Debug" / name,
    ]:
        if candidate.exists():
            return candidate
    return None


def make_zip(zip_path, files):
    """Create a zip archive. files is a list of (src_path, arc_name) tuples."""
    zip_path = Path(zip_path)
    zip_path.parent.mkdir(parents=True, exist_ok=True)
    print(f"[build] creating {zip_path}", flush=True)
    with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for src, arc_name in files:
            zf.write(str(src), arc_name)
    size = zip_path.stat().st_size
    print(f"[build] {zip_path} ({size:,} bytes)", flush=True)


def docker_build(platform_str, out_name, docker_image="debian:stable-slim"):
    """
    Build inside a Docker container for the given platform.
    Installs cmake, gcc, and python3 via apt before building.
    Returns the path to the built binary, or None on failure.
    """
    src_abs = str(Path(".").resolve())
    container_out = "/build-out"
    print(f"[build] docker build ({platform_str})...", flush=True)

    install_deps = (
        "apt-get update -qq && "
        "apt-get install -y --no-install-recommends cmake gcc python3 ca-certificates && "
        "rm -rf /var/lib/apt/lists/*"
    )
    build_cmds = (
        f"cmake /src -B /bld -DCMAKE_BUILD_TYPE=Release && "
        f"cmake --build /bld --config Release && "
        f"cp /bld/{out_name} {container_out}/{out_name}"
    )

    cmd = [
        "docker", "run", "--rm",
        "--platform", platform_str,
        "-v", f"{src_abs}:/src:ro",
        "-v", f"{src_abs}/docker-out-{platform_str.replace('/', '-')}:{container_out}",
        docker_image,
        "sh", "-c",
        f"{install_deps} && {build_cmds}",
    ]

    host_out_dir = Path(f"docker-out-{platform_str.replace('/', '-')}")
    host_out_dir.mkdir(exist_ok=True)

    try:
        run(cmd)
    except subprocess.CalledProcessError as e:
        print(f"[build] docker build failed: {e}", flush=True)
        return None

    result = host_out_dir / out_name
    return result if result.exists() else None


def build_macos(params):
    docker_image = params.get("docker_image", "gcc:latest")
    OUTPUT_DIR.mkdir(exist_ok=True)

    # --- macOS universal binary ---
    print("[build] building macOS arm64...", flush=True)
    cmake_build("build-arm64", ["-DCMAKE_OSX_ARCHITECTURES=arm64"])
    exe_arm = find_binary("build-arm64", PROJECT_NAME)

    print("[build] building macOS x86_64...", flush=True)
    cmake_build("build-x86_64", ["-DCMAKE_OSX_ARCHITECTURES=x86_64"])
    exe_x86 = find_binary("build-x86_64", PROJECT_NAME)

    if exe_arm and exe_x86:
        universal = OUTPUT_DIR / PROJECT_NAME
        run(["lipo", "-create", "-output", str(universal), str(exe_arm), str(exe_x86)])
        make_zip(OUTPUT_DIR / f"macos-universal-{PROJECT_NAME}.zip", [(universal, PROJECT_NAME)])
    else:
        print("[build] WARNING: could not find macOS binaries for lipo", flush=True)

    # --- Linux arm64 via Docker ---
    linux_exe = docker_build("linux/arm64", PROJECT_NAME, docker_image)
    if linux_exe:
        make_zip(OUTPUT_DIR / f"linux-arm64-{PROJECT_NAME}.zip", [(linux_exe, PROJECT_NAME)])
    else:
        print("[build] WARNING: linux/arm64 docker build failed", flush=True)


def build_windows(params):
    docker_image = params.get("docker_image", "gcc:latest")
    OUTPUT_DIR.mkdir(exist_ok=True)

    # --- Windows x86_64 native ---
    print("[build] building Windows x86_64...", flush=True)
    cmake_build("build-win64", ["-A", "x64"])
    exe_win = find_binary("build-win64", PROJECT_NAME)
    if exe_win:
        make_zip(OUTPUT_DIR / f"windows-x86_64-{PROJECT_NAME}.zip",
                 [(exe_win, f"{PROJECT_NAME}.exe")])
    else:
        print("[build] WARNING: Windows binary not found", flush=True)

    # --- Linux x86_64 via Docker ---
    linux_exe = docker_build("linux/amd64", PROJECT_NAME, docker_image)
    if linux_exe:
        make_zip(OUTPUT_DIR / f"linux-x86_64-{PROJECT_NAME}.zip", [(linux_exe, PROJECT_NAME)])
    else:
        print("[build] WARNING: linux/amd64 docker build failed", flush=True)


def build_linux(params):
    OUTPUT_DIR.mkdir(exist_ok=True)
    arch = platform.machine().lower()
    print(f"[build] building Linux {arch} natively...", flush=True)
    cmake_build("build-linux")
    exe = find_binary("build-linux", PROJECT_NAME)
    if exe:
        make_zip(OUTPUT_DIR / f"linux-{arch}-{PROJECT_NAME}.zip", [(exe, PROJECT_NAME)])
    else:
        print("[build] WARNING: Linux binary not found", flush=True)


def main():
    params_file = sys.argv[1] if len(sys.argv) > 1 else "params.json"
    params = {}
    if os.path.exists(params_file):
        with open(params_file) as f:
            params = json.load(f)
        print(f"[build] loaded params: {params}", flush=True)

    # PROJECT_NAME: from params (injected by runner), then directory name, then "hello"
    global PROJECT_NAME
    PROJECT_NAME = params.get("project_name", None) or Path(".").resolve().name or "hello"
    print(f"[build] project name: {PROJECT_NAME}", flush=True)

    system = platform.system().lower()
    print(f"[build] platform: {system} / {platform.machine()}", flush=True)

    if system == "darwin":
        build_macos(params)
    elif system == "windows":
        build_windows(params)
    elif system == "linux":
        build_linux(params)
    else:
        print(f"[build] unsupported platform: {system}", flush=True)
        sys.exit(1)

    print("[build] all done!", flush=True)


if __name__ == "__main__":
    main()
`

const ciCommonGitignore = `# editors
.vscode/
.idea/
*.swp

# macOS
.DS_Store
.AppleDouble
.LSOverride

# Linux
*~

# Windows
Thumbs.db
Desktop.ini
$RECYCLE.BIN/
`

const ciGitignore = `# Build directories
build-*/
docker-out-*/
output/

# CMake
CMakeFiles/
CMakeCache.txt
cmake_install.cmake
Makefile
*.cmake

# Compiled objects
*.o
*.a
*.so
*.dylib
*.dll
*.exe

# params (generated by CI runner)
params.json

` + ciCommonGitignore
