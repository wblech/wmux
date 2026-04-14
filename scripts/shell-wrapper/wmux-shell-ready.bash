#!/usr/bin/env bash
# wmux shell-ready marker for bash.
# Source this file in .bashrc to emit the shell-ready OSC sequence
# after each prompt is displayed.
#
# Usage: source /path/to/wmux-shell-ready.bash

# Only activate inside a wmux session.
if [[ -z "${WMUX_SESSION_ID:-}" ]]; then
    return 0 2>/dev/null || exit 0
fi

__wmux_shell_ready() {
    printf '\033]777;wmux;shell-ready\033\\'
}

# Append to PROMPT_COMMAND so we emit after every prompt render.
if [[ -z "${PROMPT_COMMAND:-}" ]]; then
    PROMPT_COMMAND="__wmux_shell_ready"
else
    PROMPT_COMMAND="${PROMPT_COMMAND};__wmux_shell_ready"
fi
