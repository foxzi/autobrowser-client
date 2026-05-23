#!/bin/bash
# compare-engines.sh - Compare detection site results across browser engines
#
# This script runs a YAML scenario template against multiple browser engines
# (patchright and cloakbrowser) to compare how they handle detection sites.
#
# For cloakbrowser, the script automatically injects --fingerprint-platform=windows
# via a generated config file to spoof the Windows platform.
#
# Before running, ensure cloakbrowser is installed:
#   .venv/bin/pip install cloakbrowser
#   .venv/bin/python -m cloakbrowser install
#
# Usage:
#   bash scripts/compare-engines.sh [OPTIONS]
#
# Options:
#   --headful              Run in headful mode (default: headless)
#   --engine <name>        Engine to test (repeatable; default: patchright cloakbrowser)
#   --keep-tmp             Keep generated YAML files after completion
#   -h, --help             Show this help message
#
# Examples:
#   bash scripts/compare-engines.sh
#   bash scripts/compare-engines.sh --headful --engine patchright
#   bash scripts/compare-engines.sh --engine patchright --engine cloakbrowser --keep-tmp

set -euo pipefail

HEADLESS=true
ENGINES=()
KEEP_TMP=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="${PROJECT_ROOT}/output/engine-compare"
TEMPLATE="${SCRIPT_DIR}/engine-compare.yaml.tpl"

usage() {
	sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# //'
	exit 0
}

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage
			;;
		--headful)
			HEADLESS=false
			shift
			;;
		--keep-tmp)
			KEEP_TMP=true
			shift
			;;
		--engine)
			if [[ -z "${2:-}" ]]; then
				echo "Error: --engine requires an argument" >&2
				exit 1
			fi
			ENGINES+=("$2")
			shift 2
			;;
		*)
			echo "Error: unknown option '$1'" >&2
			exit 1
			;;
		esac
	done

	# Default engines if none specified
	if [[ ${#ENGINES[@]} -eq 0 ]]; then
		ENGINES=("patchright" "cloakbrowser")
	fi
}

validate_files() {
	if [[ ! -f "$TEMPLATE" ]]; then
		echo "Error: template not found: $TEMPLATE" >&2
		exit 1
	fi
	if [[ ! -f "${PROJECT_ROOT}/.venv/bin/autobrowser" ]]; then
		echo "Error: autobrowser CLI not found" >&2
		exit 1
	fi
}

render_template() {
	local engine="$1"
	local output_yaml="${OUTPUT_DIR}/_run-${engine}.yaml"

	# Ensure output directory exists
	mkdir -p "${OUTPUT_DIR}/${engine}"

	# Copy template and replace __ENGINE__ placeholder
	sed "s|__ENGINE__|${engine}|g" "$TEMPLATE" >"$output_yaml"

	# If headful mode, disable headless
	if [[ "$HEADLESS" == "false" ]]; then
		sed -i 's/headless: true/headless: false/' "$output_yaml"
	fi

	echo "$output_yaml"
}

run_engine() {
	local engine="$1"
	local yaml_file="$2"
	local config_file="${3:-}"
	local start_time
	local end_time
	local duration

	start_time=$(date +%s)

	echo "Running $engine..."
	{
		AUTOBROWSER_BROWSER_ENGINE="$engine" \
			"${PROJECT_ROOT}/.venv/bin/autobrowser" run-script --file "$yaml_file" \
			${config_file:+--config "$config_file"}
	} 2>&1 | tee "${OUTPUT_DIR}/${engine}/run.log" || {
		local exit_code=$?
		echo "FAIL: $engine exited with code $exit_code"
		return "$exit_code"
	}

	end_time=$(date +%s)
	duration=$((end_time - start_time))

	echo "PASS: $engine completed in ${duration}s"
}

list_outputs() {
	local engine="$1"
	local engine_dir="${OUTPUT_DIR}/${engine}"

	if [[ -d "$engine_dir" ]]; then
		echo ""
		echo "Output files for $engine:"
		ls -la "$engine_dir"/*.png "$engine_dir"/*.html 2>/dev/null || echo "(no output files found)"
	fi
}

make_cb_config() {
	local cfg="${OUTPUT_DIR}/_run-cloakbrowser-config.yaml"
	cat >"$cfg" <<'YAML'
browser:
  launchArgs:
    - --fingerprint-platform=windows
YAML
	echo "$cfg"
}

cleanup_tmp() {
	if [[ "$KEEP_TMP" == "false" ]]; then
		for engine in "${ENGINES[@]}"; do
			local yaml_file="${OUTPUT_DIR}/_run-${engine}.yaml"
			if [[ -f "$yaml_file" ]]; then
				rm -f "$yaml_file"
			fi
		done
		local cb_config="${OUTPUT_DIR}/_run-cloakbrowser-config.yaml"
		if [[ -f "$cb_config" ]]; then
			rm -f "$cb_config"
		fi
	fi
}

print_hints() {
	echo ""
	echo "To compare screenshots side by side, open:"
	for site in sannysoft creepjs browserscan areyouheadless botd; do
		echo "  xdg-open ${OUTPUT_DIR}/patchright/${site}.png ${OUTPUT_DIR}/cloakbrowser/${site}.png"
	done
}

main() {
	parse_args "$@"
	validate_files

	echo "Comparing engines: ${ENGINES[*]}"
	echo "Headless mode: $HEADLESS"
	echo "Output directory: $OUTPUT_DIR"
	echo ""

	for engine in "${ENGINES[@]}"; do
		yaml_file=$(render_template "$engine")
		cfg_file=""
		if [[ "$engine" == "cloakbrowser" ]]; then
			cfg_file=$(make_cb_config)
		fi
		run_engine "$engine" "$yaml_file" "$cfg_file" || true
		list_outputs "$engine"
	done

	cleanup_tmp
	print_hints
}

main "$@"
