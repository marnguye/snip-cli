_snip_completions() {
    local cur prev commands
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD - 1]}"
    commands="add edit find remove run migrate storage"

    case "$prev" in
    run | edit | remove | rm)
        local IFS=$'\n'
        COMPREPLY=($(compgen -W "$(snip _list-descriptions 2>/dev/null)" -- "$cur"))
        return
        ;;
    esac

    if [[ "${COMP_WORDS[1]}" == "storage" ]]; then
        COMPREPLY=($(compgen -W "status use" -- "$cur"))
        if [[ "$prev" == "use" ]]; then
            COMPREPLY=($(compgen -W "toml legacy" -- "$cur"))
        fi
        return
    fi

    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
}

complete -F _snip_completions snip
