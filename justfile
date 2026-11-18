# mygo cross-compiles every platform from Linux, so the build recipes are
# bash; CI (windows-exe.yml) runs them on ubuntu-latest.

app_id := "pray-time"
# Go toolchain: mygo needs Go 1.27 or newer (mise installs it, see README).
version := `scripts/version`
linux_tarball := "build/linux-amd64/pray-time-" + version + "-linux-amd64.tar.gz"
windows_setup := "build/windows-amd64/pray-time-setup-" + version + "-amd64.exe"

# List available recipes.
default:
    @just --list

# Run the app with rebuild and restart on save.
dev:
    go tool mygo dev

# Run the unit and view tests.
test:
    go test ./...

# Save the views as PNGs in DIR (light and dark), from the headless view tests.
snapshots dir="build/snapshots":
    mkdir -p {{dir}}
    PRAYTIME_SNAPSHOTS="$(realpath {{dir}})" go test -run 'Popup|Reminder|Settings' -count=1 .

# Build for the given platforms (default: Linux amd64) into build/<os>-<arch>/.
build platforms="linux/amd64":
    scripts/build {{platforms}}

# Build the Linux app and stage its tarball (with install.sh) in _apk/.
build-linux:
    scripts/build linux/amd64
    bash scripts/stage-artifact '{{linux_tarball}}' '{{app_id}}' '{{version}}' _apk

# Build the Windows installer (needs makensis) and stage it in _apk/.
build-exe:
    #!/usr/bin/env bash
    set -euo pipefail
    command -v makensis >/dev/null || { echo "Install NSIS (makensis) to build the Windows installer." >&2; exit 1; }
    scripts/build windows/amd64
    bash scripts/stage-artifact '{{windows_setup}}' '{{app_id}}' '{{version}}' _apk

# Build Linux and Windows in one pass and stage both in _apk/ (what CI runs).
stage-all:
    scripts/build linux/amd64,windows/amd64
    bash scripts/stage-artifact '{{linux_tarball}}' '{{app_id}}' '{{version}}' _apk
    bash scripts/stage-artifact '{{windows_setup}}' '{{app_id}}' '{{version}}' _apk

# Install the built Linux app for this user (~/.local, launcher entry, `pray-time` command).
install:
    #!/usr/bin/env bash
    set -euo pipefail
    [[ -f '{{linux_tarball}}' ]] || scripts/build linux/amd64
    sh build/linux-amd64/install.sh '{{linux_tarball}}'

# Remove the user install (settings in ~/.config/praytime stay).
uninstall:
    sh build/linux-amd64/install.sh --uninstall

# Build, stage, and install the Linux app.
update: build-linux
    sh build/linux-amd64/install.sh '{{linux_tarball}}'

# Wait for the GitHub Actions build of the pushed HEAD, save it in _apk/, and share it via Gokapi.
[linux]
ci-exe:
    bash scripts/ci-exe

# Tag v<version> and push it: CI publishes a GitHub release with the installers.
release:
    #!/usr/bin/env bash
    set -euo pipefail
    git diff --quiet && git diff --cached --quiet || { echo "Commit first." >&2; exit 1; }
    git tag "v{{version}}"
    git push origin "v{{version}}"

# Bump major.minor in mygo.json (the last field is the commit count): just bump [major|minor].
bump part="minor":
    #!/usr/bin/env bash
    set -euo pipefail
    current=$(jq -r '.version' mygo.json)
    IFS='.' read -r major minor _ <<< "$current"
    case "{{part}}" in
      major) major=$((major+1)); minor=0;;
      minor) minor=$((minor+1));;
      *) echo "unknown part '{{part}}' (use major or minor)"; exit 1;;
    esac
    jq --arg v "$major.$minor.0" '.version = $v' mygo.json > mygo.json.tmp && mv mygo.json.tmp mygo.json
    echo "version: $current -> $major.$minor.0"
