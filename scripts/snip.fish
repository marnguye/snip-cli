set -l commands add edit find remove run migrate storage

complete -c snip -f
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a add -d "Add new snippet"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a edit -d "Edit snippet command"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a find -d "Search snippets by query"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a remove -d "Remove snippet"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a run -d "Execute snippet by description"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a migrate -d "Migrate legacy Markdown snippets"
complete -c snip -n "not __fish_seen_subcommand_from $commands" -a storage -d "Inspect or select storage"

complete -c snip -n "__fish_seen_subcommand_from storage" -f -a "status use"
complete -c snip -n "__fish_seen_subcommand_from storage; and __fish_seen_subcommand_from use" -f -a "toml legacy"

complete -c snip -n "__fish_seen_subcommand_from run edit remove rm" -f -a "(snip _list-descriptions)"
