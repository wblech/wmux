#!/usr/bin/env zsh
# wmux shell-ready marker for zsh.
# Source this file in .zshrc to emit the shell-ready OSC sequence
# after each prompt is displayed.
#
# Usage: source /path/to/wmux-shell-ready.zsh

# Only activate inside a wmux session.
if [[ -z "${WMUX_SESSION_ID:-}" ]]; then
    return 0 2>/dev/null || exit 0
fi

__wmux_shell_ready() {
    printf '\033]777;wmux;shell-ready\033\\'
}

# Use precmd hook to emit after every prompt render.
autoload -Uz add-zsh-hook
add-zsh-hook precmd __wmux_shell_ready
