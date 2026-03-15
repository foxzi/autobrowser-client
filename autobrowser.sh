#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${SCRIPT_DIR}/.venv"
PYTHON="${VENV_DIR}/bin/python"
PIP="${VENV_DIR}/bin/pip"
AUTOBROWSER="${VENV_DIR}/bin/autobrowser"
PATCHRIGHT="${VENV_DIR}/bin/patchright"

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [args...]

Commands:
  setup       Create venv, install dependencies and patchright browser
  run [args]  Run autobrowser CLI with given arguments
  test [args] Run pytest with given arguments
  clean       Remove .venv directory

Examples:
  $(basename "$0") setup
  $(basename "$0") run version
  $(basename "$0") run run-script --file examples/simple.yaml
  $(basename "$0") run serve --port 8080
  $(basename "$0") test
  $(basename "$0") test tests/test_config.py -v
  $(basename "$0") clean
EOF
}

ensure_venv() {
    if [ ! -d "$VENV_DIR" ]; then
        echo "Error: venv not found. Run '$(basename "$0") setup' first."
        exit 1
    fi
}

ensure_patchright() {
    if ! "$PATCHRIGHT" install --dry-run chrome >/dev/null 2>&1; then
        echo "Installing patchright chrome browser..."
        "$PATCHRIGHT" install chrome
    fi
}

cmd_setup() {
    echo "Creating venv in ${VENV_DIR}..."
    python3 -m venv "$VENV_DIR"

    echo "Upgrading pip..."
    "$PIP" install --upgrade pip

    echo "Installing project in editable mode..."
    "$PIP" install -e ".[dev]"

    echo "Installing patchright chrome browser..."
    "$PATCHRIGHT" install chrome

    echo "Setup complete."
    echo "Run '$(basename "$0") run version' to verify."
}

cmd_run() {
    ensure_venv
    exec "$AUTOBROWSER" "$@"
}

cmd_test() {
    ensure_venv
    exec "$VENV_DIR/bin/pytest" "$@"
}

cmd_clean() {
    if [ -d "$VENV_DIR" ]; then
        echo "Removing ${VENV_DIR}..."
        rm -rf "$VENV_DIR"
        echo "Done."
    else
        echo "Nothing to clean, ${VENV_DIR} does not exist."
    fi
}

if [ $# -eq 0 ]; then
    usage
    exit 1
fi

COMMAND="$1"
shift

case "$COMMAND" in
    setup) cmd_setup ;;
    run)   cmd_run "$@" ;;
    test)  cmd_test "$@" ;;
    clean) cmd_clean ;;
    -h|--help|help) usage ;;
    *)
        echo "Unknown command: $COMMAND"
        usage
        exit 1
        ;;
esac
