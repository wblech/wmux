# wmux shell-ready marker for fish.
# Source this file in config.fish to emit the shell-ready OSC sequence
# after each prompt is displayed.
#
# Usage: source /path/to/wmux-shell-ready.fish

# Only activate inside a wmux session.
if not set -q WMUX_SESSION_ID
    exit 0
end

function __wmux_shell_ready --on-event fish_prompt
    printf '\033]777;wmux;shell-ready\033\\'
end
