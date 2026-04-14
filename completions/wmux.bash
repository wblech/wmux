#!/usr/bin/env bash
# Bash completion for wmux.
# Source this file or place it in /etc/bash_completion.d/

_wmux_sessions() {
    local sessions
    sessions=$(wmux list --quiet 2>/dev/null)
    echo "$sessions"
}

_wmux_completions() {
    local cur prev subcmd
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Determine the subcommand (first non-flag word after 'wmux').
    subcmd=""
    local i=1
    while [[ $i -lt ${#COMP_WORDS[@]} ]]; do
        case "${COMP_WORDS[$i]}" in
            --socket|--token)
                ((i+=2))
                continue
                ;;
            -*)
                ((i++))
                continue
                ;;
            *)
                subcmd="${COMP_WORDS[$i]}"
                break
                ;;
        esac
    done

    # Complete subcommands.
    if [[ -z "$subcmd" || "$subcmd" == "$cur" ]]; then
        local subcommands="daemon create attach detach kill list info status events exec wait record history"
        COMPREPLY=($(compgen -W "$subcommands" -- "$cur"))
        return
    fi

    # Complete flags and arguments per subcommand.
    case "$subcmd" in
        attach|detach|info)
            COMPREPLY=($(compgen -W "$(_wmux_sessions)" -- "$cur"))
            ;;
        kill)
            case "$prev" in
                --prefix)
                    return
                    ;;
            esac
            COMPREPLY=($(compgen -W "--prefix $(_wmux_sessions)" -- "$cur"))
            ;;
        list)
            COMPREPLY=($(compgen -W "--prefix --quiet -q" -- "$cur"))
            ;;
        create)
            COMPREPLY=($(compgen -W "--shell --cwd" -- "$cur"))
            ;;
        exec)
            case "$prev" in
                --prefix)
                    return
                    ;;
            esac
            COMPREPLY=($(compgen -W "--sync --prefix --no-newline $(_wmux_sessions)" -- "$cur"))
            ;;
        events)
            COMPREPLY=($(compgen -W "$(_wmux_sessions)" -- "$cur"))
            ;;
        wait)
            case "$prev" in
                wait)
                    COMPREPLY=($(compgen -W "$(_wmux_sessions)" -- "$cur"))
                    return
                    ;;
                --timeout)
                    return
                    ;;
            esac
            COMPREPLY=($(compgen -W "exit idle match --timeout" -- "$cur"))
            ;;
        record)
            case "$prev" in
                record)
                    COMPREPLY=($(compgen -W "start stop" -- "$cur"))
                    return
                    ;;
            esac
            COMPREPLY=($(compgen -W "$(_wmux_sessions)" -- "$cur"))
            ;;
        history)
            case "$prev" in
                --format)
                    COMPREPLY=($(compgen -W "ansi text html" -- "$cur"))
                    return
                    ;;
                --lines)
                    return
                    ;;
                history)
                    COMPREPLY=($(compgen -W "$(_wmux_sessions)" -- "$cur"))
                    return
                    ;;
            esac
            COMPREPLY=($(compgen -W "--format --lines" -- "$cur"))
            ;;
    esac
}

complete -F _wmux_completions wmux
