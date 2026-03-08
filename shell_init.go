package main

// shellInitScript returns the shell integration snippet for bash and zsh.
// Users add: eval "$(git ctx shell-init)" to ~/.bashrc or ~/.zshrc
func shellInitScript() string {
	return `# git-ctx shell integration
# Add to ~/.bashrc or ~/.zshrc:
#   eval "$(git ctx shell-init)"

# gc is a short alias for git-ctx
alias gc="git-ctx"

__git_ctx_auto() {
    git-ctx profile auto --silent 2>/dev/null
}

# bash
if [ -n "$BASH_VERSION" ]; then
    PROMPT_COMMAND="${PROMPT_COMMAND:+${PROMPT_COMMAND};}__git_ctx_auto"
fi

# zsh
if [ -n "$ZSH_VERSION" ]; then
    autoload -U add-zsh-hook
    add-zsh-hook chpwd __git_ctx_auto
    __git_ctx_auto  # run once on shell start
fi
`
}
