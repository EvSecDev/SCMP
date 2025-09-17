_controller()
{
    local cur prev words cword
    _init_completion || return

    # Main config of options
    declare -A COMMANDS=(
        [root_sub]="deploy exec git install scp secrets seed version file header"
        [root_opts]="--allow-deletions --force --log-file --with-summary -c --config -T --dry-run -v --verbosity -w --wet-run"

        [deploy_sub]="all diff failures"
        [deploy_opts]="--disable-privilege-escalation --disable-reloads --execution-timeout --ignore-deployment-state --install --regex -C --commitid -l --local-files -m --max-conns -r --remote-hosts -t --test-config -u --run-as-user -M --max-deploy-threads"

        [deploy:all_opts]="__inherit__"
        [deploy:diff_opts]="__inherit__"
        [deploy:failures_opts]="__inherit__"

        [exec_opts]="--regex -r --remote-hosts -R --remote-file --disable-privilege-escalation -m --max-conns -u --run-as-user --execution-timeout"

        [git_sub]="add commit status"
        [git_opts]="-m --message"

        [git:commit_opts]="__inherit__"

        [install_opts]="--apparmor-profile --default-config --repository-branch-name --repository-path"

        [scp_opts]="-c --config"
        [secrets_opts]="-p --modify-vault-password"
        [seed_opts]="--regex -r --remote-hosts -R --remote-files"
        [version_opts]="-v"

        [file_sub]="new replace-data"
        [file_opts]="-y --yes"

        [file:new_opts]="__inherit__"
        [file:replace-data_opts]="__inherit__"

        [header_sub]="edit strip insert read verify"
        [header_opts]="-i --in-place -C --compact -j --json-metadata"

        [header:edit_opts]="__inherit__"
        [header:strip_opts]="__inherit__"
        [header:insert_opts]="__inherit__"
        [header:read_opts]="__inherit__"
    )

    # Special completion options
    case "$prev" in
        --remote-hosts|-r|--modify-vault-password|-p)
            local ssh_config="${HOME}/.ssh/config"
            if [[ -f "$ssh_config" ]]
            then
                mapfile -t COMPREPLY < <(awk '/^Host / {print $2}' "$ssh_config" | grep -i "^$cur")
            fi
            return 0
            ;;
        --commitid|-C)
            if [[ -d ".git" ]]
            then
                mapfile -t COMPREPLY < <(git log --pretty=format:"%H" -n 20 | grep -i "^$cur")
            fi
            return 0
            ;;
        --max-conns|-m)
            mapfile -t COMPREPLY < <(compgen -W "1 5 10 15 20 50" -- "$cur")
            return 0
            ;;
        --verbosity|-v|--verbose)
            mapfile -t COMPREPLY < <(compgen -W "0 1 2 3 4 5" -- "$cur")
            return 0
            ;;
    esac

    # Walk commands
    local path="root"
    for ((i=1; i < COMP_CWORD; i++))
    do
        local word="${COMP_WORDS[i]}"
        local subs="${COMMANDS[${path}_sub]}"

        if [[ -n "$subs" && " $subs " == *" $word "* ]]
        then
            # descend into this subcommand
            [[ "$path" == "root" ]] && path="$word" || path="$path:$word"
        else
            # no deeper subcommand match, stop here
            break
        fi
    done

    # Main suggestions
    local subs="${COMMANDS[${path}_sub]}"
    local opts="${COMMANDS[${path}_opts]}"

    # Handle flag to use parent args instead of explicit ones
    if [[ "$opts" == "__inherit__" ]]
    then
      # Only inherit from immediate parent
      if [[ "$path" == *:* ]]
      then
          local parent="${path%:*}"
      else
          local parent="root"
      fi
      opts="${COMMANDS[${parent}_opts]}"
    fi

    local globals="${COMMANDS[root_opts]}"

    local suggestions="$subs $opts $globals"

    if [[ "$cur" != -* ]]
    then
        # Base completions
        mapfile -t COMPREPLY < <(compgen -W "$suggestions" -- "$cur")

        # Append file/dir matches
        local files
        mapfile -t files < <(compgen -f -- "$cur")
        COMPREPLY+=( "${files[@]}" )

        # If current word resolves to a directory then no space after completion
        for i in "${!COMPREPLY[@]}"
        do
            if [[ -d "${COMPREPLY[$i]}" ]]
            then
                compopt -o nospace
                # shellcheck disable=SC2004
                COMPREPLY[$i]="${COMPREPLY[$i]}/"
            fi
        done
    else
        # Options only
        mapfile -t COMPREPLY < <(compgen -W "$suggestions" -- "$cur")
    fi
}
complete -F _controller controller
