# Fish completion for wmux.
# Place this file in ~/.config/fish/completions/ or /usr/share/fish/vendor_completions.d/

function __wmux_sessions
    wmux list --quiet 2>/dev/null
end

function __wmux_needs_command
    set -l cmd (commandline -opc)
    if test (count $cmd) -eq 1
        return 0
    end
    return 1
end

function __wmux_using_command
    set -l cmd (commandline -opc)
    if test (count $cmd) -gt 1
        if test "$argv[1]" = "$cmd[2]"
            return 0
        end
    end
    return 1
end

# Disable file completions by default.
complete -c wmux -f

# Global flags.
complete -c wmux -l socket -d 'Unix socket path' -r -F
complete -c wmux -l token -d 'Token file path' -r -F

# Subcommands.
complete -c wmux -n '__wmux_needs_command' -a daemon -d 'Start the daemon process'
complete -c wmux -n '__wmux_needs_command' -a create -d 'Create a new session'
complete -c wmux -n '__wmux_needs_command' -a attach -d 'Attach to a session'
complete -c wmux -n '__wmux_needs_command' -a detach -d 'Detach from a session'
complete -c wmux -n '__wmux_needs_command' -a kill -d 'Kill a session'
complete -c wmux -n '__wmux_needs_command' -a list -d 'List sessions'
complete -c wmux -n '__wmux_needs_command' -a info -d 'Show session info'
complete -c wmux -n '__wmux_needs_command' -a status -d 'Show daemon status'
complete -c wmux -n '__wmux_needs_command' -a events -d 'Subscribe to events'
complete -c wmux -n '__wmux_needs_command' -a exec -d 'Send input to a session'
complete -c wmux -n '__wmux_needs_command' -a wait -d 'Wait for a session condition'
complete -c wmux -n '__wmux_needs_command' -a record -d 'Start or stop session recording'
complete -c wmux -n '__wmux_needs_command' -a history -d 'Show session scrollback history'

# Session ID completion for subcommands that take a session.
complete -c wmux -n '__wmux_using_command attach' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command detach' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command info' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command kill' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command exec' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command events' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command wait' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command history' -a '(__wmux_sessions)' -d 'Session'

# Flags per subcommand.
complete -c wmux -n '__wmux_using_command kill' -l prefix -d 'Kill all sessions with prefix' -r
complete -c wmux -n '__wmux_using_command list' -l prefix -d 'Filter by prefix' -r
complete -c wmux -n '__wmux_using_command list' -l quiet -d 'Output IDs only'
complete -c wmux -n '__wmux_using_command list' -s q -d 'Output IDs only'
complete -c wmux -n '__wmux_using_command create' -l shell -d 'Shell binary' -r
complete -c wmux -n '__wmux_using_command create' -l cwd -d 'Working directory' -r -F
complete -c wmux -n '__wmux_using_command exec' -l sync -d 'Send to multiple sessions'
complete -c wmux -n '__wmux_using_command exec' -l prefix -d 'Target by prefix' -r
complete -c wmux -n '__wmux_using_command exec' -l no-newline -d 'Do not append newline'
complete -c wmux -n '__wmux_using_command wait' -a 'exit idle match' -d 'Wait mode'
complete -c wmux -n '__wmux_using_command wait' -l timeout -d 'Timeout in milliseconds' -r
complete -c wmux -n '__wmux_using_command record' -a 'start stop' -d 'Action'
complete -c wmux -n '__wmux_using_command record' -a '(__wmux_sessions)' -d 'Session'
complete -c wmux -n '__wmux_using_command history' -l format -d 'Output format' -r -a 'ansi text html'
complete -c wmux -n '__wmux_using_command history' -l lines -d 'Number of lines' -r
